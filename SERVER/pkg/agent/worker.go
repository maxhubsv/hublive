// Copyright 2024 HubLive, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	pagent "__GITHUB_HUBLIVE__protocol/agent"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/rpc"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/guid"
	"__GITHUB_HUBLIVE__psrpc"
)

var (
	ErrUnimplementedWrorkerSignal = errors.New("unimplemented worker signal")
	ErrUnknownWorkerSignal        = errors.New("unknown worker signal")
	ErrUnknownJobType             = errors.New("unknown job type")
	ErrJobNotFound                = psrpc.NewErrorf(psrpc.NotFound, "no running job for given jobID")
	ErrWorkerClosed               = errors.New("worker closed")
	ErrWorkerNotAvailable         = errors.New("worker not available")
	ErrAvailabilityTimeout        = errors.New("agent worker availability timeout")
	ErrDuplicateJobAssignment     = errors.New("duplicate job assignment")
)

const AgentNameAttributeKey = "lk.agent_name"

type WorkerProtocolVersion int

const CurrentProtocol = 1

const (
	RegisterTimeout  = 10 * time.Second
	AssignJobTimeout = 10 * time.Second
)

type SignalConn interface {
	WriteServerMessage(msg *hublive.ServerMessage) (int, error)
	ReadWorkerMessage() (*hublive.WorkerMessage, int, error)
	SetReadDeadline(time.Time) error
	Close() error
}

func JobStatusIsEnded(s hublive.JobStatus) bool {
	return s == hublive.JobStatus_JS_SUCCESS || s == hublive.JobStatus_JS_FAILED
}

type WorkerSignalHandler interface {
	HandleRegister(*hublive.RegisterWorkerRequest) error
	HandleAvailability(*hublive.AvailabilityResponse) error
	HandleUpdateJob(*hublive.UpdateJobStatus) error
	HandleSimulateJob(*hublive.SimulateJobRequest) error
	HandlePing(*hublive.WorkerPing) error
	HandleUpdateWorker(*hublive.UpdateWorkerStatus) error
	HandleMigrateJob(*hublive.MigrateJobRequest) error
}

func DispatchWorkerSignal(req *hublive.WorkerMessage, h WorkerSignalHandler) error {
	switch m := req.Message.(type) {
	case *hublive.WorkerMessage_Register:
		return h.HandleRegister(m.Register)
	case *hublive.WorkerMessage_Availability:
		return h.HandleAvailability(m.Availability)
	case *hublive.WorkerMessage_UpdateJob:
		return h.HandleUpdateJob(m.UpdateJob)
	case *hublive.WorkerMessage_SimulateJob:
		return h.HandleSimulateJob(m.SimulateJob)
	case *hublive.WorkerMessage_Ping:
		return h.HandlePing(m.Ping)
	case *hublive.WorkerMessage_UpdateWorker:
		return h.HandleUpdateWorker(m.UpdateWorker)
	case *hublive.WorkerMessage_MigrateJob:
		return h.HandleMigrateJob(m.MigrateJob)
	default:
		return ErrUnknownWorkerSignal
	}
}

var _ WorkerSignalHandler = (*UnimplementedWorkerSignalHandler)(nil)

type UnimplementedWorkerSignalHandler struct{}

func (UnimplementedWorkerSignalHandler) HandleRegister(*hublive.RegisterWorkerRequest) error {
	return fmt.Errorf("%w: Register", ErrUnimplementedWrorkerSignal)
}
func (UnimplementedWorkerSignalHandler) HandleAvailability(*hublive.AvailabilityResponse) error {
	return fmt.Errorf("%w: Availability", ErrUnimplementedWrorkerSignal)
}
func (UnimplementedWorkerSignalHandler) HandleUpdateJob(*hublive.UpdateJobStatus) error {
	return fmt.Errorf("%w: UpdateJob", ErrUnimplementedWrorkerSignal)
}
func (UnimplementedWorkerSignalHandler) HandleSimulateJob(*hublive.SimulateJobRequest) error {
	return fmt.Errorf("%w: SimulateJob", ErrUnimplementedWrorkerSignal)
}
func (UnimplementedWorkerSignalHandler) HandlePing(*hublive.WorkerPing) error {
	return fmt.Errorf("%w: Ping", ErrUnimplementedWrorkerSignal)
}
func (UnimplementedWorkerSignalHandler) HandleUpdateWorker(*hublive.UpdateWorkerStatus) error {
	return fmt.Errorf("%w: UpdateWorker", ErrUnimplementedWrorkerSignal)
}
func (UnimplementedWorkerSignalHandler) HandleMigrateJob(*hublive.MigrateJobRequest) error {
	return fmt.Errorf("%w: MigrateJob", ErrUnimplementedWrorkerSignal)
}

