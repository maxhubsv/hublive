package testutils

import (
	"context"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/frostbyte73/core"
	"github.com/gammazero/deque"
	"golang.org/x/exp/maps"

	"github.com/maxhubsv/hublive-server/pkg/agent"
	"github.com/maxhubsv/hublive-server/pkg/config"
	"github.com/maxhubsv/hublive-server/pkg/routing"
	"github.com/maxhubsv/hublive-server/pkg/service"
	"__GITHUB_HUBLIVE__protocol/auth"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/utils/events"
	"__GITHUB_HUBLIVE__protocol/utils/guid"
	"__GITHUB_HUBLIVE__protocol/utils/must"
	"__GITHUB_HUBLIVE__protocol/utils/options"
	"__GITHUB_HUBLIVE__psrpc"
)

type AgentService interface {
	HandleConnection(context.Context, agent.SignalConn, agent.WorkerRegistration)
	DrainConnections(time.Duration)
}

type TestServer struct {
	AgentService
	TestAPIKey    string
	TestAPISecret string
}

func NewTestServer(bus psrpc.MessageBus) *TestServer {
	localNode, _ := routing.NewLocalNode(nil)
	return NewTestServerWithService(must.Get(service.NewAgentService(
		&config.Config{Region: "test"},
		localNode,
		bus,
		auth.NewSimpleKeyProvider("test", "verysecretsecret"),
	)))
}

func NewTestServerWithService(s AgentService) *TestServer {
	return &TestServer{s, "test", "verysecretsecret"}
}

type SimulatedWorkerOptions struct {
	Context            context.Context
	Label              string
	SupportResume      bool
	DefaultJobLoad     float32
	JobLoadThreshold   float32
	DefaultWorkerLoad  float32
	HandleAvailability func(AgentJobRequest)
	HandleAssignment   func(*hublive.Job) JobLoad
}

type SimulatedWorkerOption func(*SimulatedWorkerOptions)

func WithContext(ctx context.Context) SimulatedWorkerOption {
	return func(o *SimulatedWorkerOptions) {
		o.Context = ctx
	}
}

func WithLabel(label string) SimulatedWorkerOption {
	return func(o *SimulatedWorkerOptions) {
		o.Label = label
	}
}

func WithJobAvailabilityHandler(h func(AgentJobRequest)) SimulatedWorkerOption {
	return func(o *SimulatedWorkerOptions) {
		o.HandleAvailability = h
	}
}

func WithJobAssignmentHandler(h func(*hublive.Job) JobLoad) SimulatedWorkerOption {
	return func(o *SimulatedWorkerOptions) {
		o.HandleAssignment = h
	}
}

func WithJobLoad(l JobLoad) SimulatedWorkerOption {
	return WithJobAssignmentHandler(func(j *hublive.Job) JobLoad { return l })
}

func WithDefaultWorkerLoad(load float32) SimulatedWorkerOption {
	return func(o *SimulatedWorkerOptions) {
		o.DefaultWorkerLoad = load
	}
}

func (h *TestServer) SimulateAgentWorker(opts ...SimulatedWorkerOption) *AgentWorker {
	o := &SimulatedWorkerOptions{
		Context:            context.Background(),
		Label:              guid.New("TEST_AGENT_"),
		DefaultJobLoad:     0.1,
		JobLoadThreshold:   0.8,
		DefaultWorkerLoad:  0.0,
		HandleAvailability: func(r AgentJobRequest) { r.Accept() },
		HandleAssignment:   func(j *hublive.Job) JobLoad { return nil },
	}
	options.Apply(o, opts)

	w := &AgentWorker{
		workerMessages:         make(chan *hublive.WorkerMessage, 1),
		jobs:                   map[string]*AgentJob{},
		SimulatedWorkerOptions: o,

		RegisterWorkerResponses: events.NewObserverList[*hublive.RegisterWorkerResponse](),
		AvailabilityRequests:    events.NewObserverList[*hublive.AvailabilityRequest](),
		JobAssignments:          events.NewObserverList[*hublive.JobAssignment](),
		JobTerminations:         events.NewObserverList[*hublive.JobTermination](),
		WorkerPongs:             events.NewObserverList[*hublive.WorkerPong](),
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())

	if o.DefaultWorkerLoad > 0.0 {
		w.sendStatus()
	}

	ctx := service.WithAPIKey(o.Context, &auth.ClaimGrants{}, "test")
	go h.HandleConnection(ctx, w, agent.MakeWorkerRegistration())

	return w
}

func (h *TestServer) Close() {
	h.DrainConnections(1)
}

var _ agent.SignalConn = (*AgentWorker)(nil)

type JobLoad interface {
	Load() float32
}

type AgentJob struct {
	*hublive.Job
	JobLoad
}

type AgentJobRequest struct {
	w *AgentWorker
	*hublive.AvailabilityRequest
}

