// Copyright 2023 HubLive, Inc.
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

package rtc

import (
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"time"

	"github.com/frostbyte73/core"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"go.uber.org/zap/zapcore"
	"golang.org/x/exp/maps"
	"google.golang.org/protobuf/proto"

	"__GITHUB_HUBLIVE__mediatransportutil/pkg/twcc"
	"__GITHUB_HUBLIVE__protocol/auth"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/observability"
	"__GITHUB_HUBLIVE__protocol/observability/roomobs"
	protosignalling "__GITHUB_HUBLIVE__protocol/signalling"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/pointer"
	"__GITHUB_HUBLIVE__psrpc"

	"github.com/maxhubsv/hublive-server/pkg/config"
	"github.com/maxhubsv/hublive-server/pkg/metric"
	"github.com/maxhubsv/hublive-server/pkg/routing"
	"github.com/maxhubsv/hublive-server/pkg/rtc/signalling"
	"github.com/maxhubsv/hublive-server/pkg/rtc/supervisor"
	"github.com/maxhubsv/hublive-server/pkg/rtc/transport"
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/sfu"
	"github.com/maxhubsv/hublive-server/pkg/sfu/buffer"
	"github.com/maxhubsv/hublive-server/pkg/sfu/connectionquality"
	"github.com/maxhubsv/hublive-server/pkg/sfu/interceptor"
	"github.com/maxhubsv/hublive-server/pkg/sfu/pacer"
	"github.com/maxhubsv/hublive-server/pkg/sfu/streamallocator"
	"github.com/maxhubsv/hublive-server/pkg/telemetry"
	"github.com/maxhubsv/hublive-server/pkg/telemetry/prometheus"
	sutils "github.com/maxhubsv/hublive-server/pkg/utils"
)

var _ types.LocalParticipant = (*ParticipantImpl)(nil)

const (
	sdBatchSize       = 30
	rttUpdateInterval = 5 * time.Second

	disconnectCleanupDuration          = 5 * time.Second
	migrationWaitDuration              = 3 * time.Second
	migrationWaitContinuousMsgDuration = 2 * time.Second

	PingIntervalSeconds = 5
	PingTimeoutSeconds  = 15

	audioSectionsCountWithJoinResponse = 3
	videoSectionsCountWithJoinResponse = 3
)

var (
	ErrMoveOldClientVersion = errors.New("participant client version does not support moving")
)

// -------------------------------------------------

type pendingTrackInfo struct {
	trackInfos []*hublive.TrackInfo
	sdpRids    buffer.VideoLayersRid
	migrated   bool
	createdAt  time.Time

	// indicates if this track is queued for publishing to avoid a track has been published
	// before the previous track is unpublished(closed) because client is allowed to negotiate
	// webrtc track before AddTrackRequest return to speed up the publishing process
	queued bool
}

func (p *pendingTrackInfo) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if p == nil {
		return nil
	}

	e.AddArray("trackInfos", logger.ProtoSlice(p.trackInfos))
	e.AddArray("sdpRids", logger.StringSlice(p.sdpRids[:]))
	e.AddBool("migrated", p.migrated)
	e.AddTime("createdAt", p.createdAt)
	e.AddBool("queued", p.queued)
	return nil
}

// --------------------------------------------------

type pendingRemoteTrack struct {
	track    *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
}

type downTrackState struct {
	transceiver *webrtc.RTPTransceiver
	downTrack   sfu.DownTrackState
}

type postRtcpOp struct {
	*ParticipantImpl
	pkts []rtcp.Packet
}

// ---------------------------------------------------------------

type participantUpdateInfo struct {
	identity  hublive.ParticipantIdentity
	version   uint32
	state     hublive.ParticipantInfo_State
	updatedAt time.Time
}

func (p participantUpdateInfo) String() string {
	return fmt.Sprintf("identity: %s, version: %d, state: %s, updatedAt: %s", p.identity, p.version, p.state.String(), p.updatedAt.String())
}

type reliableDataInfo struct {
	joiningMessageLock            sync.Mutex
	joiningMessageFirstSeqs       map[hublive.ParticipantID]uint32
	joiningMessageLastWrittenSeqs map[hublive.ParticipantID]uint32
	lastPubReliableSeq            atomic.Uint32
	stopReliableByMigrateOut      atomic.Bool
	canWriteReliable              bool
	migrateInPubDataCache         atomic.Pointer[MigrationDataCache]
}

// ---------------------------------------------------------------

var _ types.LocalParticipant = (*ParticipantImpl)(nil)

type ParticipantParams struct {
	Identity                hublive.ParticipantIdentity
	Name                    hublive.ParticipantName
	SID                     hublive.ParticipantID
	Config                  *WebRTCConfig
	Sink                    routing.MessageSink
	AudioConfig             sfu.AudioConfig
	VideoConfig             config.VideoConfig
	LimitConfig             config.LimitConfig
	ProtocolVersion         types.ProtocolVersion
	SessionStartTime        time.Time
	TelemetryListener       types.ParticipantTelemetryListener
	Trailer                 []byte
	PLIThrottleConfig       sfu.PLIThrottleConfig
	CongestionControlConfig config.CongestionControlConfig
	// codecs that are enabled for this room
	PublishEnabledCodecs                []*hublive.Codec
	SubscribeEnabledCodecs              []*hublive.Codec
	Logger                              logger.Logger
	LoggerResolver                      logger.DeferredFieldResolver
	Reporter                            roomobs.ParticipantSessionReporter
	ReporterResolver                    roomobs.ParticipantReporterResolver
	SimTracks                           map[uint32]interceptor.SimulcastTrackInfo
	Grants                              *auth.ClaimGrants
	InitialVersion                      uint32
	ClientConf                          *hublive.ClientConfiguration
	ClientInfo                          ClientInfo
	Region                              string
	Migration                           bool
	Reconnect                           bool
	AdaptiveStream                      bool
	AllowTCPFallback                    bool
	TCPFallbackRTTThreshold             int
	AllowUDPUnstableFallback            bool
	TURNSEnabled                        bool
	ParticipantListener                 types.LocalParticipantListener
	ParticipantHelper                   types.LocalParticipantHelper
	DisableSupervisor                   bool
	ReconnectOnPublicationError         bool
	ReconnectOnSubscriptionError        bool
	ReconnectOnDataChannelError         bool
	VersionGenerator                    utils.TimedVersionGenerator
	DisableDynacast                     bool
	SubscriberAllowPause                bool
	SubscriptionLimitAudio              int32
	SubscriptionLimitVideo              int32
	PlayoutDelay                        *hublive.PlayoutDelay
	SyncStreams                         bool
	ForwardStats                        *sfu.ForwardStats
	DisableSenderReportPassThrough      bool
	MetricConfig                        metric.MetricConfig
	UseOneShotSignallingMode            bool
	EnableMetrics                       bool
	DataChannelMaxBufferedAmount        uint64
	DatachannelSlowThreshold            int
	DatachannelLossyTargetLatency       time.Duration
	FireOnTrackBySdp                    bool
	DisableCodecRegression              bool
	LastPubReliableSeq                  uint32
	Country                             string
	PreferVideoSizeFromMedia            bool
	UseSinglePeerConnection             bool
	EnableDataTracks                    bool
	EnableRTPStreamRestartDetection     bool
	ForceBackupCodecPolicySimulcast     bool
	RequireMediaSectionWithJoinResponse bool
	DisableTransceiverReuseForE2EE      bool
}

type ParticipantImpl struct {
	// utils.TimedVersion is a atomic. To be correctly aligned also on 32bit archs
	// 64it atomics need to be at the front of a struct
	timedVersion utils.TimedVersion

	params ParticipantParams

	participantListener atomic.Pointer[types.LocalParticipantListener]
	participantHelper   atomic.Value // types.LocalParticipantHelper
	id                  atomic.Value // types.ParticipantID

	isClosed    atomic.Bool
	closeReason atomic.Value // types.ParticipantCloseReason

	state        atomic.Value // hublive.ParticipantInfo_State
	disconnected chan struct{}

	grants      atomic.Pointer[auth.ClaimGrants]
	isPublisher atomic.Bool

	sessionStartRecorded atomic.Bool
	lastActiveAt         atomic.Pointer[time.Time]
	// when first connected
	connectedAt    time.Time
	disconnectedAt atomic.Pointer[time.Time]
	// timer that's set when disconnect is detected on primary PC
	disconnectTimer *time.Timer
	migrationTimer  *time.Timer

	pubRTCPQueue *sutils.TypedOpsQueue[postRtcpOp]

	// hold reference for MediaTrack
	twcc *twcc.Responder

	// client intended to publish, yet to be reconciled
	pendingTracksLock       utils.RWMutex
	pendingTracks           map[string]*pendingTrackInfo
	pendingPublishingTracks map[hublive.TrackID]*pendingTrackInfo
	pendingRemoteTracks     []*pendingRemoteTrack

	// supported codecs
	enabledPublishCodecs   []*hublive.Codec
	enabledSubscribeCodecs []*hublive.Codec

	*TransportManager
	*UpTrackManager
	*UpDataTrackManager
	*SubscriptionManager

	nextSubscribedDataTrackHandle uint16

	icQueue [2]atomic.Pointer[webrtc.ICECandidate]

	requireBroadcast bool
	// queued participant updates before join response is sent
	// guarded by updateLock
	queuedUpdates []*hublive.ParticipantInfo
	// cache of recently sent updates, to ensure ordering by version
	// guarded by updateLock
	updateCache *lru.Cache[hublive.ParticipantID, participantUpdateInfo]
	updateLock  utils.Mutex

	dataChannelStats *BytesTrackStats

	reliableDataInfo reliableDataInfo

	rttUpdatedAt time.Time
	lastRTT      uint32

	// idempotent reference guard for telemetry stats worker
	telemetryGuard *telemetry.ReferenceGuard

	lock utils.RWMutex

	dirty   atomic.Bool
	version atomic.Uint32

	migrateState                atomic.Value // types.MigrateState
	migratedTracksPublishedFuse core.Fuse

	onClose            map[string]func(types.LocalParticipant)
	onClaimsChanged    func(participant types.LocalParticipant)
	onICEConfigChanged func(participant types.LocalParticipant, iceConfig *hublive.ICEConfig)

	cachedDownTracks map[hublive.TrackID]*downTrackState
	forwarderState   map[hublive.TrackID]*hublive.RTPForwarderState

	supervisor *supervisor.ParticipantSupervisor

	connectionQuality hublive.ConnectionQuality

	metricTimestamper *metric.MetricTimestamper
	metricsCollector  *metric.MetricsCollector
	metricsReporter   *metric.MetricsReporter

	signalling    signalling.ParticipantSignalling
	signalHandler signalling.ParticipantSignalHandler
	signaller     signalling.ParticipantSignaller

	// loggers for publisher and subscriber
	pubLogger logger.Logger
	subLogger logger.Logger

	rpcLock             sync.Mutex
	rpcPendingAcks      map[string]*utils.DataChannelRpcPendingAckHandler
	rpcPendingResponses map[string]*utils.DataChannelRpcPendingResponseHandler
}