type WorkerPingHandler struct {
	UnimplementedWorkerSignalHandler
	conn SignalConn
}

func (h WorkerPingHandler) HandlePing(ping *hublive.WorkerPing) error {
	_, err := h.conn.WriteServerMessage(&hublive.ServerMessage{
		Message: &hublive.ServerMessage_Pong{
			Pong: &hublive.WorkerPong{
				LastTimestamp: ping.Timestamp,
				Timestamp:     time.Now().UnixMilli(),
			},
		},
	})
	return err
}

type WorkerRegistration struct {
	Protocol    WorkerProtocolVersion
	ID          string
	Version     string
	AgentID     string
	AgentName   string
	Namespace   string
	JobType     hublive.JobType
	Permissions *hublive.ParticipantPermission
	ClientIP    string
}

func MakeWorkerRegistration() WorkerRegistration {
	return WorkerRegistration{
		ID:       guid.New(guid.AgentWorkerPrefix),
		Protocol: CurrentProtocol,
	}
}

var _ WorkerSignalHandler = (*WorkerRegisterer)(nil)

type WorkerRegisterer struct {
	WorkerPingHandler
	serverInfo *hublive.ServerInfo
	deadline   time.Time

	registration WorkerRegistration
	registered   bool
}

func NewWorkerRegisterer(conn SignalConn, serverInfo *hublive.ServerInfo, base WorkerRegistration) *WorkerRegisterer {
	return &WorkerRegisterer{
		WorkerPingHandler: WorkerPingHandler{conn: conn},
		serverInfo:        serverInfo,
		registration:      base,
		deadline:          time.Now().Add(RegisterTimeout),
	}
}

func (h *WorkerRegisterer) Deadline() time.Time {
	return h.deadline
}

func (h *WorkerRegisterer) Registration() WorkerRegistration {
	return h.registration
}

func (h *WorkerRegisterer) Registered() bool {
	return h.registered
}

func (h *WorkerRegisterer) HandleRegister(req *hublive.RegisterWorkerRequest) error {
	if !hublive.IsJobType(req.GetType()) {
		return ErrUnknownJobType
	}

	permissions := req.AllowedPermissions
	if permissions == nil {
		permissions = &hublive.ParticipantPermission{
			CanSubscribe:      true,
			CanPublish:        true,
			CanPublishData:    true,
			CanUpdateMetadata: true,
		}
	}

	h.registration.Version = req.Version
	h.registration.AgentName = req.AgentName
	h.registration.Namespace = req.GetNamespace()
	h.registration.JobType = req.GetType()
	h.registration.Permissions = permissions
	h.registered = true

	_, err := h.conn.WriteServerMessage(&hublive.ServerMessage{
		Message: &hublive.ServerMessage_Register{
			Register: &hublive.RegisterWorkerResponse{
				WorkerId:   h.registration.ID,
				ServerInfo: h.serverInfo,
			},
		},
	})
	return err
}

var _ WorkerSignalHandler = (*Worker)(nil)

type Worker struct {
	WorkerPingHandler
	WorkerRegistration

	apiKey    string
	apiSecret string
	logger    logger.Logger

	ctx    context.Context
	cancel context.CancelFunc
	closed chan struct{}

	mu     sync.Mutex
	load   float32
	status hublive.WorkerStatus

	runningJobs  map[hublive.JobID]*hublive.Job
	availability map[hublive.JobID]chan *hublive.AvailabilityResponse
}

func NewWorker(
	registration WorkerRegistration,
	apiKey string,
	apiSecret string,
	conn SignalConn,
	logger logger.Logger,
) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		WorkerPingHandler:  WorkerPingHandler{conn: conn},
		WorkerRegistration: registration,
		apiKey:             apiKey,
		apiSecret:          apiSecret,
		logger: logger.WithValues(
			"workerID", registration.ID,
			"agentName", registration.AgentName,
			"jobType", registration.JobType.String(),
		),

		ctx:    ctx,
		cancel: cancel,
		closed: make(chan struct{}),

		runningJobs:  make(map[hublive.JobID]*hublive.Job),
		availability: make(map[hublive.JobID]chan *hublive.AvailabilityResponse),
	}
}