func (r AgentJobRequest) Accept() {
	identity := guid.New("PI_")
	r.w.SendAvailability(&hublive.AvailabilityResponse{
		JobId:               r.Job.Id,
		Available:           true,
		SupportsResume:      r.w.SupportResume,
		ParticipantName:     identity,
		ParticipantIdentity: identity,
	})
}

func (r AgentJobRequest) Reject() {
	r.w.SendAvailability(&hublive.AvailabilityResponse{
		JobId:     r.Job.Id,
		Available: false,
	})
}

type AgentWorker struct {
	*SimulatedWorkerOptions

	fuse           core.Fuse
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	workerMessages chan *hublive.WorkerMessage
	serverMessages deque.Deque[*hublive.ServerMessage]
	jobs           map[string]*AgentJob

	RegisterWorkerResponses *events.ObserverList[*hublive.RegisterWorkerResponse]
	AvailabilityRequests    *events.ObserverList[*hublive.AvailabilityRequest]
	JobAssignments          *events.ObserverList[*hublive.JobAssignment]
	JobTerminations         *events.ObserverList[*hublive.JobTermination]
	WorkerPongs             *events.ObserverList[*hublive.WorkerPong]
}

func (w *AgentWorker) statusWorker() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()

	for !w.fuse.IsBroken() {
		w.sendStatus()
		<-t.C
	}
}

func (w *AgentWorker) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fuse.Break()
	return nil
}

// Closed returns a channel that is closed when the connection is closed by the server
func (w *AgentWorker) Closed() <-chan struct{} {
	return w.fuse.Watch()
}

func (w *AgentWorker) SetReadDeadline(t time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.fuse.IsBroken() {
		cancel := w.cancel
		if t.IsZero() {
			w.ctx, w.cancel = context.WithCancel(context.Background())
		} else {
			w.ctx, w.cancel = context.WithDeadline(context.Background(), t)
		}
		cancel()
	}
	return nil
}

func (w *AgentWorker) ReadWorkerMessage() (*hublive.WorkerMessage, int, error) {
	for {
		w.mu.Lock()
		ctx := w.ctx
		w.mu.Unlock()

		select {
		case <-w.fuse.Watch():
			return nil, 0, io.EOF
		case <-ctx.Done():
			if err := ctx.Err(); errors.Is(err, context.DeadlineExceeded) {
				return nil, 0, err
			}
		case m := <-w.workerMessages:
			return m, 0, nil
		}
	}
}

func (w *AgentWorker) WriteServerMessage(m *hublive.ServerMessage) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.serverMessages.PushBack(m)
	if w.serverMessages.Len() == 1 {
		go w.handleServerMessages()
	}
	return 0, nil
}

func (w *AgentWorker) handleServerMessages() {
	w.mu.Lock()
	for w.serverMessages.Len() != 0 {
		m := w.serverMessages.Front()
		w.mu.Unlock()

		switch m := m.Message.(type) {
		case *hublive.ServerMessage_Register:
			w.handleRegister(m.Register)
		case *hublive.ServerMessage_Availability:
			w.handleAvailability(m.Availability)
		case *hublive.ServerMessage_Assignment:
			w.handleAssignment(m.Assignment)
		case *hublive.ServerMessage_Termination:
			w.handleTermination(m.Termination)
		case *hublive.ServerMessage_Pong:
			w.handlePong(m.Pong)
		}

		w.mu.Lock()
		w.serverMessages.PopFront()
	}
	w.mu.Unlock()
}

func (w *AgentWorker) handleRegister(m *hublive.RegisterWorkerResponse) {
	w.RegisterWorkerResponses.Emit(m)
}

func (w *AgentWorker) handleAvailability(m *hublive.AvailabilityRequest) {
	w.AvailabilityRequests.Emit(m)
	if w.HandleAvailability != nil {
		w.HandleAvailability(AgentJobRequest{w, m})
	} else {
		AgentJobRequest{w, m}.Accept()
	}
}

func (w *AgentWorker) handleAssignment(m *hublive.JobAssignment) {
	w.JobAssignments.Emit(m)

	var load JobLoad
	if w.HandleAssignment != nil {
		load = w.HandleAssignment(m.Job)
	}

	if load == nil {
		load = NewStableJobLoad(w.DefaultJobLoad)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.jobs[m.Job.Id] = &AgentJob{m.Job, load}
}

func (w *AgentWorker) handleTermination(m *hublive.JobTermination) {
	w.JobTerminations.Emit(m)

	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.jobs, m.JobId)
}

func (w *AgentWorker) handlePong(m *hublive.WorkerPong) {
	w.WorkerPongs.Emit(m)
}

func (w *AgentWorker) SendMessage(m *hublive.WorkerMessage) {
	select {
	case <-w.fuse.Watch():
	case w.workerMessages <- m:
	}
}

func (w *AgentWorker) SendRegister(m *hublive.RegisterWorkerRequest) {
	w.SendMessage(&hublive.WorkerMessage{Message: &hublive.WorkerMessage_Register{
		Register: m,
	}})
}