func NewParticipant(params ParticipantParams) (*ParticipantImpl, error) {
	if params.Identity == "" {
		return nil, ErrEmptyIdentity
	}
	if params.SID == "" {
		return nil, ErrEmptyParticipantID
	}
	if params.Grants == nil || params.Grants.Video == nil {
		return nil, ErrMissingGrants
	}
	p := &ParticipantImpl{
		params:       params,
		disconnected: make(chan struct{}),
		pubRTCPQueue: sutils.NewTypedOpsQueue[postRtcpOp](sutils.OpsQueueParams{
			Name:    "pub-rtcp",
			MinSize: 64,
			Logger:  params.Logger,
		}),
		pendingTracks:           make(map[string]*pendingTrackInfo),
		pendingPublishingTracks: make(map[hublive.TrackID]*pendingTrackInfo),
		connectedAt:             time.Now().Truncate(time.Millisecond),
		rttUpdatedAt:            time.Now(),
		cachedDownTracks:        make(map[hublive.TrackID]*downTrackState),
		connectionQuality:       hublive.ConnectionQuality_EXCELLENT,
		pubLogger:               params.Logger.WithComponent(sutils.ComponentPub),
		subLogger:               params.Logger.WithComponent(sutils.ComponentSub),
		reliableDataInfo: reliableDataInfo{
			joiningMessageFirstSeqs:       make(map[hublive.ParticipantID]uint32),
			joiningMessageLastWrittenSeqs: make(map[hublive.ParticipantID]uint32),
		},
		rpcPendingAcks:                make(map[string]*utils.DataChannelRpcPendingAckHandler),
		rpcPendingResponses:           make(map[string]*utils.DataChannelRpcPendingResponseHandler),
		onClose:                       make(map[string]func(types.LocalParticipant)),
		telemetryGuard:                &telemetry.ReferenceGuard{},
		nextSubscribedDataTrackHandle: uint16(rand.Intn(256)),
		requireBroadcast:              params.Grants.Metadata != "" || len(params.Grants.Attributes) != 0,
	}
	p.setupSignalling()

	p.id.Store(params.SID)
	p.dataChannelStats = NewBytesTrackStats(
		p.params.Country,
		BytesTrackIDForParticipantID(BytesTrackTypeData, p.ID()),
		p.ID(),
		params.TelemetryListener,
		params.Reporter,
	)
	p.reliableDataInfo.lastPubReliableSeq.Store(params.LastPubReliableSeq)
	p.setListener(params.ParticipantListener)
	p.participantHelper.Store(params.ParticipantHelper)
	if !params.DisableSupervisor {
		p.supervisor = supervisor.NewParticipantSupervisor(supervisor.ParticipantSupervisorParams{Logger: params.Logger})
	}
	p.closeReason.Store(types.ParticipantCloseReasonNone)
	p.version.Store(params.InitialVersion)
	p.timedVersion.Update(params.VersionGenerator.Next())

	p.migrateState.Store(types.MigrateStateInit)

	p.state.Store(hublive.ParticipantInfo_JOINING)
	p.grants.Store(params.Grants.Clone())
	p.SwapResponseSink(params.Sink, types.SignallingCloseReasonUnknown)
	p.setupEnabledCodecs(params.PublishEnabledCodecs, params.SubscribeEnabledCodecs, params.ClientConf.GetDisabledCodecs())

	if p.supervisor != nil {
		p.supervisor.OnPublicationError(p.onPublicationError)
	}

	sessionTimer := observability.NewSessionTimer(p.params.SessionStartTime)
	params.Reporter.RegisterFunc(func(ts time.Time, tx roomobs.ParticipantSessionTx) bool {
		if dts := p.disconnectedAt.Load(); dts != nil {
			ts = *dts
			tx.ReportEndTime(ts)
		}

		millis, mins := sessionTimer.Advance(ts)
		tx.ReportDuration(uint16(millis))
		tx.ReportDurationMinutes(uint8(mins))

		return !p.IsClosed()
	})

	var err error
	// keep last participants and when updates were sent
	if p.updateCache, err = lru.New[hublive.ParticipantID, participantUpdateInfo](128); err != nil {
		return nil, err
	}

	err = p.setupTransportManager()
	if err != nil {
		return nil, err
	}

	p.setupUpTrackManager()
	p.setupUpDataTrackManager()
	p.setupSubscriptionManager()
	p.setupMetrics()

	return p, nil
}

func (p *ParticipantImpl) setListener(listener types.LocalParticipantListener) {
	if listener == nil {
		p.participantListener.Store(nil)
		return
	}
	p.participantListener.Store(&listener)
}

func (p *ParticipantImpl) listener() types.LocalParticipantListener {
	if l := p.participantListener.Load(); l != nil {
		return *l
	}
	return &types.NullLocalParticipantListener{}
}

func (p *ParticipantImpl) GetParticipantListener() types.ParticipantListener {
	return p.listener()
}

func (p *ParticipantImpl) ClearParticipantListener() {
	p.setListener(nil)
}

func (p *ParticipantImpl) GetCountry() string {
	return p.params.Country
}

func (p *ParticipantImpl) GetTrailer() []byte {
	trailer := make([]byte, len(p.params.Trailer))
	copy(trailer, p.params.Trailer)
	return trailer
}

func (p *ParticipantImpl) GetLogger() logger.Logger {
	return p.params.Logger
}

func (p *ParticipantImpl) GetLoggerResolver() logger.DeferredFieldResolver {
	return p.params.LoggerResolver
}

func (p *ParticipantImpl) GetReporter() roomobs.ParticipantSessionReporter {
	return p.params.Reporter
}

func (p *ParticipantImpl) GetReporterResolver() roomobs.ParticipantReporterResolver {
	return p.params.ReporterResolver
}

func (p *ParticipantImpl) GetAdaptiveStream() bool {
	return p.params.AdaptiveStream
}

func (p *ParticipantImpl) GetPacer() pacer.Pacer {
	return p.TransportManager.GetSubscriberPacer()
}

func (p *ParticipantImpl) GetDisableSenderReportPassThrough() bool {
	return p.params.DisableSenderReportPassThrough
}

func (p *ParticipantImpl) ID() hublive.ParticipantID {
	return p.id.Load().(hublive.ParticipantID)
}

func (p *ParticipantImpl) Identity() hublive.ParticipantIdentity {
	return p.params.Identity
}

func (p *ParticipantImpl) State() hublive.ParticipantInfo_State {
	return p.state.Load().(hublive.ParticipantInfo_State)
}

func (p *ParticipantImpl) Kind() hublive.ParticipantInfo_Kind {
	return p.grants.Load().GetParticipantKind()
}

func (p *ParticipantImpl) IsRecorder() bool {
	grants := p.grants.Load()
	return grants.GetParticipantKind() == hublive.ParticipantInfo_EGRESS || grants.Video.Recorder
}

func (p *ParticipantImpl) IsAgent() bool {
	grants := p.grants.Load()
	return grants.GetParticipantKind() == hublive.ParticipantInfo_AGENT || grants.Video.Agent
}

func (p *ParticipantImpl) IsDependent() bool {
	grants := p.grants.Load()
	switch grants.GetParticipantKind() {
	case hublive.ParticipantInfo_AGENT, hublive.ParticipantInfo_EGRESS:
		return true
	default:
		return grants.Video.Agent || grants.Video.Recorder
	}
}

func (p *ParticipantImpl) ProtocolVersion() types.ProtocolVersion {
	return p.params.ProtocolVersion
}

func (p *ParticipantImpl) IsReady() bool {
	state := p.State()

	// when migrating, there is no JoinResponse, state transitions from JOINING -> ACTIVE -> DISCONNECTED
	// so JOINING is considered ready.
	if p.params.Migration {
		return state != hublive.ParticipantInfo_DISCONNECTED
	}

	// when not migrating, there is a JoinResponse, state transitions from JOINING -> JOINED -> ACTIVE -> DISCONNECTED
	return state == hublive.ParticipantInfo_JOINED || state == hublive.ParticipantInfo_ACTIVE
}

func (p *ParticipantImpl) IsDisconnected() bool {
	return p.State() == hublive.ParticipantInfo_DISCONNECTED
}

func (p *ParticipantImpl) Disconnected() <-chan struct{} {
	return p.disconnected
}

func (p *ParticipantImpl) IsIdle() bool {
	// check if there are any published tracks that are subscribed
	for _, t := range p.GetPublishedTracks() {
		if t.GetNumSubscribers() > 0 {
			return false
		}
	}

	return !p.SubscriptionManager.HasSubscriptions()
}

func (p *ParticipantImpl) ConnectedAt() time.Time {
	return p.connectedAt
}

func (p *ParticipantImpl) ActiveAt() time.Time {
	if activeAt := p.lastActiveAt.Load(); activeAt != nil {
		return *activeAt
	}

	return time.Time{}
}

func (p *ParticipantImpl) GetClientInfo() *hublive.ClientInfo {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.params.ClientInfo.ClientInfo
}

func (p *ParticipantImpl) GetClientConfiguration() *hublive.ClientConfiguration {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return utils.CloneProto(p.params.ClientConf)
}

func (p *ParticipantImpl) GetBufferFactory() *buffer.Factory {
	return p.params.Config.BufferFactory
}

// checkMetadataLimits check if name/metadata/attributes of a participant is within configured limits
func (p *ParticipantImpl) checkMetadataLimits(
	name string,
	metadata string,
	attributes map[string]string,
) error {
	if !p.params.LimitConfig.CheckParticipantNameLength(name) {
		return signalling.ErrNameExceedsLimits
	}

	if !p.params.LimitConfig.CheckMetadataSize(metadata) {
		return signalling.ErrMetadataExceedsLimits
	}

	if !p.params.LimitConfig.CheckAttributesSize(attributes) {
		return signalling.ErrAttributesExceedsLimits
	}

	return nil
}