func (w *Worker) sendRequest(req *hublive.ServerMessage) {
	if _, err := w.conn.WriteServerMessage(req); err != nil {
		w.logger.Warnw("error writing to websocket", err)
	}
}

func (w *Worker) Status() hublive.WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *Worker) Load() float32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.load
}

func (w *Worker) Logger() logger.Logger {
	return w.logger
}

func (w *Worker) RunningJobs() map[hublive.JobID]*hublive.Job {
	w.mu.Lock()
	defer w.mu.Unlock()
	jobs := make(map[hublive.JobID]*hublive.Job, len(w.runningJobs))
	for k, v := range w.runningJobs {
		jobs[k] = v
	}
	return jobs
}

func (w *Worker) RunningJobCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.runningJobs)
}

func (w *Worker) GetJobState(jobID hublive.JobID) (*hublive.JobState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	j, ok := w.runningJobs[jobID]
	if !ok {
		return nil, ErrJobNotFound
	}
	return utils.CloneProto(j.State), nil
}

func (w *Worker) AssignJob(ctx context.Context, job *hublive.Job) (*hublive.JobState, error) {
	availCh := make(chan *hublive.AvailabilityResponse, 1)
	job = utils.CloneProto(job)
	jobID := hublive.JobID(job.Id)

	w.mu.Lock()
	if _, ok := w.availability[jobID]; ok {
		w.mu.Unlock()
		return nil, ErrDuplicateJobAssignment
	}

	w.availability[jobID] = availCh
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		delete(w.availability, jobID)
		w.mu.Unlock()
	}()

	if job.State == nil {
		job.State = &hublive.JobState{}
	}
	now := time.Now()
	job.State.WorkerId = w.ID
	job.State.AgentId = w.AgentID
	job.State.UpdatedAt = now.UnixNano()
	job.State.StartedAt = now.UnixNano()
	job.State.Status = hublive.JobStatus_JS_RUNNING

	w.sendRequest(&hublive.ServerMessage{Message: &hublive.ServerMessage_Availability{
		Availability: &hublive.AvailabilityRequest{Job: job},
	}})

	timeout := time.NewTimer(AssignJobTimeout)
	defer timeout.Stop()

	// See handleAvailability for the response
	select {
	case res := <-availCh:
		if res.Terminate {
			job.State.EndedAt = now.UnixNano()
			job.State.Status = hublive.JobStatus_JS_SUCCESS
			return job.State, nil
		}

		if !res.Available {
			return nil, ErrWorkerNotAvailable
		}

		job.State.ParticipantIdentity = res.ParticipantIdentity
		attributes := res.ParticipantAttributes
		if attributes == nil {
			attributes = make(map[string]string)
		}
		attributes[AgentNameAttributeKey] = w.AgentName

		token, err := pagent.BuildAgentToken(
			w.apiKey,
			w.apiSecret,
			job.Room.Name,
			res.ParticipantIdentity,
			res.ParticipantName,
			res.ParticipantMetadata,
			attributes,
			w.Permissions,
		)
		if err != nil {
			w.logger.Errorw("failed to build agent token", err)
			return nil, err
		}

		// In OSS, Url is nil, and the used API Key is the same as the one used to connect the worker
		w.sendRequest(&hublive.ServerMessage{Message: &hublive.ServerMessage_Assignment{
			Assignment: &hublive.JobAssignment{Job: job, Url: nil, Token: token},
		}})

		state := utils.CloneProto(job.State)

		w.mu.Lock()
		w.runningJobs[jobID] = job
		w.mu.Unlock()

		// TODO sweep jobs that are never started. We can't do this until all SDKs actually update the the JOB state

		return state, nil
	case <-timeout.C:
		return nil, ErrAvailabilityTimeout
	case <-w.ctx.Done():
		return nil, ErrWorkerClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *Worker) TerminateJob(jobID hublive.JobID, reason rpc.JobTerminateReason) (*hublive.JobState, error) {
	w.mu.Lock()
	_, ok := w.runningJobs[jobID]
	w.mu.Unlock()

	if !ok {
		return nil, ErrJobNotFound
	}

	w.sendRequest(&hublive.ServerMessage{Message: &hublive.ServerMessage_Termination{
		Termination: &hublive.JobTermination{
			JobId: string(jobID),
		},
	}})

	status := hublive.JobStatus_JS_SUCCESS
	errorStr := ""
	if reason == rpc.JobTerminateReason_AGENT_LEFT_ROOM {
		status = hublive.JobStatus_JS_FAILED
		errorStr = "agent worker left the room"
	}

	return w.UpdateJobStatus(&hublive.UpdateJobStatus{
		JobId:  string(jobID),
		Status: status,
		Error:  errorStr,
	})
}