func (w *AgentWorker) SendAvailability(m *hublive.AvailabilityResponse) {
	w.SendMessage(&hublive.WorkerMessage{Message: &hublive.WorkerMessage_Availability{
		Availability: m,
	}})
}

func (w *AgentWorker) SendUpdateWorker(m *hublive.UpdateWorkerStatus) {
	w.SendMessage(&hublive.WorkerMessage{Message: &hublive.WorkerMessage_UpdateWorker{
		UpdateWorker: m,
	}})
}

func (w *AgentWorker) SendUpdateJob(m *hublive.UpdateJobStatus) {
	w.SendMessage(&hublive.WorkerMessage{Message: &hublive.WorkerMessage_UpdateJob{
		UpdateJob: m,
	}})
}

func (w *AgentWorker) SendPing(m *hublive.WorkerPing) {
	w.SendMessage(&hublive.WorkerMessage{Message: &hublive.WorkerMessage_Ping{
		Ping: m,
	}})
}

func (w *AgentWorker) SendSimulateJob(m *hublive.SimulateJobRequest) {
	w.SendMessage(&hublive.WorkerMessage{Message: &hublive.WorkerMessage_SimulateJob{
		SimulateJob: m,
	}})
}

func (w *AgentWorker) SendMigrateJob(m *hublive.MigrateJobRequest) {
	w.SendMessage(&hublive.WorkerMessage{Message: &hublive.WorkerMessage_MigrateJob{
		MigrateJob: m,
	}})
}

func (w *AgentWorker) sendStatus() {
	w.mu.Lock()
	var load float32
	jobCount := len(w.jobs)

	if len(w.jobs) == 0 {
		load = w.DefaultWorkerLoad
	} else {
		for _, j := range w.jobs {
			load += j.Load()
		}
	}
	w.mu.Unlock()

	status := hublive.WorkerStatus_WS_AVAILABLE
	if load > w.JobLoadThreshold {
		status = hublive.WorkerStatus_WS_FULL
	}

	w.SendUpdateWorker(&hublive.UpdateWorkerStatus{
		Status:   &status,
		Load:     load,
		JobCount: uint32(jobCount),
	})
}

func (w *AgentWorker) Register(agentName string, jobType hublive.JobType) {
	w.SendRegister(&hublive.RegisterWorkerRequest{
		Type:      jobType,
		AgentName: agentName,
	})
	go w.statusWorker()
}

func (w *AgentWorker) SimulateRoomJob(roomName string) {
	w.SendSimulateJob(&hublive.SimulateJobRequest{
		Type: hublive.JobType_JT_ROOM,
		Room: &hublive.Room{
			Sid:  guid.New(guid.RoomPrefix),
			Name: roomName,
		},
	})
}

func (w *AgentWorker) Jobs() []*AgentJob {
	w.mu.Lock()
	defer w.mu.Unlock()
	return maps.Values(w.jobs)
}

type stableJobLoad struct {
	load float32
}

func NewStableJobLoad(load float32) JobLoad {
	return stableJobLoad{load}
}

func (s stableJobLoad) Load() float32 {
	return s.load
}

type periodicJobLoad struct {
	amplitude float64
	period    time.Duration
	epoch     time.Time
}

func NewPeriodicJobLoad(max float32, period time.Duration) JobLoad {
	return periodicJobLoad{
		amplitude: float64(max / 2),
		period:    period,
		epoch:     time.Now().Add(-time.Duration(rand.Int64N(int64(period)))),
	}
}

func (s periodicJobLoad) Load() float32 {
	a := math.Sin(time.Since(s.epoch).Seconds() / s.period.Seconds() * math.Pi * 2)
	return float32(s.amplitude + a*s.amplitude)
}

type uniformRandomJobLoad struct {
	min, max float32
	rng      func() float64
}

func NewUniformRandomJobLoad(min, max float32) JobLoad {
	return uniformRandomJobLoad{min, max, rand.Float64}
}

func NewUniformRandomJobLoadWithRNG(min, max float32, rng *rand.Rand) JobLoad {
	return uniformRandomJobLoad{min, max, rng.Float64}
}

func (s uniformRandomJobLoad) Load() float32 {
	return rand.Float32()*(s.max-s.min) + s.min
}

type normalRandomJobLoad struct {
	mean, stddev float64
	rng          func() float64
}

func NewNormalRandomJobLoad(mean, stddev float64) JobLoad {
	return normalRandomJobLoad{mean, stddev, rand.Float64}
}

func NewNormalRandomJobLoadWithRNG(mean, stddev float64, rng *rand.Rand) JobLoad {
	return normalRandomJobLoad{mean, stddev, rng.Float64}
}

func (s normalRandomJobLoad) Load() float32 {
	u := 1 - s.rng()
	v := s.rng()
	z := math.Sqrt(-2*math.Log(u)) * math.Cos(2*math.Pi*v)
	return float32(max(0, z*s.stddev+s.mean))
}