func (p *ParticipantImpl) UpdateMetadata(update *hublive.UpdateParticipantMetadata, fromAdmin bool) error {
	lgr := p.params.Logger.WithUnlikelyValues(
		"update", logger.Proto(update),
		"fromAdmin", fromAdmin,
	)
	lgr.Debugw("updating participant metadata")

	var err error
	requestResponse := &hublive.RequestResponse{
		RequestId: update.RequestId,
	}
	sendRequestResponse := func() error {
		if !fromAdmin || (update.RequestId != 0 || err != nil) {
			requestResponse.Request = &hublive.RequestResponse_UpdateMetadata{
				UpdateMetadata: utils.CloneProto(update),
			}
			p.sendRequestResponse(requestResponse)
		}
		if err != nil {
			lgr.Warnw("could not update metadata", err)
		}
		return err
	}

	if !fromAdmin && !p.ClaimGrants().Video.GetCanUpdateOwnMetadata() {
		requestResponse.Reason = hublive.RequestResponse_NOT_ALLOWED
		requestResponse.Message = "does not have permission to update own metadata"
		err = signalling.ErrUpdateOwnMetadataNotAllowed
		return sendRequestResponse()
	}

	if err = p.checkMetadataLimits(update.Name, update.Metadata, update.Attributes); err != nil {
		switch err {
		case signalling.ErrNameExceedsLimits:
			requestResponse.Reason = hublive.RequestResponse_LIMIT_EXCEEDED
			requestResponse.Message = "exceeds name length limit"

		case signalling.ErrMetadataExceedsLimits:
			requestResponse.Reason = hublive.RequestResponse_LIMIT_EXCEEDED
			requestResponse.Message = "exceeds metadata size limit"

		case signalling.ErrAttributesExceedsLimits:
			requestResponse.Reason = hublive.RequestResponse_LIMIT_EXCEEDED
			requestResponse.Message = "exceeds attributes size limit"
		}
		return sendRequestResponse()
	}

	if update.Name != "" {
		p.SetName(update.Name)
	}
	if update.Metadata != "" {
		p.SetMetadata(update.Metadata)
	}
	if update.Attributes != nil {
		p.SetAttributes(update.Attributes)
	}
	return sendRequestResponse()
}

// SetName attaches name to the participant
func (p *ParticipantImpl) SetName(name string) {
	p.lock.Lock()
	grants := p.grants.Load()
	if grants.Name == name {
		p.lock.Unlock()
		return
	}

	grants = grants.Clone()
	grants.Name = name
	p.grants.Store(grants)
	p.dirty.Store(true)

	onClaimsChanged := p.onClaimsChanged
	p.lock.Unlock()

	p.listener().OnParticipantUpdate(p)

	if onClaimsChanged != nil {
		onClaimsChanged(p)
	}
}

// SetMetadata attaches metadata to the participant
func (p *ParticipantImpl) SetMetadata(metadata string) {
	p.lock.Lock()
	grants := p.grants.Load()
	if grants.Metadata == metadata {
		p.lock.Unlock()
		return
	}

	grants = grants.Clone()
	grants.Metadata = metadata
	p.grants.Store(grants)
	p.requireBroadcast = p.requireBroadcast || metadata != ""
	p.dirty.Store(true)

	onClaimsChanged := p.onClaimsChanged
	p.lock.Unlock()

	p.listener().OnParticipantUpdate(p)

	if onClaimsChanged != nil {
		onClaimsChanged(p)
	}
}

func (p *ParticipantImpl) SetAttributes(attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}
	p.lock.Lock()
	grants := p.grants.Load().Clone()
	if grants.Attributes == nil {
		grants.Attributes = make(map[string]string)
	}
	var keysToDelete []string
	for k, v := range attrs {
		if v == "" {
			keysToDelete = append(keysToDelete, k)
		} else {
			grants.Attributes[k] = v
		}
	}
	for _, k := range keysToDelete {
		delete(grants.Attributes, k)
	}

	p.grants.Store(grants)
	p.requireBroadcast = true // already checked above
	p.dirty.Store(true)

	onClaimsChanged := p.onClaimsChanged
	p.lock.Unlock()

	p.listener().OnParticipantUpdate(p)

	if onClaimsChanged != nil {
		onClaimsChanged(p)
	}
}

func (p *ParticipantImpl) ClaimGrants() *auth.ClaimGrants {
	return p.grants.Load()
}

func (p *ParticipantImpl) SetPermission(permission *hublive.ParticipantPermission) bool {
	if permission == nil {
		return false
	}
	p.lock.Lock()
	grants := p.grants.Load()

	if grants.Video.MatchesPermission(permission) {
		p.lock.Unlock()
		return false
	}

	p.params.Logger.Infow("updating participant permission", "permission", permission)

	grants = grants.Clone()
	grants.Video.UpdateFromPermission(permission)
	p.grants.Store(grants)
	p.dirty.Store(true)

	canPublish := grants.Video.GetCanPublish()
	canSubscribe := grants.Video.GetCanSubscribe()

	onClaimsChanged := p.onClaimsChanged

	isPublisher := canPublish && p.TransportManager.IsPublisherEstablished()
	p.requireBroadcast = p.requireBroadcast || isPublisher
	p.lock.Unlock()

	// publish permission has been revoked then remove offending tracks
	for _, track := range p.GetPublishedTracks() {
		if !grants.Video.GetCanPublishSource(track.Source()) {
			p.removePublishedTrack(track)
		}
	}

	if canSubscribe {
		// reconcile everything
		p.SubscriptionManager.ReconcileAll()
	} else {
		// revoke all subscriptions
		for _, st := range p.SubscriptionManager.GetSubscribedTracks() {
			st.MediaTrack().RemoveSubscriber(p.ID(), false)
		}
	}

	if !grants.Video.GetCanPublishData() {
		for _, dt := range p.UpDataTrackManager.GetPublishedDataTracks() {
			p.UpDataTrackManager.RemovePublishedDataTrack(dt)
		}
	}

	// update isPublisher attribute
	p.isPublisher.Store(isPublisher)

	p.listener().OnParticipantUpdate(p)

	if onClaimsChanged != nil {
		onClaimsChanged(p)
	}
	return true
}

func (p *ParticipantImpl) CanSkipBroadcast() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return !p.requireBroadcast
}

func (p *ParticipantImpl) maybeIncVersion() {
	if p.dirty.Load() {
		p.lock.Lock()
		if p.dirty.Swap(false) {
			p.version.Inc()
			p.timedVersion.Update(p.params.VersionGenerator.Next())
		}
		p.lock.Unlock()
	}
}

func (p *ParticipantImpl) Version() utils.TimedVersion {
	p.maybeIncVersion()

	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.timedVersion
}

func (p *ParticipantImpl) ToProtoWithVersion() (*hublive.ParticipantInfo, utils.TimedVersion) {
	p.maybeIncVersion()

	p.lock.RLock()
	grants := p.grants.Load()
	v := p.version.Load()
	piv := p.timedVersion

	var clientProtocol int32
	if p.params.ClientInfo.ClientInfo != nil {
		clientProtocol = p.params.ClientInfo.ClientInfo.GetClientProtocol()
	}
	pi := &hublive.ParticipantInfo{
		Sid:              string(p.ID()),
		Identity:         string(p.params.Identity),
		Name:             grants.Name,
		State:            p.State(),
		JoinedAt:         p.ConnectedAt().Unix(),
		JoinedAtMs:       p.ConnectedAt().UnixMilli(),
		Version:          v,
		Permission:       grants.Video.ToPermission(),
		Metadata:         grants.Metadata,
		Attributes:       grants.Attributes,
		Region:           p.params.Region,
		IsPublisher:      p.IsPublisher(),
		Kind:             grants.GetParticipantKind(),
		KindDetails:      grants.GetKindDetails(),
		DisconnectReason: p.CloseReason().ToDisconnectReason(),
		ClientProtocol:   clientProtocol,
	}
	p.lock.RUnlock()

	p.pendingTracksLock.RLock()
	pi.Tracks = p.UpTrackManager.ToProto()

	// add any pending migrating tracks, else an update could delete/unsubscribe from yet to be published, migrating tracks
	maybeAdd := func(pti *pendingTrackInfo) {
		if !pti.migrated {
			return
		}

		found := false
		for _, ti := range pi.Tracks {
			if ti.Sid == pti.trackInfos[0].Sid {
				found = true
				break
			}
		}

		if !found {
			pi.Tracks = append(pi.Tracks, utils.CloneProto(pti.trackInfos[0]))
		}
	}

	for _, pt := range p.pendingTracks {
		maybeAdd(pt)
	}
	for _, ppt := range p.pendingPublishingTracks {
		maybeAdd(ppt)
	}
	p.pendingTracksLock.RUnlock()

	pi.DataTracks = p.UpDataTrackManager.ToProto()

	return pi, piv
}

func (p *ParticipantImpl) ToProto() *hublive.ParticipantInfo {
	pi, _ := p.ToProtoWithVersion()
	return pi
}

func (p *ParticipantImpl) TelemetryGuard() *telemetry.ReferenceGuard {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.telemetryGuard
}

func (p *ParticipantImpl) GetTelemetryListener() types.ParticipantTelemetryListener {
	if p.params.TelemetryListener == nil {
		return &types.NullParticipantTelemetryListener{}
	}

	return p.params.TelemetryListener
}

func (p *ParticipantImpl) AddOnClose(key string, callback func(types.LocalParticipant)) {
	if p.isClosed.Load() {
		go callback(p)
		return
	}

	p.lock.Lock()
	if callback == nil {
		delete(p.onClose, key)
	} else {
		p.onClose[key] = callback
	}
	p.lock.Unlock()
}

func (p *ParticipantImpl) OnClaimsChanged(callback func(types.LocalParticipant)) {
	p.lock.Lock()
	p.onClaimsChanged = callback
	p.lock.Unlock()
}

func (p *ParticipantImpl) HandleSignalSourceClose() {
	p.TransportManager.SetSignalSourceValid(false)

	if !p.HasConnected() {
		_ = p.Close(false, types.ParticipantCloseReasonSignalSourceClose, false)
	}
}

func (p *ParticipantImpl) HandleICETrickle(trickleRequest *hublive.TrickleRequest) {
	candidateInit, err := protosignalling.FromProtoTrickle(trickleRequest)
	if err != nil {
		p.params.Logger.Warnw("could not decode trickle", err)
		p.sendRequestResponse(&hublive.RequestResponse{
			Reason:  hublive.RequestResponse_UNCLASSIFIED_ERROR,
			Message: err.Error(),
			Request: &hublive.RequestResponse_Trickle{
				Trickle: utils.CloneProto(trickleRequest),
			},
		})
		return
	}

	p.TransportManager.AddICECandidate(candidateInit, trickleRequest.Target)
}