func (w *Worker) UpdateMetadata(metadata string) {
	w.logger.Debugw("worker metadata updated", nil, "metadata", metadata)
}

func (w *Worker) IsClosed() bool {
	select {
	case <-w.closed:
		return true
	default:
		return false
	}
}

func (w *Worker) Close() {
	w.mu.Lock()
	if w.IsClosed() {
		w.mu.Unlock()
		return
	}

	w.logger.Infow("closing worker", "workerID", w.ID, "jobType", w.JobType, "agentName", w.AgentName)

	close(w.closed)
	w.cancel()
	_ = w.conn.Close()
	w.mu.Unlock()
}

func (w *Worker) HandleAvailability(res *hublive.AvailabilityResponse) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	jobID := hublive.JobID(res.JobId)
	availCh, ok := w.availability[jobID]
	if !ok {
		w.logger.Warnw("received availability response for unknown job", nil, "jobID", jobID)
		return nil
	}

	availCh <- res
	delete(w.availability, jobID)

	return nil
}

func (w *Worker) HandleUpdateJob(update *hublive.UpdateJobStatus) error {
	_, err := w.UpdateJobStatus(update)
	if err != nil {
		// treating this as a debug message only
		// this can happen if the Room closes first, which would delete the agent dispatch
		// that would mark the job as successful. subsequent updates from the same worker
		// would not be able to find the same jobID.
		w.logger.Debugw("received job update for unknown job", "jobID", update.JobId)
	}
	return nil
}

func (w *Worker) UpdateJobStatus(update *hublive.UpdateJobStatus) (*hublive.JobState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	jobID := hublive.JobID(update.JobId)
	job, ok := w.runningJobs[jobID]
	if !ok {
		return nil, psrpc.NewErrorf(psrpc.NotFound, "received job update for unknown job")
	}

	now := time.Now()
	job.State.UpdatedAt = now.UnixNano()

	if job.State.Status == hublive.JobStatus_JS_PENDING && update.Status != hublive.JobStatus_JS_PENDING {
		job.State.StartedAt = now.UnixNano()
	}

	job.State.Status = update.Status
	job.State.Error = update.Error

	if JobStatusIsEnded(update.Status) {
		job.State.EndedAt = now.UnixNano()
		delete(w.runningJobs, jobID)

		w.logger.Infow("job ended", "jobID", update.JobId, "status", update.Status, "error", update.Error)
	}

	return proto.Clone(job.State).(*hublive.JobState), nil
}

func (w *Worker) HandleSimulateJob(simulate *hublive.SimulateJobRequest) error {
	jobType := hublive.JobType_JT_ROOM
	if simulate.Participant != nil {
		jobType = hublive.JobType_JT_PUBLISHER
	}

	job := &hublive.Job{
		Id:          guid.New(guid.AgentJobPrefix),
		Type:        jobType,
		Room:        simulate.Room,
		Participant: simulate.Participant,
		Namespace:   w.Namespace,
		AgentName:   w.AgentName,
	}

	go func() {
		_, err := w.AssignJob(w.ctx, job)
		if err != nil {
			w.logger.Errorw("unable to simulate job", err, "jobID", job.Id)
		}
	}()

	return nil
}

func (w *Worker) HandleUpdateWorker(update *hublive.UpdateWorkerStatus) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if status := update.Status; status != nil && w.status != *status {
		w.status = *status
		w.Logger().Debugw("worker status changed", "status", w.status)
	}
	w.load = update.GetLoad()

	return nil
}

func (w *Worker) HandleMigrateJob(req *hublive.MigrateJobRequest) error {
	// TODO(theomonnom): On OSS this is not implemented
	// We could maybe just move a specific job to another worker
	return nil
}