func (p *ParticipantImpl) GetAnswer() (webrtc.SessionDescription, uint32, error) {
	if p.IsClosed() || p.IsDisconnected() {
		return webrtc.SessionDescription{}, 0, ErrParticipantSessionClosed
	}

	answer, answerId, err := p.TransportManager.GetAnswer()
	if err != nil {
		return answer, answerId, err
	}

	answer = p.configurePublisherAnswer(answer)
	p.pubLogger.Debugw(
		"returning answer",
		"transport", hublive.SignalTarget_PUBLISHER,
		"answer", answer,
		"answerId", answerId,
	)
	return answer, answerId, nil
}

// HandleAnswer handles a client answer response, with subscriber PC, server initiates the
// offer and client answers
func (p *ParticipantImpl) HandleAnswer(sd *hublive.SessionDescription) {
	answer, answerId, _ := protosignalling.FromProtoSessionDescription(sd)
	p.subLogger.Debugw(
		"received answer",
		"transport", hublive.SignalTarget_SUBSCRIBER,
		"answer", answer,
		"answerId", answerId,
	)

	/* from server received join request to client answer
	 * 1. server send join response & offer
	 * ... swap candidates
	 * 2. client send answer
	 */
	signalConnCost := time.Since(p.ConnectedAt()).Milliseconds()
	p.TransportManager.UpdateSignalingRTT(uint32(signalConnCost))

	p.TransportManager.HandleAnswer(answer, answerId)
}

func (p *ParticipantImpl) SetMigrateInfo(
	previousOffer, previousAnswer *webrtc.SessionDescription,
	mediaTracks []*hublive.TrackPublishedResponse,
	dataChannels []*hublive.DataChannelInfo,
	dataChannelReceiveState []*hublive.DataChannelReceiveState,
	dataTracks []*hublive.PublishDataTrackResponse,
) {
	p.pendingTracksLock.Lock()
	for _, t := range mediaTracks {
		ti := t.GetTrack()

		if p.supervisor != nil {
			p.supervisor.AddPublication(hublive.TrackID(ti.Sid))
			p.supervisor.SetPublicationMute(hublive.TrackID(ti.Sid), ti.Muted)
		}

		p.pendingTracks[t.GetCid()] = &pendingTrackInfo{
			trackInfos: []*hublive.TrackInfo{ti},
			migrated:   true,
			createdAt:  time.Now(),
		}
		p.pubLogger.Infow(
			"pending track added (migration)",
			"trackID", ti.Sid,
			"cid", t.GetCid(),
			"pendingTrack", p.pendingTracks[t.GetCid()],
		)
	}
	p.pendingTracksLock.Unlock()

	for _, t := range dataTracks {
		dti := t.GetInfo()
		dt := NewDataTrack(
			DataTrackParams{
				Logger:              p.params.Logger.WithValues("trackID", dti.Sid),
				ParticipantID:       p.ID,
				ParticipantIdentity: p.params.Identity,
			},
			dti,
		)
		p.UpDataTrackManager.AddPublishedDataTrack(dt)
	}

	if len(mediaTracks) != 0 || len(dataTracks) != 0 {
		p.setIsPublisher(true)
	}

	p.reliableDataInfo.joiningMessageLock.Lock()
	for _, state := range dataChannelReceiveState {
		p.reliableDataInfo.joiningMessageFirstSeqs[hublive.ParticipantID(state.PublisherSid)] = state.LastSeq + 1
	}
	p.reliableDataInfo.joiningMessageLock.Unlock()

	p.TransportManager.SetMigrateInfo(previousOffer, previousAnswer, dataChannels)
}

func (p *ParticipantImpl) IsReconnect() bool {
	return p.params.Reconnect
}

func (p *ParticipantImpl) Close(sendLeave bool, reason types.ParticipantCloseReason, isExpectedToResume bool) error {
	if p.isClosed.Swap(true) {
		// already closed
		return nil
	}

	var sessionDuration time.Duration
	if activeAt := p.ActiveAt(); !activeAt.IsZero() {
		sessionDuration = time.Since(activeAt)
	}
	p.params.Logger.Infow(
		"participant closing",
		"sendLeave", sendLeave,
		"reason", reason.String(),
		"isExpectedToResume", isExpectedToResume,
		"clientInfo", logger.Proto(sutils.ClientInfoWithoutAddress(p.GetClientInfo())),
		"kind", p.Kind(),
		"sessionDuration", sessionDuration,
	)
	p.closeReason.Store(reason)
	p.clearDisconnectTimer()
	p.clearMigrationTimer()

	if sendLeave {
		p.sendLeaveRequest(
			reason,
			isExpectedToResume,
			false, // isExpectedToReconnect
			false, // sendOnlyIfSupportingLeaveRequestWithAction
		)
	}

	if p.supervisor != nil {
		p.supervisor.Stop()
	}

	p.pendingTracksLock.Lock()
	for _, pti := range p.pendingTracks {
		if len(pti.trackInfos) == 0 {
			continue
		}
		prometheus.RecordTrackPublishCancels(pti.trackInfos[0].Type.String(), int32(len(pti.trackInfos)))
	}
	p.pendingTracks = make(map[string]*pendingTrackInfo)
	p.pendingPublishingTracks = make(map[hublive.TrackID]*pendingTrackInfo)
	p.pendingTracksLock.Unlock()

	p.UpTrackManager.Close(isExpectedToResume)

	p.rpcLock.Lock()
	clear(p.rpcPendingAcks)
	for _, handler := range p.rpcPendingResponses {
		handler.Resolve("", utils.DataChannelRpcErrorFromBuiltInCodes(utils.DataChannelRpcRecipientDisconnected, ""))
	}
	p.rpcPendingResponses = make(map[string]*utils.DataChannelRpcPendingResponseHandler)
	p.rpcLock.Unlock()

	p.updateState(hublive.ParticipantInfo_DISCONNECTED)
	close(p.disconnected)

	// ensure this is synchronized
	p.CloseSignalConnection(types.SignallingCloseReasonParticipantClose)
	p.lock.RLock()
	onClose := maps.Values(p.onClose)
	p.lock.RUnlock()
	for _, cb := range onClose {
		cb(p)
	}

	// Close peer connections without blocking participant Close. If peer connections are gathering candidates
	// Close will block.
	go func() {
		p.SubscriptionManager.Close(isExpectedToResume)
		p.TransportManager.Close()

		p.metricsCollector.Stop()
		p.metricsReporter.Stop()
	}()

	p.dataChannelStats.Stop()
	return nil
}

func (p *ParticipantImpl) IsClosed() bool {
	return p.isClosed.Load()
}

func (p *ParticipantImpl) CloseReason() types.ParticipantCloseReason {
	return p.closeReason.Load().(types.ParticipantCloseReason)
}

// Negotiate subscriber SDP with client, if force is true, will cancel pending
// negotiate task and negotiate immediately
func (p *ParticipantImpl) Negotiate(force bool) {
	if p.params.UseOneShotSignallingMode {
		return
	}

	if p.MigrateState() != types.MigrateStateInit {
		p.TransportManager.NegotiateSubscriber(force)
	}
}

func (p *ParticipantImpl) clearMigrationTimer() {
	p.lock.Lock()
	if p.migrationTimer != nil {
		p.migrationTimer.Stop()
		p.migrationTimer = nil
	}
	p.lock.Unlock()
}

func (p *ParticipantImpl) setupMigrationTimerLocked() {
	if p.params.UseSinglePeerConnection {
		return
	}

	//
	// On subscriber peer connection, remote side will try ICE on both
	// pre- and post-migration ICE candidates as the migrating out
	// peer connection leaves itself open to enable transition of
	// media with as less disruption as possible.
	//
	// But, sometimes clients could delay the migration because of
	// pinging the incorrect ICE candidates. Give the remote some time
	// to try and succeed. If not, close the subscriber peer connection
	// and help the remote side to narrow down its ICE candidate pool.
	//
	p.migrationTimer = time.AfterFunc(migrationWaitDuration, func() {
		p.clearMigrationTimer()

		if p.IsClosed() || p.IsDisconnected() {
			return
		}
		p.subLogger.Debugw("closing peer connection(s) to aid migration")

		//
		// Close all down tracks before closing subscriber peer connection.
		// Closing subscriber peer connection will call `Unbind` on all down tracks.
		// DownTrack close has checks to handle the case of closing before bind.
		// So, an `Unbind` before close would bypass that logic.
		//
		p.SubscriptionManager.Close(true)

		p.TransportManager.Close()
	})
}

func (p *ParticipantImpl) MaybeStartMigration(force bool, onStart func()) bool {
	if p.IsClosed() || p.params.UseOneShotSignallingMode {
		return false
	}

	allTransportConnected := p.TransportManager.HasSubscriberEverConnected()
	if p.IsPublisher() {
		allTransportConnected = allTransportConnected && p.TransportManager.HasPublisherEverConnected()
	}
	if !force && !allTransportConnected {
		return false
	}

	if onStart != nil {
		onStart()
	}

	p.sendLeaveRequest(
		types.ParticipantCloseReasonMigrationRequested,
		true,  // isExpectedToResume
		false, // isExpectedToReconnect
		true,  // sendOnlyIfSupportingLeaveRequestWithAction
	)
	p.CloseSignalConnection(types.SignallingCloseReasonMigration)

	p.clearMigrationTimer()

	p.lock.Lock()
	p.setupMigrationTimerLocked()
	p.lock.Unlock()

	return true
}

func (p *ParticipantImpl) NotifyMigration() {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.migrationTimer != nil {
		// already set up
		return
	}

	p.setupMigrationTimerLocked()
}

func (p *ParticipantImpl) SetMigrateState(s types.MigrateState) {
	preState := p.MigrateState()
	if preState == types.MigrateStateComplete || preState == s {
		return
	}

	p.params.Logger.Debugw("SetMigrateState", "state", s)
	var migratedTracks []*MediaTrack
	if s == types.MigrateStateComplete {
		migratedTracks = p.handleMigrateTracks()
	}
	p.migrateState.Store(s)
	p.dirty.Store(true)

	switch s {
	case types.MigrateStateSync:
		p.TransportManager.ProcessPendingPublisherOffer()

	case types.MigrateStateComplete:
		if preState == types.MigrateStateSync {
			p.params.Logger.Infow("migration complete")

			if p.params.LastPubReliableSeq > 0 {
				p.reliableDataInfo.migrateInPubDataCache.Store(NewMigrationDataCache(p.params.LastPubReliableSeq, time.Now().Add(migrationWaitContinuousMsgDuration)))
			}
		}
		p.TransportManager.ProcessPendingPublisherDataChannels()
		go p.cacheForwarderState()
	}

	go func() {
		// launch callbacks in goroutine since they could block.
		// callbacks handle webhooks as well as db persistence
		for _, t := range migratedTracks {
			p.handleTrackPublished(t, true)
		}

		if s == types.MigrateStateComplete {
			// wait for all migrated track to be published,
			// it is possible that synthesized track publish above could
			// race with actual publish from client and the above synthesized
			// one could actually be a no-op because the actual publish path is active.
			//
			// if the actual publish path has not finished, the migration state change
			// callback could close the remote participant/tracks before the local track
			// is fully active.
			//
			// that could lead subscribers to unsubscribe due to source
			// track going away, i. e. in this case, the remote track close would have
			// notified the subscription manager, the subscription manager would
			// re-resolve to check if the track is still active and unsubscribe if none
			// is active, as local track is in the process of completing publish,
			// the check would have resolved to an empty track leading to unsubscription.
			go func() {
				startTime := time.Now()
				for {
					if !p.hasPendingMigratedTrack() || p.IsDisconnected() || time.Since(startTime) > 15*time.Second {
						// a time out just to be safe, but it should not be needed
						p.migratedTracksPublishedFuse.Break()
						return
					}

					time.Sleep(20 * time.Millisecond)
				}
			}()

			<-p.migratedTracksPublishedFuse.Watch()
		}

		p.listener().OnMigrateStateChange(p, s)
	}()
}

func (p *ParticipantImpl) MigrateState() types.MigrateState {
	return p.migrateState.Load().(types.MigrateState)
}

// ICERestart restarts subscriber ICE connections
func (p *ParticipantImpl) ICERestart(iceConfig *hublive.ICEConfig) {
	if p.params.UseOneShotSignallingMode {
		return
	}

	p.clearDisconnectTimer()
	p.clearMigrationTimer()

	for _, t := range p.GetPublishedTracks() {
		t.(types.LocalMediaTrack).Restart()
	}

	if err := p.TransportManager.ICERestart(iceConfig); err != nil {
		p.IssueFullReconnect(types.ParticipantCloseReasonNegotiateFailed)
	}
}

func (p *ParticipantImpl) OnICEConfigChanged(f func(participant types.LocalParticipant, iceConfig *hublive.ICEConfig)) {
	p.lock.Lock()
	p.onICEConfigChanged = f
	p.lock.Unlock()
}

func (p *ParticipantImpl) GetConnectionQuality() *hublive.ConnectionQualityInfo {
	minQuality := hublive.ConnectionQuality_EXCELLENT
	minScore := connectionquality.MaxMOS

	for _, pt := range p.GetPublishedTracks() {
		score, quality := pt.(types.LocalMediaTrack).GetConnectionScoreAndQuality()
		if utils.IsConnectionQualityLower(minQuality, quality) {
			minQuality = quality
			minScore = score
		} else if quality == minQuality && score < minScore {
			minScore = score
		}
	}

	subscribedTracks := p.SubscriptionManager.GetSubscribedTracks()
	for _, subTrack := range subscribedTracks {
		score, quality := subTrack.DownTrack().GetConnectionScoreAndQuality()
		if utils.IsConnectionQualityLower(minQuality, quality) {
			minQuality = quality
			minScore = score
		} else if quality == minQuality && score < minScore {
			minScore = score
		}
	}

	prometheus.RecordQuality(minQuality, minScore)

	if minQuality == hublive.ConnectionQuality_LOST && !p.ProtocolVersion().SupportsConnectionQualityLost() {
		minQuality = hublive.ConnectionQuality_POOR
	}

	p.lock.Lock()
	if minQuality != p.connectionQuality {
		p.params.Logger.Debugw("connection quality changed", "from", p.connectionQuality, "to", minQuality)
	}
	p.connectionQuality = minQuality
	p.lock.Unlock()

	return &hublive.ConnectionQualityInfo{
		ParticipantSid: string(p.ID()),
		Quality:        minQuality,
		Score:          minScore,
	}
}

func (p *ParticipantImpl) CanPublish() bool {
	return p.grants.Load().Video.GetCanPublish()
}

func (p *ParticipantImpl) CanPublishSource(source hublive.TrackSource) bool {
	return p.grants.Load().Video.GetCanPublishSource(source)
}

func (p *ParticipantImpl) CanSubscribe() bool {
	return p.grants.Load().Video.GetCanSubscribe()
}

func (p *ParticipantImpl) CanPublishData() bool {
	return p.grants.Load().Video.GetCanPublishData()
}

func (p *ParticipantImpl) Hidden() bool {
	return p.grants.Load().Video.Hidden
}

func (p *ParticipantImpl) CanSubscribeMetrics() bool {
	return p.grants.Load().Video.GetCanSubscribeMetrics()
}

func (p *ParticipantImpl) UpdateMediaRTT(rtt uint32) {
	now := time.Now()
	p.lock.Lock()
	if now.Sub(p.rttUpdatedAt) < rttUpdateInterval || p.lastRTT == rtt {
		p.lock.Unlock()
		return
	}
	p.rttUpdatedAt = now
	p.lastRTT = rtt
	p.lock.Unlock()
	p.TransportManager.UpdateMediaRTT(rtt)

	for _, pt := range p.GetPublishedTracks() {
		pt.(types.LocalMediaTrack).SetRTT(rtt)
	}
}

// ----------------------------------------------------------

var _ transport.Handler = (*AnyTransportHandler)(nil)

type AnyTransportHandler struct {
	transport.UnimplementedHandler
	p *ParticipantImpl
}

func (h AnyTransportHandler) OnFailed(_isShortLived bool, _ici *types.ICEConnectionInfo) {
	h.p.onAnyTransportFailed()
}

func (h AnyTransportHandler) OnNegotiationFailed() {
	h.p.onAnyTransportNegotiationFailed()
}

func (h AnyTransportHandler) OnICECandidate(c *webrtc.ICECandidate, target hublive.SignalTarget) error {
	return h.p.onICECandidate(c, target)
}

// ----------------------------------------------------------

type PublisherTransportHandler struct {
	AnyTransportHandler
}

func (h PublisherTransportHandler) OnSetRemoteDescriptionOffer() {
	h.p.onPublisherSetRemoteDescription()
}

func (h PublisherTransportHandler) OnAnswer(sd webrtc.SessionDescription, answerId uint32, midToTrackID map[string]string) error {
	return h.p.onPublisherAnswer(sd, answerId, midToTrackID)
}

func (h PublisherTransportHandler) OnTrack(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
	h.p.onMediaTrack(track, rtpReceiver)
}

func (h PublisherTransportHandler) OnInitialConnected() {
	h.p.onPublisherInitialConnected()
}

func (h PublisherTransportHandler) OnDataMessage(kind hublive.DataPacket_Kind, data []byte) {
	h.p.onReceivedDataMessage(kind, data)
}

func (h PublisherTransportHandler) OnDataMessageUnlabeled(data []byte) {
	h.p.onReceivedDataMessageUnlabeled(data)
}

func (h PublisherTransportHandler) OnDataTrackMessage(data []byte, arrivalTime int64) {
	h.p.onReceivedDataTrackMessage(data, arrivalTime)
}

func (h PublisherTransportHandler) OnDataSendError(err error) {
	h.p.onDataSendError(err)
}

func (h PublisherTransportHandler) OnUnmatchedMedia(numAudios uint32, numVideos uint32) error {
	return h.p.sendMediaSectionsRequirement(numAudios, numVideos)
}

// ----------------------------------------------------------

type SubscriberTransportHandler struct {
	AnyTransportHandler
}

func (h SubscriberTransportHandler) OnOffer(sd webrtc.SessionDescription, offerId uint32, midToTrackID map[string]string) error {
	return h.p.onSubscriberOffer(sd, offerId, midToTrackID)
}

func (h SubscriberTransportHandler) OnStreamStateChange(update *streamallocator.StreamStateUpdate) error {
	return h.p.onStreamStateChange(update)
}

func (h SubscriberTransportHandler) OnInitialConnected() {
	h.p.onSubscriberInitialConnected()
}

func (h SubscriberTransportHandler) OnDataSendError(err error) {
	h.p.onDataSendError(err)
}

// ----------------------------------------------------------

type PrimaryTransportHandler struct {
	transport.Handler
	p *ParticipantImpl
}

func (h PrimaryTransportHandler) OnInitialConnected() {
	h.Handler.OnInitialConnected()
	h.p.onPrimaryTransportInitialConnected()
}

func (h PrimaryTransportHandler) OnFullyEstablished() {
	h.p.onPrimaryTransportFullyEstablished()
}

// ----------------------------------------------------------

func (p *ParticipantImpl) setupSignalling() {
	p.signalling = signalling.NewSignalling(signalling.SignallingParams{
		Logger: p.params.Logger,
	})
	p.signalHandler = signalling.NewSignalHandler(signalling.SignalHandlerParams{
		Logger:      p.params.Logger,
		Participant: p,
	})
	p.signaller = signalling.NewSignallerAsync(signalling.SignallerAsyncParams{
		Logger:      p.params.Logger,
		Participant: p,
	})
}

func (p *ParticipantImpl) setupTransportManager() error {
	p.twcc = twcc.NewTransportWideCCResponder()
	p.twcc.OnFeedback(func(pkts []rtcp.Packet) {
		p.postRtcp(pkts)
	})
	ath := AnyTransportHandler{p: p}
	var pth transport.Handler = PublisherTransportHandler{ath}
	var sth transport.Handler = SubscriberTransportHandler{ath}

	subscriberAsPrimary := !p.params.UseOneShotSignallingMode && (p.ProtocolVersion().SubscriberAsPrimary() && p.CanSubscribe()) && !p.params.UseSinglePeerConnection
	if subscriberAsPrimary {
		sth = PrimaryTransportHandler{sth, p}
	} else {
		pth = PrimaryTransportHandler{pth, p}
	}

	params := TransportManagerParams{
		// primary connection does not change, canSubscribe can change if permission was updated
		// after the participant has joined
		SubscriberAsPrimary:           subscriberAsPrimary,
		UseSinglePeerConnection:       p.params.UseSinglePeerConnection,
		Config:                        p.params.Config,
		Twcc:                          p.twcc,
		ProtocolVersion:               p.params.ProtocolVersion,
		CongestionControlConfig:       p.params.CongestionControlConfig,
		EnabledPublishCodecs:          p.enabledPublishCodecs,
		EnabledSubscribeCodecs:        p.enabledSubscribeCodecs,
		SimTracks:                     p.params.SimTracks,
		ClientInfo:                    p.params.ClientInfo,
		Migration:                     p.params.Migration,
		AllowTCPFallback:              p.params.AllowTCPFallback,
		TCPFallbackRTTThreshold:       p.params.TCPFallbackRTTThreshold,
		AllowUDPUnstableFallback:      p.params.AllowUDPUnstableFallback,
		TURNSEnabled:                  p.params.TURNSEnabled,
		AllowPlayoutDelay:             p.params.PlayoutDelay.GetEnabled(),
		DataChannelMaxBufferedAmount:  p.params.DataChannelMaxBufferedAmount,
		DatachannelSlowThreshold:      p.params.DatachannelSlowThreshold,
		DatachannelLossyTargetLatency: p.params.DatachannelLossyTargetLatency,
		Logger:                        p.params.Logger.WithComponent(sutils.ComponentTransport),
		PublisherHandler:              pth,
		SubscriberHandler:             sth,
		DataChannelStats:              p.dataChannelStats,
		UseOneShotSignallingMode:      p.params.UseOneShotSignallingMode,
		FireOnTrackBySdp:              p.params.FireOnTrackBySdp,
		EnableDataTracks:              p.params.EnableDataTracks,
	}
	if p.params.SyncStreams && p.params.PlayoutDelay.GetEnabled() && p.params.ClientInfo.isFirefox() {
		// we will disable playout delay for Firefox if the user is expecting
		// the streams to be synced. Firefox doesn't support SyncStreams
		params.AllowPlayoutDelay = false
	}
	tm, err := NewTransportManager(params)
	if err != nil {
		return err
	}

	tm.OnICEConfigChanged(func(iceConfig *hublive.ICEConfig) {
		p.lock.Lock()
		onICEConfigChanged := p.onICEConfigChanged

		if p.params.ClientConf == nil {
			p.params.ClientConf = &hublive.ClientConfiguration{}
		}
		if iceConfig.PreferenceSubscriber == hublive.ICECandidateType_ICT_TLS {
			p.params.ClientConf.ForceRelay = hublive.ClientConfigSetting_ENABLED
		} else {
			// UNSET indicates that clients could override RTCConfiguration to forceRelay
			p.params.ClientConf.ForceRelay = hublive.ClientConfigSetting_UNSET
		}
		p.lock.Unlock()

		if onICEConfigChanged != nil {
			onICEConfigChanged(p, iceConfig)
		}
	})

	tm.SetSubscriberAllowPause(p.params.SubscriberAllowPause)
	p.TransportManager = tm
	return nil
}

func (p *ParticipantImpl) setupUpTrackManager() {
	p.UpTrackManager = NewUpTrackManager(UpTrackManagerParams{
		Logger:           p.pubLogger,
		VersionGenerator: p.params.VersionGenerator,
	})

	p.UpTrackManager.OnPublishedTrackUpdated(func(track types.MediaTrack) {
		p.dirty.Store(true)
		p.listener().OnTrackUpdated(p, track)
	})

	p.UpTrackManager.OnUpTrackManagerClose(p.onUpTrackManagerClose)
}

func (p *ParticipantImpl) setupUpDataTrackManager() {
	p.UpDataTrackManager = NewUpDataTrackManager(UpDataTrackManagerParams{
		Logger:      p.pubLogger,
		Participant: p,
	})
}

func (p *ParticipantImpl) setupSubscriptionManager() {
	p.SubscriptionManager = NewSubscriptionManager(SubscriptionManagerParams{
		Participant: p,
		Logger:      p.subLogger.WithoutSampler(),
		TrackResolver: func(lp types.LocalParticipant, ti hublive.TrackID) types.MediaResolverResult {
			return p.helper().ResolveMediaTrack(lp, ti)
		},
		DataTrackResolver: func(lp types.LocalParticipant, ti hublive.TrackID) types.DataResolverResult {
			return p.helper().ResolveDataTrack(lp, ti)
		},
		TelemetryListener:        p.params.TelemetryListener,
		OnTrackSubscribed:        p.onTrackSubscribed,
		OnTrackUnsubscribed:      p.onTrackUnsubscribed,
		OnSubscriptionError:      p.onSubscriptionError,
		SubscriptionLimitVideo:   p.params.SubscriptionLimitVideo,
		SubscriptionLimitAudio:   p.params.SubscriptionLimitAudio,
		UseOneShotSignallingMode: p.params.UseOneShotSignallingMode,
	})
}

func (p *ParticipantImpl) MetricsCollectorTimeToCollectMetrics() {
	publisherRTT, ok := p.TransportManager.GetPublisherRTT()
	if ok {
		p.metricsCollector.AddPublisherRTT(p.Identity(), float32(publisherRTT))
	}

	subscriberRTT, ok := p.TransportManager.GetSubscriberRTT()
	if ok {
		p.metricsCollector.AddSubscriberRTT(float32(subscriberRTT))
	}
}

func (p *ParticipantImpl) MetricsCollectorBatchReady(mb *hublive.MetricsBatch) {
	p.listener().OnMetrics(p, &hublive.DataPacket{
		ParticipantIdentity: string(p.Identity()),
		Value: &hublive.DataPacket_Metrics{
			Metrics: mb,
		},
	})
}

func (p *ParticipantImpl) MetricsReporterBatchReady(mb *hublive.MetricsBatch) {
	dpData, err := proto.Marshal(&hublive.DataPacket{
		ParticipantIdentity: string(p.Identity()),
		Value: &hublive.DataPacket_Metrics{
			Metrics: mb,
		},
	})
	if err != nil {
		p.params.Logger.Errorw("failed to marshal data packet", err)
		return
	}

	p.TransportManager.SendDataMessage(hublive.DataPacket_RELIABLE, dpData)
}

func (p *ParticipantImpl) setupMetrics() {
	if !p.params.EnableMetrics {
		return
	}

	p.metricTimestamper = metric.NewMetricTimestamper(metric.MetricTimestamperParams{
		Config: p.params.MetricConfig.Timestamper,
		Logger: p.params.Logger,
	})
	p.metricsCollector = metric.NewMetricsCollector(metric.MetricsCollectorParams{
		ParticipantIdentity: p.Identity(),
		Config:              p.params.MetricConfig.Collector,
		Provider:            p,
		Logger:              p.params.Logger,
	})
	p.metricsReporter = metric.NewMetricsReporter(metric.MetricsReporterParams{
		ParticipantIdentity: p.Identity(),
		Config:              p.params.MetricConfig.Reporter,
		Consumer:            p,
		Logger:              p.params.Logger,
	})
}

func (p *ParticipantImpl) updateState(state hublive.ParticipantInfo_State) {
	var oldState hublive.ParticipantInfo_State
	for {
		oldState = p.state.Load().(hublive.ParticipantInfo_State)
		if state <= oldState {
			p.params.Logger.Debugw("ignoring out of order participant state", "state", state.String())
			return
		}
		if state == hublive.ParticipantInfo_ACTIVE {
			p.lastActiveAt.CompareAndSwap(nil, pointer.To(time.Now()))
		}
		if p.state.CompareAndSwap(oldState, state) {
			break
		}
	}

	p.params.Logger.Debugw("updating participant state", "state", state.String())
	p.dirty.Store(true)

	go p.listener().OnStateChange(p)

	if state == hublive.ParticipantInfo_DISCONNECTED && oldState == hublive.ParticipantInfo_ACTIVE {
		p.disconnectedAt.Store(pointer.To(time.Now()))
		prometheus.RecordSessionDuration(int(p.ProtocolVersion()), time.Since(*p.lastActiveAt.Load()))
	}
}

func (p *ParticipantImpl) onReceivedDataMessage(kind hublive.DataPacket_Kind, data []byte) {
	if p.IsDisconnected() || !p.CanPublishData() {
		return
	}

	p.dataChannelStats.AddBytes(uint64(len(data)), false)

	dp := &hublive.DataPacket{}
	if err := proto.Unmarshal(data, dp); err != nil {
		p.pubLogger.Warnw("could not parse data packet", err)
		return
	}

	dp.ParticipantSid = string(p.ID())
	if kind == hublive.DataPacket_RELIABLE && dp.Sequence > 0 {
		if p.reliableDataInfo.stopReliableByMigrateOut.Load() {
			return
		}

		if migrationCache := p.reliableDataInfo.migrateInPubDataCache.Load(); migrationCache != nil {
			switch migrationCache.Add(dp) {
			case MigrationDataCacheStateWaiting:
				// waiting for the reliable sequence to continue from last node
				return

			case MigrationDataCacheStateTimeout:
				p.reliableDataInfo.migrateInPubDataCache.Store(nil)
				// waiting time out, handle all cached messages
				cachedMsgs := migrationCache.Get()
				if len(cachedMsgs) == 0 {
					p.pubLogger.Warnw(
						"migration data cache timed out, no cached messages received", nil,
						"lastPubReliableSeq", p.params.LastPubReliableSeq,
					)
				} else {
					p.pubLogger.Warnw(
						"migration data cache timed out, handling cached messages", nil,
						"cachedFirstSeq", cachedMsgs[0].Sequence,
						"cachedLastSeq", cachedMsgs[len(cachedMsgs)-1].Sequence,
						"lastPubReliableSeq", p.params.LastPubReliableSeq,
					)
				}
				for _, cachedDp := range cachedMsgs {
					p.handleReceivedDataMessage(kind, cachedDp)
				}
				return

			case MigrationDataCacheStateDone:
				// see the continuous message, drop the cache
				p.reliableDataInfo.migrateInPubDataCache.Store(nil)
			}
		}
	}

	p.handleReceivedDataMessage(kind, dp)
}

func (p *ParticipantImpl) handleReceivedDataMessage(kind hublive.DataPacket_Kind, dp *hublive.DataPacket) {
	if kind == hublive.DataPacket_RELIABLE && dp.Sequence > 0 {
		if p.reliableDataInfo.lastPubReliableSeq.Load() >= dp.Sequence {
			p.params.Logger.Infow(
				"received out of order reliable data packet",
				"lastPubReliableSeq", p.reliableDataInfo.lastPubReliableSeq.Load(),
				"dpSequence", dp.Sequence,
			)
			return
		}

		p.reliableDataInfo.lastPubReliableSeq.Store(dp.Sequence)
	}

	// trust the channel that it came in as the source of truth
	dp.Kind = kind

	shouldForwardData := true
	shouldForwardMetrics := false
	overrideSenderIdentity := true
	// only forward on user payloads
	switch payload := dp.Value.(type) {
	case *hublive.DataPacket_User:
		if payload.User == nil {
			return
		}
		u := payload.User
		if p.Hidden() {
			u.ParticipantSid = ""
			u.ParticipantIdentity = ""
		} else {
			u.ParticipantSid = string(p.ID())
			u.ParticipantIdentity = string(p.params.Identity)
		}
		if len(dp.DestinationIdentities) != 0 {
			u.DestinationIdentities = dp.DestinationIdentities
		} else {
			dp.DestinationIdentities = u.DestinationIdentities
		}
	case *hublive.DataPacket_SipDtmf:
		if payload.SipDtmf == nil {
			return
		}
	case *hublive.DataPacket_Transcription:
		if payload.Transcription == nil {
			return
		}
		if !p.IsAgent() {
			shouldForwardData = false
		}
	case *hublive.DataPacket_ChatMessage:
		if payload.ChatMessage == nil {
			return
		}
		if p.IsAgent() && dp.ParticipantIdentity != "" && string(p.params.Identity) != dp.ParticipantIdentity {
			overrideSenderIdentity = false
			payload.ChatMessage.Generated = true
		}
	case *hublive.DataPacket_Metrics:
		if payload.Metrics == nil {
			return
		}
		shouldForwardData = false
		shouldForwardMetrics = true
		p.metricTimestamper.Process(payload.Metrics)
	case *hublive.DataPacket_RpcRequest:
		if payload.RpcRequest == nil {
			return
		}
		p.pubLogger.Debugw(
			"received RPC request",
			"method", payload.RpcRequest.Method,
			"rpc_request_id", payload.RpcRequest.Id,
			"destinationIdentities", dp.DestinationIdentities,
		)
	case *hublive.DataPacket_RpcResponse:
		if payload.RpcResponse == nil {
			return
		}
		p.pubLogger.Debugw(
			"received RPC response",
			"rpc_request_id", payload.RpcResponse.RequestId,
		)

		rpcResponse := payload.RpcResponse
		switch res := rpcResponse.Value.(type) {
		case *hublive.RpcResponse_Payload:
			shouldForwardData = !p.handleIncomingRpcResponse(payload.RpcResponse.GetRequestId(), res.Payload, nil)
		case *hublive.RpcResponse_Error:
			shouldForwardData = !p.handleIncomingRpcResponse(payload.RpcResponse.GetRequestId(), "", &utils.DataChannelRpcError{
				Code:    utils.DataChannelRpcErrorCode(res.Error.GetCode()),
				Message: res.Error.GetMessage(),
				Data:    res.Error.GetData(),
			})
		}
	case *hublive.DataPacket_RpcAck:
		if payload.RpcAck == nil {
			return
		}
		p.pubLogger.Debugw(
			"received RPC ack",
			"rpc_request_id", payload.RpcAck.RequestId,
		)

		shouldForwardData = !p.handleIncomingRpcAck(payload.RpcAck.GetRequestId())
	case *hublive.DataPacket_StreamHeader:
		if payload.StreamHeader == nil {
			return
		}

		prometheus.RecordDataPacketStream(payload.StreamHeader, len(dp.DestinationIdentities))

		if p.IsAgent() && dp.ParticipantIdentity != "" && string(p.params.Identity) != dp.ParticipantIdentity {
			switch contentHeader := payload.StreamHeader.ContentHeader.(type) {
			case *hublive.DataStream_Header_TextHeader:
				contentHeader.TextHeader.Generated = true
				overrideSenderIdentity = false
			default:
				overrideSenderIdentity = true
			}
		}
	case *hublive.DataPacket_StreamChunk:
		if payload.StreamChunk == nil {
			return
		}
	case *hublive.DataPacket_StreamTrailer:
		if payload.StreamTrailer == nil {
			return
		}
	case *hublive.DataPacket_EncryptedPacket:
		if payload.EncryptedPacket == nil {
			return
		}
	default:
		p.pubLogger.Warnw("received unsupported data packet", nil, "payload", payload)
	}

	// SFU typically asserts the sender's identity. However, agents are able to
	// publish data on behalf of the participant in case of transcriptions/text streams
	// in those cases we'd leave the existing identity on the data packet alone.
	if overrideSenderIdentity {
		if p.Hidden() {
			dp.ParticipantIdentity = ""
		} else {
			dp.ParticipantIdentity = string(p.params.Identity)
		}
	}

	if shouldForwardData {
		p.listener().OnDataMessage(p, kind, dp)
	}
	if shouldForwardMetrics {
		p.listener().OnMetrics(p, dp)
	}
}

func (p *ParticipantImpl) onReceivedDataMessageUnlabeled(data []byte) {
	if p.IsDisconnected() || !p.CanPublishData() {
		return
	}

	p.dataChannelStats.AddBytes(uint64(len(data)), false)

	p.listener().OnDataMessageUnlabeled(p, data)
}

func (p *ParticipantImpl) onICECandidate(c *webrtc.ICECandidate, target hublive.SignalTarget) error {
	if p.IsDisconnected() || p.IsClosed() {
		return nil
	}

	if target == hublive.SignalTarget_SUBSCRIBER && p.MigrateState() == types.MigrateStateInit {
		return nil
	}

	return p.sendICECandidate(c, target)
}

func (p *ParticipantImpl) onPrimaryTransportInitialConnected() {
	if !p.hasPendingMigratedTrack() && len(p.GetPublishedTracks()) == 0 {
		// if there are no published tracks, declare migration complete on primary transport initial connect,
		// else, wait for all tracks to be published and publisher peer connection established
		p.SetMigrateState(types.MigrateStateComplete)
	}

	if !p.sessionStartRecorded.Swap(true) {
		prometheus.RecordSessionStartTime(int(p.ProtocolVersion()), time.Since(p.params.SessionStartTime))
	}
	p.updateState(hublive.ParticipantInfo_ACTIVE)
}

func (p *ParticipantImpl) onPrimaryTransportFullyEstablished() {
	p.replayJoiningReliableMessages()
}

func (p *ParticipantImpl) clearDisconnectTimer() {
	p.lock.Lock()
	if p.disconnectTimer != nil {
		p.disconnectTimer.Stop()
		p.disconnectTimer = nil
	}
	p.lock.Unlock()
}

func (p *ParticipantImpl) setupDisconnectTimer() {
	p.clearDisconnectTimer()

	p.lock.Lock()
	p.disconnectTimer = time.AfterFunc(disconnectCleanupDuration, func() {
		p.clearDisconnectTimer()

		if p.IsClosed() || p.IsDisconnected() {
			return
		}
		_ = p.Close(true, types.ParticipantCloseReasonPeerConnectionDisconnected, false)
	})
	p.lock.Unlock()
}

func (p *ParticipantImpl) onAnyTransportFailed() {
	if p.params.UseOneShotSignallingMode {
		// as there is no way to notify participant, close the participant on transport failure
		_ = p.Close(false, types.ParticipantCloseReasonPeerConnectionDisconnected, false)
		return
	}

	p.sendLeaveRequest(
		types.ParticipantCloseReasonPeerConnectionDisconnected,
		true,  // isExpectedToResume
		false, // isExpectedToReconnect
		true,  // sendOnlyIfSupportingLeaveRequestWithAction
	)

	// clients support resuming of connections when signalling becomes disconnected
	p.CloseSignalConnection(types.SignallingCloseReasonTransportFailure)

	// detect when participant has actually left.
	p.setupDisconnectTimer()
}

func (p *ParticipantImpl) HasConnected() bool {
	return p.TransportManager.HasSubscriberEverConnected() || p.TransportManager.HasPublisherEverConnected()
}

func (p *ParticipantImpl) DebugInfo() map[string]any {
	info := map[string]any{
		"ID":    p.ID(),
		"State": p.State().String(),
	}

	pendingTrackInfo := make(map[string]any)
	p.pendingTracksLock.RLock()
	for clientID, pti := range p.pendingTracks {
		var trackInfos []string
		for _, ti := range pti.trackInfos {
			trackInfos = append(trackInfos, ti.String())
		}

		pendingTrackInfo[clientID] = map[string]any{
			"TrackInfos": trackInfos,
			"Migrated":   pti.migrated,
		}
	}
	p.pendingTracksLock.RUnlock()
	info["PendingTracks"] = pendingTrackInfo

	info["UpTrackManager"] = p.UpTrackManager.DebugInfo()

	return info
}

func (p *ParticipantImpl) IssueFullReconnect(reason types.ParticipantCloseReason) {
	p.sendLeaveRequest(
		reason,
		false, // isExpectedToResume
		true,  // isExpectedToReconnect
		false, // sendOnlyIfSupportingLeaveRequestWithAction
	)

	scr := types.SignallingCloseReasonUnknown
	switch reason {
	case types.ParticipantCloseReasonPublicationError, types.ParticipantCloseReasonMigrateCodecMismatch:
		scr = types.SignallingCloseReasonFullReconnectPublicationError
	case types.ParticipantCloseReasonSubscriptionError:
		scr = types.SignallingCloseReasonFullReconnectSubscriptionError
	case types.ParticipantCloseReasonDataChannelError:
		scr = types.SignallingCloseReasonFullReconnectDataChannelError
	case types.ParticipantCloseReasonNegotiateFailed:
		scr = types.SignallingCloseReasonFullReconnectNegotiateFailed
	}
	p.CloseSignalConnection(scr)

	// a full reconnect == client should connect back with a new session, close current one
	p.Close(false, reason, false)
}

func (p *ParticipantImpl) onAnyTransportNegotiationFailed() {
	if p.TransportManager.SinceLastSignal() < negotiationFailedTimeout/2 {
		p.params.Logger.Infow("negotiation failed, starting full reconnect")
	}
	p.IssueFullReconnect(types.ParticipantCloseReasonNegotiateFailed)
}

func (p *ParticipantImpl) GetPlayoutDelayConfig() *hublive.PlayoutDelay {
	return p.params.PlayoutDelay
}

func (p *ParticipantImpl) SupportsSyncStreamID() bool {
	return p.ProtocolVersion().SupportsSyncStreamID() && !p.params.ClientInfo.isFirefox() && p.params.SyncStreams
}

func (p *ParticipantImpl) SupportsTransceiverReuse(mt types.MediaTrack) bool {
	if p.params.UseOneShotSignallingMode {
		return p.ProtocolVersion().SupportsTransceiverReuse()
	}

	return p.ProtocolVersion().SupportsTransceiverReuse() && !p.SupportsSyncStreamID() && (!mt.IsEncrypted() || !p.params.DisableTransceiverReuseForE2EE)
}

func (p *ParticipantImpl) SendDataMessage(kind hublive.DataPacket_Kind, data []byte, sender hublive.ParticipantID, seq uint32) error {
	if sender == "" || kind != hublive.DataPacket_RELIABLE || seq == 0 {
		if p.State() != hublive.ParticipantInfo_ACTIVE {
			return ErrDataChannelUnavailable
		}
		return p.TransportManager.SendDataMessage(kind, data)
	}

	p.reliableDataInfo.joiningMessageLock.Lock()
	if !p.reliableDataInfo.canWriteReliable {
		if _, ok := p.reliableDataInfo.joiningMessageFirstSeqs[sender]; !ok {
			p.reliableDataInfo.joiningMessageFirstSeqs[sender] = seq
		}
		p.reliableDataInfo.joiningMessageLock.Unlock()
		return nil
	}

	lastWrittenSeq, ok := p.reliableDataInfo.joiningMessageLastWrittenSeqs[sender]
	if ok {
		if seq <= lastWrittenSeq {
			// already sent by replayJoiningReliableMessages
			p.reliableDataInfo.joiningMessageLock.Unlock()
			return nil
		} else {
			delete(p.reliableDataInfo.joiningMessageLastWrittenSeqs, sender)
		}
	}

	p.reliableDataInfo.joiningMessageLock.Unlock()

	return p.TransportManager.SendDataMessage(kind, data)
}

func (p *ParticipantImpl) SendDataMessageUnlabeled(data []byte, useRaw bool, sender hublive.ParticipantIdentity) error {
	if p.State() != hublive.ParticipantInfo_ACTIVE {
		return ErrDataChannelUnavailable
	}

	return p.TransportManager.SendDataMessageUnlabeled(data, useRaw, sender)
}

func (p *ParticipantImpl) onDataSendError(err error) {
	if p.params.ReconnectOnDataChannelError {
		p.params.Logger.Infow("issuing full reconnect on data channel error", "error", err)
		p.IssueFullReconnect(types.ParticipantCloseReasonDataChannelError)
	}
}

func (p *ParticipantImpl) replayJoiningReliableMessages() {
	p.reliableDataInfo.joiningMessageLock.Lock()
	for _, msgCache := range p.helper().GetCachedReliableDataMessage(p.reliableDataInfo.joiningMessageFirstSeqs) {
		if len(msgCache.DestIdentities) != 0 && !slices.Contains(msgCache.DestIdentities, p.Identity()) {
			continue
		}
		if lastSeq, ok := p.reliableDataInfo.joiningMessageLastWrittenSeqs[msgCache.SenderID]; !ok || lastSeq < msgCache.Seq {
			p.reliableDataInfo.joiningMessageLastWrittenSeqs[msgCache.SenderID] = msgCache.Seq
		}

		p.TransportManager.SendDataMessage(hublive.DataPacket_RELIABLE, msgCache.Data)
	}

	p.reliableDataInfo.joiningMessageFirstSeqs = make(map[hublive.ParticipantID]uint32)
	p.reliableDataInfo.canWriteReliable = true
	p.reliableDataInfo.joiningMessageLock.Unlock()
}

func (p *ParticipantImpl) HandleMetrics(senderParticipantID hublive.ParticipantID, metrics *hublive.MetricsBatch) error {
	if p.State() != hublive.ParticipantInfo_ACTIVE {
		return ErrDataChannelUnavailable
	}

	if !p.CanSubscribeMetrics() {
		return ErrNoSubscribeMetricsPermission
	}

	if senderParticipantID != p.ID() && !p.SubscriptionManager.IsSubscribedTo(senderParticipantID) {
		return nil
	}

	p.metricsReporter.Merge(metrics)
	return nil
}

func (p *ParticipantImpl) SupportsCodecChange() bool {
	return p.params.ClientInfo.SupportsCodecChange()
}

func (p *ParticipantImpl) SupportsMoving() error {
	if !p.ProtocolVersion().SupportsMoving() {
		return ErrMoveOldClientVersion
	}

	if kind := p.Kind(); kind == hublive.ParticipantInfo_EGRESS || kind == hublive.ParticipantInfo_AGENT || p.params.UseOneShotSignallingMode {
		return fmt.Errorf("%s participants cannot be moved, one-shot signaling mode: %t", kind.String(), p.params.UseOneShotSignallingMode)
	}

	return nil
}

func (p *ParticipantImpl) MoveToRoom(params types.MoveToRoomParams) {
	for _, track := range p.GetPublishedTracks() {
		for _, sub := range track.GetAllSubscribers() {
			track.RemoveSubscriber(sub, false)
		}

		// clear the subscriber node max quality/audio codecs as the remote quality notify
		// from source room would not reach the moving out participant.
		track.(types.LocalMediaTrack).ClearSubscriberNodes()

		trackInfo := track.ToProto()
		p.params.TelemetryListener.OnTrackUnpublished(
			p.ID(),
			p.Identity(),
			trackInfo,
			true,
		)
	}

	// fire onClose callback for original room
	p.lock.Lock()
	onClose := p.onClose
	p.onClose = make(map[string]func(types.LocalParticipant))
	p.lock.Unlock()
	for _, cb := range onClose {
		cb(p)
	}

	p.params.Logger.Infow("move participant to new room", "newRoomName", params.RoomName, "newID", params.ParticipantID)

	p.lock.Lock()
	p.telemetryGuard = &telemetry.ReferenceGuard{}
	p.lock.Unlock()

	p.params.LoggerResolver.Reset()
	p.params.ReporterResolver.Reset()
	p.setListener(params.Listener)
	p.participantHelper.Store(params.Helper)
	p.SubscriptionManager.ClearAllSubscriptions()
	p.id.Store(params.ParticipantID)
	grants := p.grants.Load().Clone()
	grants.Video.Room = string(params.RoomName)
	p.grants.Store(grants)
}

func (p *ParticipantImpl) helper() types.LocalParticipantHelper {
	return p.participantHelper.Load().(types.LocalParticipantHelper)
}

func (p *ParticipantImpl) GetLastReliableSequence(migrateOut bool) uint32 {
	if migrateOut {
		p.reliableDataInfo.stopReliableByMigrateOut.Store(true)
	}
	return p.reliableDataInfo.lastPubReliableSeq.Load()
}

func (p *ParticipantImpl) HandleSimulateScenario(simulateScenario *hublive.SimulateScenario) error {
	return p.listener().OnSimulateScenario(p, simulateScenario)
}

func (p *ParticipantImpl) HandleLeaveRequest(reason types.ParticipantCloseReason) {
	p.listener().OnLeave(p, reason)
}

func (p *ParticipantImpl) HandleSignalMessage(msg proto.Message) error {
	return p.signalHandler.HandleMessage(msg)
}

func (p *ParticipantImpl) IsUsingSinglePeerConnection() bool {
	return p.params.UseSinglePeerConnection
}

func (p *ParticipantImpl) handleIncomingRpcAck(requestId string) bool {
	p.rpcLock.Lock()
	defer p.rpcLock.Unlock()

	handler, ok := p.rpcPendingAcks[requestId]
	if !ok {
		return false
	}

	handler.Resolve()
	delete(p.rpcPendingAcks, requestId)
	return true
}

func (p *ParticipantImpl) handleIncomingRpcResponse(requestId string, payload string, err *utils.DataChannelRpcError) bool {
	p.rpcLock.Lock()
	defer p.rpcLock.Unlock()

	handler, ok := p.rpcPendingResponses[requestId]
	if !ok {
		return false
	}

	handler.Resolve(payload, err)
	delete(p.rpcPendingResponses, requestId)
	return true
}

func (p *ParticipantImpl) PerformRpc(req *hublive.PerformRpcRequest, resultCh chan string, errorCh chan error) {
	responseTimeout := req.GetResponseTimeoutMs()
	if responseTimeout <= 0 {
		responseTimeout = uint32(utils.DataChannelRpcDefaultResponseTimeout.Milliseconds())
	}

	go func() {
		if len([]byte(req.GetPayload())) > utils.DataChannelRpcMaxPayloadBytes {
			errorCh <- utils.DataChannelRpcErrorFromBuiltInCodes(utils.DataChannelRpcRequestPayloadTooLarge, "").PsrpcError()
			return
		}

		id := uuid.NewString()

		responseTimer := time.AfterFunc(time.Duration(responseTimeout)*time.Millisecond, func() {
			p.rpcLock.Lock()
			delete(p.rpcPendingResponses, id)
			p.rpcLock.Unlock()

			select {
			case errorCh <- utils.DataChannelRpcErrorFromBuiltInCodes(utils.DataChannelRpcResponseTimeout, "").PsrpcError():
			default:
			}
		})
		ackTimer := time.AfterFunc(utils.DataChannelRpcMaxRoundTripLatency, func() {
			p.rpcLock.Lock()
			delete(p.rpcPendingAcks, id)
			delete(p.rpcPendingResponses, id)
			p.rpcLock.Unlock()
			responseTimer.Stop()

			select {
			case errorCh <- utils.DataChannelRpcErrorFromBuiltInCodes(utils.DataChannelRpcConnectionTimeout, "").PsrpcError():
			default:
			}
		})

		rpcRequest := &hublive.DataPacket{
			Kind:                hublive.DataPacket_RELIABLE,
			ParticipantIdentity: id,
			Value: &hublive.DataPacket_RpcRequest{
				RpcRequest: &hublive.RpcRequest{
					Id:                id,
					Method:            req.GetMethod(),
					Payload:           req.GetPayload(),
					ResponseTimeoutMs: responseTimeout - p.lastRTT,
					Version:           1,
				},
			},
		}
		data, err := proto.Marshal(rpcRequest)
		if err != nil {
			ackTimer.Stop()
			responseTimer.Stop()
			errorCh <- psrpc.NewError(psrpc.Internal, err)
			return
		}

		// using RPC ID as the unique ID for server to identify the response
		err = p.SendDataMessage(hublive.DataPacket_RELIABLE, data, hublive.ParticipantID(id), 0)
		if err != nil {
			ackTimer.Stop()
			responseTimer.Stop()
			errorCh <- psrpc.NewError(psrpc.Internal, err)
			return
		}

		p.rpcLock.Lock()
		p.rpcPendingAcks[id] = &utils.DataChannelRpcPendingAckHandler{
			Resolve: func() {
				ackTimer.Stop()
			},
			ParticipantIdentity: req.GetDestinationIdentity(),
		}
		p.rpcPendingResponses[id] = &utils.DataChannelRpcPendingResponseHandler{
			Resolve: func(payload string, error *utils.DataChannelRpcError) {
				responseTimer.Stop()
				if _, ok := p.rpcPendingAcks[id]; ok {
					p.rpcPendingAcks[id].Resolve()
					ackTimer.Stop()
				}

				if error != nil {
					errorCh <- error.PsrpcError()
				} else {
					resultCh <- payload
				}
			},
			ParticipantIdentity: req.GetDestinationIdentity(),
		}
		p.rpcLock.Unlock()
	}()
}
