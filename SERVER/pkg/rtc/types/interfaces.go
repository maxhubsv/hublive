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

package types

import (
	"fmt"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"

	"__GITHUB_HUBLIVE__protocol/auth"
	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/observability/roomobs"
	"__GITHUB_HUBLIVE__protocol/utils"

	"github.com/maxhubsv/hublive-server/pkg/routing"
	"github.com/maxhubsv/hublive-server/pkg/rtc/datatrack"
	"github.com/maxhubsv/hublive-server/pkg/sfu"
	"github.com/maxhubsv/hublive-server/pkg/sfu/buffer"
	"github.com/maxhubsv/hublive-server/pkg/sfu/pacer"
	"github.com/maxhubsv/hublive-server/pkg/telemetry"

	"google.golang.org/protobuf/proto"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . WebsocketClient
type WebsocketClient interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetReadDeadline(deadline time.Time) error
	Close() error
}

type AddSubscriberParams struct {
	AllTracks bool
	TrackIDs  []hublive.TrackID
}

// ---------------------------------------------

type MigrateState int32

const (
	MigrateStateInit MigrateState = iota
	MigrateStateSync
	MigrateStateComplete
)

func (m MigrateState) String() string {
	switch m {
	case MigrateStateInit:
		return "MIGRATE_STATE_INIT"
	case MigrateStateSync:
		return "MIGRATE_STATE_SYNC"
	case MigrateStateComplete:
		return "MIGRATE_STATE_COMPLETE"
	default:
		return fmt.Sprintf("%d", int(m))
	}
}

// ---------------------------------------------

type SubscribedCodecQuality struct {
	CodecMime mime.MimeType
	Quality   hublive.VideoQuality
}

// ---------------------------------------------

type ParticipantCloseReason int

const (
	ParticipantCloseReasonNone ParticipantCloseReason = iota
	ParticipantCloseReasonClientRequestLeave
	ParticipantCloseReasonRoomManagerStop
	ParticipantCloseReasonVerifyFailed
	ParticipantCloseReasonJoinFailed
	ParticipantCloseReasonJoinTimeout
	ParticipantCloseReasonMessageBusFailed
	ParticipantCloseReasonPeerConnectionDisconnected
	ParticipantCloseReasonDuplicateIdentity
	ParticipantCloseReasonMigrationComplete
	ParticipantCloseReasonStale
	ParticipantCloseReasonServiceRequestRemoveParticipant
	ParticipantCloseReasonServiceRequestDeleteRoom
	ParticipantCloseReasonSimulateMigration
	ParticipantCloseReasonSimulateNodeFailure
	ParticipantCloseReasonSimulateServerLeave
	ParticipantCloseReasonSimulateLeaveRequest
	ParticipantCloseReasonNegotiateFailed
	ParticipantCloseReasonMigrationRequested
	ParticipantCloseReasonPublicationError
	ParticipantCloseReasonSubscriptionError
	ParticipantCloseReasonDataChannelError
	ParticipantCloseReasonMigrateCodecMismatch
	ParticipantCloseReasonSignalSourceClose
	ParticipantCloseReasonRoomClosed
	ParticipantCloseReasonUserUnavailable
	ParticipantCloseReasonUserRejected
	ParticipantCloseReasonMoveFailed
	ParticipantCloseReasonAgentError
)

func (p ParticipantCloseReason) String() string {
	switch p {
	case ParticipantCloseReasonNone:
		return "NONE"
	case ParticipantCloseReasonClientRequestLeave:
		return "CLIENT_REQUEST_LEAVE"
	case ParticipantCloseReasonRoomManagerStop:
		return "ROOM_MANAGER_STOP"
	case ParticipantCloseReasonVerifyFailed:
		return "VERIFY_FAILED"
	case ParticipantCloseReasonJoinFailed:
		return "JOIN_FAILED"
	case ParticipantCloseReasonJoinTimeout:
		return "JOIN_TIMEOUT"
	case ParticipantCloseReasonMessageBusFailed:
		return "MESSAGE_BUS_FAILED"
	case ParticipantCloseReasonPeerConnectionDisconnected:
		return "PEER_CONNECTION_DISCONNECTED"
	case ParticipantCloseReasonDuplicateIdentity:
		return "DUPLICATE_IDENTITY"
	case ParticipantCloseReasonMigrationComplete:
		return "MIGRATION_COMPLETE"
	case ParticipantCloseReasonStale:
		return "STALE"
	case ParticipantCloseReasonServiceRequestRemoveParticipant:
		return "SERVICE_REQUEST_REMOVE_PARTICIPANT"
	case ParticipantCloseReasonServiceRequestDeleteRoom:
		return "SERVICE_REQUEST_DELETE_ROOM"
	case ParticipantCloseReasonSimulateMigration:
		return "SIMULATE_MIGRATION"
	case ParticipantCloseReasonSimulateNodeFailure:
		return "SIMULATE_NODE_FAILURE"
	case ParticipantCloseReasonSimulateServerLeave:
		return "SIMULATE_SERVER_LEAVE"
	case ParticipantCloseReasonSimulateLeaveRequest:
		return "SIMULATE_LEAVE_REQUEST"
	case ParticipantCloseReasonNegotiateFailed:
		return "NEGOTIATE_FAILED"
	case ParticipantCloseReasonMigrationRequested:
		return "MIGRATION_REQUESTED"
	case ParticipantCloseReasonPublicationError:
		return "PUBLICATION_ERROR"
	case ParticipantCloseReasonSubscriptionError:
		return "SUBSCRIPTION_ERROR"
	case ParticipantCloseReasonDataChannelError:
		return "DATA_CHANNEL_ERROR"
	case ParticipantCloseReasonMigrateCodecMismatch:
		return "MIGRATE_CODEC_MISMATCH"
	case ParticipantCloseReasonSignalSourceClose:
		return "SIGNAL_SOURCE_CLOSE"
	case ParticipantCloseReasonRoomClosed:
		return "ROOM_CLOSED"
	case ParticipantCloseReasonUserUnavailable:
		return "USER_UNAVAILABLE"
	case ParticipantCloseReasonUserRejected:
		return "USER_REJECTED"
	case ParticipantCloseReasonMoveFailed:
		return "MOVE_FAILED"
	case ParticipantCloseReasonAgentError:
		return "AGENT_ERROR"
	default:
		return fmt.Sprintf("%d", int(p))
	}
}

func (p ParticipantCloseReason) ToDisconnectReason() hublive.DisconnectReason {
	switch p {
	case ParticipantCloseReasonClientRequestLeave, ParticipantCloseReasonSimulateLeaveRequest:
		return hublive.DisconnectReason_CLIENT_INITIATED
	case ParticipantCloseReasonRoomManagerStop:
		return hublive.DisconnectReason_SERVER_SHUTDOWN
	case ParticipantCloseReasonVerifyFailed, ParticipantCloseReasonJoinFailed, ParticipantCloseReasonJoinTimeout, ParticipantCloseReasonMessageBusFailed:
		// expected to be connected but is not
		return hublive.DisconnectReason_JOIN_FAILURE
	case ParticipantCloseReasonPeerConnectionDisconnected:
		return hublive.DisconnectReason_CONNECTION_TIMEOUT
	case ParticipantCloseReasonDuplicateIdentity, ParticipantCloseReasonStale:
		return hublive.DisconnectReason_DUPLICATE_IDENTITY
	case ParticipantCloseReasonMigrationRequested, ParticipantCloseReasonMigrationComplete, ParticipantCloseReasonSimulateMigration:
		return hublive.DisconnectReason_MIGRATION
	case ParticipantCloseReasonServiceRequestRemoveParticipant:
		return hublive.DisconnectReason_PARTICIPANT_REMOVED
	case ParticipantCloseReasonServiceRequestDeleteRoom:
		return hublive.DisconnectReason_ROOM_DELETED
	case ParticipantCloseReasonSimulateNodeFailure, ParticipantCloseReasonSimulateServerLeave:
		return hublive.DisconnectReason_SERVER_SHUTDOWN
	case ParticipantCloseReasonNegotiateFailed, ParticipantCloseReasonPublicationError, ParticipantCloseReasonSubscriptionError,
		ParticipantCloseReasonDataChannelError, ParticipantCloseReasonMigrateCodecMismatch, ParticipantCloseReasonMoveFailed:
		return hublive.DisconnectReason_STATE_MISMATCH
	case ParticipantCloseReasonSignalSourceClose:
		return hublive.DisconnectReason_SIGNAL_CLOSE
	case ParticipantCloseReasonRoomClosed:
		return hublive.DisconnectReason_ROOM_CLOSED
	case ParticipantCloseReasonUserUnavailable:
		return hublive.DisconnectReason_USER_UNAVAILABLE
	case ParticipantCloseReasonUserRejected:
		return hublive.DisconnectReason_USER_REJECTED
	case ParticipantCloseReasonAgentError:
		return hublive.DisconnectReason_AGENT_ERROR
	default:
		// the other types will map to unknown reason
		return hublive.DisconnectReason_UNKNOWN_REASON
	}
}

// ---------------------------------------------

type SignallingCloseReason int

const (
	SignallingCloseReasonUnknown SignallingCloseReason = iota
	SignallingCloseReasonMigration
	SignallingCloseReasonResume
	SignallingCloseReasonTransportFailure
	SignallingCloseReasonFullReconnectPublicationError
	SignallingCloseReasonFullReconnectSubscriptionError
	SignallingCloseReasonFullReconnectDataChannelError
	SignallingCloseReasonFullReconnectNegotiateFailed
	SignallingCloseReasonParticipantClose
	SignallingCloseReasonDisconnectOnResume
	SignallingCloseReasonDisconnectOnResumeNoMessages
)

func (s SignallingCloseReason) String() string {
	switch s {
	case SignallingCloseReasonUnknown:
		return "UNKNOWN"
	case SignallingCloseReasonMigration:
		return "MIGRATION"
	case SignallingCloseReasonResume:
		return "RESUME"
	case SignallingCloseReasonTransportFailure:
		return "TRANSPORT_FAILURE"
	case SignallingCloseReasonFullReconnectPublicationError:
		return "FULL_RECONNECT_PUBLICATION_ERROR"
	case SignallingCloseReasonFullReconnectSubscriptionError:
		return "FULL_RECONNECT_SUBSCRIPTION_ERROR"
	case SignallingCloseReasonFullReconnectDataChannelError:
		return "FULL_RECONNECT_DATA_CHANNEL_ERROR"
	case SignallingCloseReasonFullReconnectNegotiateFailed:
		return "FULL_RECONNECT_NEGOTIATE_FAILED"
	case SignallingCloseReasonParticipantClose:
		return "PARTICIPANT_CLOSE"
	case SignallingCloseReasonDisconnectOnResume:
		return "DISCONNECT_ON_RESUME"
	case SignallingCloseReasonDisconnectOnResumeNoMessages:
		return "DISCONNECT_ON_RESUME_NO_MESSAGES"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}

// ---------------------------------------------
const (
	ParticipantCloseKeyNormal = "normal"
	ParticipantCloseKeyWHIP   = "whip"
)

// ---------------------------------------------

//counterfeiter:generate . Participant
type Participant interface {
	ID() hublive.ParticipantID
	Identity() hublive.ParticipantIdentity
	State() hublive.ParticipantInfo_State
	ConnectedAt() time.Time
	CloseReason() ParticipantCloseReason
	Kind() hublive.ParticipantInfo_Kind
	IsRecorder() bool
	IsDependent() bool
	IsAgent() bool

	GetLogger() logger.Logger

	CanSkipBroadcast() bool
	Version() utils.TimedVersion
	ToProto() *hublive.ParticipantInfo
	ToProtoWithVersion() (*hublive.ParticipantInfo, utils.TimedVersion)

	IsPublisher() bool
	GetPublishedTrack(trackID hublive.TrackID) MediaTrack
	GetPublishedTracks() []MediaTrack
	RemovePublishedTrack(track MediaTrack, isExpectedToResume bool)

	GetPublishedDataTracks() []DataTrack
	GetPublishedDataTrack(handle uint16) DataTrack
	RemovePublishedDataTrack(track DataTrack)

	GetAudioLevel() (smoothedLevel float64, active bool)

	// HasPermission checks permission of the subscriber by identity. Returns true if subscriber is allowed to subscribe
	// to the track with trackID
	HasPermission(trackID hublive.TrackID, subIdentity hublive.ParticipantIdentity) bool

	// permissions
	Hidden() bool

	MigrateState() MigrateState

	Close(sendLeave bool, reason ParticipantCloseReason, isExpectedToResume bool) error
	IsClosed() bool
	IsDisconnected() bool

	SubscriptionPermission() (*hublive.SubscriptionPermission, utils.TimedVersion)

	// updates from remotes
	UpdateSubscriptionPermission(
		subscriptionPermission *hublive.SubscriptionPermission,
		timedVersion utils.TimedVersion,
		resolverBySid func(participantID hublive.ParticipantID) LocalParticipant,
	) error

	DebugInfo() map[string]any

	HandleReceivedDataTrackMessage([]byte, *datatrack.Packet, int64)

	GetParticipantListener() ParticipantListener
}

// -------------------------------------------------------

type AddTrackParams struct {
	Stereo bool
	Red    bool
}

type MoveToRoomParams struct {
	RoomName      hublive.RoomName
	ParticipantID hublive.ParticipantID
	Listener      LocalParticipantListener
	Helper        LocalParticipantHelper
}

type DataMessageCache struct {
	Data           []byte
	SenderID       hublive.ParticipantID
	Seq            uint32
	DestIdentities []hublive.ParticipantIdentity
}

//counterfeiter:generate . LocalParticipantHelper
type LocalParticipantHelper interface {
	ResolveMediaTrack(LocalParticipant, hublive.TrackID) MediaResolverResult
	ResolveDataTrack(LocalParticipant, hublive.TrackID) DataResolverResult
	GetParticipantInfo(pID hublive.ParticipantID) *hublive.ParticipantInfo
	GetRegionSettings(ip string) *hublive.RegionSettings
	GetSubscriberForwarderState(p LocalParticipant) (map[hublive.TrackID]*hublive.RTPForwarderState, error)
	ShouldRegressCodec() bool
	GetCachedReliableDataMessage(seqs map[hublive.ParticipantID]uint32) []*DataMessageCache
}

//counterfeiter:generate . LocalParticipant
type LocalParticipant interface {
	Participant

	TelemetryGuard() *telemetry.ReferenceGuard
	GetTelemetryListener() ParticipantTelemetryListener

	// getters
	GetCountry() string
	GetTrailer() []byte
	GetLoggerResolver() logger.DeferredFieldResolver
	GetReporter() roomobs.ParticipantSessionReporter
	GetReporterResolver() roomobs.ParticipantReporterResolver
	GetAdaptiveStream() bool
	ProtocolVersion() ProtocolVersion
	SupportsSyncStreamID() bool
	SupportsTransceiverReuse(mt MediaTrack) bool
	IsUsingSinglePeerConnection() bool
	IsReady() bool
	ActiveAt() time.Time
	Disconnected() <-chan struct{}
	IsIdle() bool
	SubscriberAsPrimary() bool
	GetClientInfo() *hublive.ClientInfo
	GetClientConfiguration() *hublive.ClientConfiguration
	GetBufferFactory() *buffer.Factory
	GetPlayoutDelayConfig() *hublive.PlayoutDelay
	GetPendingTrack(trackID hublive.TrackID) *hublive.TrackInfo
	GetICEConnectionInfo() []*ICEConnectionInfo
	HasConnected() bool
	GetEnabledPublishCodecs() []*hublive.Codec
	GetPublisherICESessionUfrag() (string, error)
	SupportsMoving() error
	GetLastReliableSequence(migrateOut bool) uint32

	SwapResponseSink(sink routing.MessageSink, reason SignallingCloseReason)
	GetResponseSink() routing.MessageSink
	CloseSignalConnection(reason SignallingCloseReason)
	UpdateLastSeenSignal()
	SetSignalSourceValid(valid bool)
	HandleSignalSourceClose()

	// updates
	UpdateMetadata(update *hublive.UpdateParticipantMetadata, fromAdmin bool) error
	SetName(name string)
	SetMetadata(metadata string)
	SetAttributes(attributes map[string]string)
	UpdateAudioTrack(update *hublive.UpdateLocalAudioTrack) error
	UpdateVideoTrack(update *hublive.UpdateLocalVideoTrack) error

	// permissions
	ClaimGrants() *auth.ClaimGrants
	SetPermission(permission *hublive.ParticipantPermission) bool
	CanPublish() bool
	CanPublishSource(source hublive.TrackSource) bool
	CanSubscribe() bool
	CanPublishData() bool

	// PeerConnection
	HandleICETrickle(trickleRequest *hublive.TrickleRequest)
	HandleOffer(sd *hublive.SessionDescription) error
	GetAnswer() (webrtc.SessionDescription, uint32, error)
	HandleICETrickleSDPFragment(sdpFragment string) error
	HandleICERestartSDPFragment(sdpFragment string) (string, error)
	AddTrack(req *hublive.AddTrackRequest)
	SetTrackMuted(mute *hublive.MuteTrackRequest, fromAdmin bool) *hublive.TrackInfo

	HandleAnswer(sd *hublive.SessionDescription)
	Negotiate(force bool)
	ICERestart(iceConfig *hublive.ICEConfig)
	AddTrackLocal(trackLocal webrtc.TrackLocal, params AddTrackParams) (*webrtc.RTPSender, *webrtc.RTPTransceiver, error)
	AddTransceiverFromTrackLocal(trackLocal webrtc.TrackLocal, params AddTrackParams) (*webrtc.RTPSender, *webrtc.RTPTransceiver, error)
	RemoveTrackLocal(sender *webrtc.RTPSender) error

	WriteSubscriberRTCP(pkts []rtcp.Packet) error

	// subscriptions
	SubscribeToTrack(trackID hublive.TrackID, isSync bool)
	UnsubscribeFromTrack(trackID hublive.TrackID)
	UpdateSubscribedTrackSettings(trackID hublive.TrackID, settings *hublive.UpdateTrackSettings)
	GetSubscribedTracks() []SubscribedTrack
	IsTrackNameSubscribed(publisherIdentity hublive.ParticipantIdentity, trackName string) bool
	SubscribeToDataTrack(trackID hublive.TrackID)
	UnsubscribeFromDataTrack(trackID hublive.TrackID)
	UpdateDataTrackSubscriptionOptions(trackID hublive.TrackID, subscriptionOptions *hublive.DataTrackSubscriptionOptions)
	Verify() bool
	VerifySubscribeParticipantInfo(pID hublive.ParticipantID, version uint32)
	// WaitUntilSubscribed waits until all subscriptions have been settled, or if the timeout
	// has been reached. If the timeout expires, it will return an error.
	WaitUntilSubscribed(timeout time.Duration) error
	StopAndGetSubscribedTracksForwarderState() map[hublive.TrackID]*hublive.RTPForwarderState
	SupportsCodecChange() bool

	// returns list of participant identities that the current participant is subscribed to
	GetSubscribedParticipants() []hublive.ParticipantID
	IsSubscribedTo(sid hublive.ParticipantID) bool

	GetConnectionQuality() *hublive.ConnectionQualityInfo

	// server sent messages
	SendJoinResponse(joinResponse *hublive.JoinResponse) error
	SendParticipantUpdate(participants []*hublive.ParticipantInfo) error
	SendSpeakerUpdate(speakers []*hublive.SpeakerInfo, force bool) error
	SendDataMessage(kind hublive.DataPacket_Kind, data []byte, senderID hublive.ParticipantID, seq uint32) error
	SendDataMessageUnlabeled(data []byte, useRaw bool, sender hublive.ParticipantIdentity) error
	SendRoomUpdate(room *hublive.Room) error
	SendConnectionQualityUpdate(update *hublive.ConnectionQualityUpdate) error
	SendSubscriptionPermissionUpdate(publisherID hublive.ParticipantID, trackID hublive.TrackID, allowed bool) error
	SendRefreshToken(token string) error
	HandleReconnectAndSendResponse(reconnectReason hublive.ReconnectReason, reconnectResponse *hublive.ReconnectResponse) error
	IssueFullReconnect(reason ParticipantCloseReason)
	SendRoomMovedResponse(moved *hublive.RoomMovedResponse) error
	SendDataTrackSubscriberHandles(handles map[uint32]*hublive.DataTrackSubscriberHandles_PublishedDataTrack) error

	AddOnClose(key string, callback func(LocalParticipant))
	OnClaimsChanged(callback func(LocalParticipant))

	HandleReceiverReport(dt *sfu.DownTrack, report *rtcp.ReceiverReport)

	// session migration
	MaybeStartMigration(force bool, onStart func()) bool
	NotifyMigration()
	SetMigrateState(s MigrateState)
	SetMigrateInfo(
		previousOffer *webrtc.SessionDescription,
		previousAnswer *webrtc.SessionDescription,
		mediaTracks []*hublive.TrackPublishedResponse,
		dataChannels []*hublive.DataChannelInfo,
		dataChannelReceiveState []*hublive.DataChannelReceiveState,
		dataTracks []*hublive.PublishDataTrackResponse,
	)
	IsReconnect() bool
	MoveToRoom(params MoveToRoomParams)

	UpdateMediaRTT(rtt uint32)
	UpdateSignalingRTT(rtt uint32)

	CacheDownTrack(trackID hublive.TrackID, rtpTransceiver *webrtc.RTPTransceiver, downTrackState sfu.DownTrackState)
	UncacheDownTrack(rtpTransceiver *webrtc.RTPTransceiver)
	GetCachedDownTrack(trackID hublive.TrackID) (*webrtc.RTPTransceiver, sfu.DownTrackState)

	SetICEConfig(iceConfig *hublive.ICEConfig)
	GetICEConfig() *hublive.ICEConfig
	OnICEConfigChanged(callback func(participant LocalParticipant, iceConfig *hublive.ICEConfig))

	UpdateSubscribedQuality(nodeID hublive.NodeID, trackID hublive.TrackID, maxQualities []SubscribedCodecQuality) error
	UpdateSubscribedAudioCodecs(nodeID hublive.NodeID, trackID hublive.TrackID, codecs []*hublive.SubscribedAudioCodec) error
	UpdateMediaLoss(nodeID hublive.NodeID, trackID hublive.TrackID, fractionalLoss uint32) error

	// down stream bandwidth management
	SetSubscriberAllowPause(allowPause bool)
	SetSubscriberChannelCapacity(channelCapacity int64)

	GetPacer() pacer.Pacer

	GetDisableSenderReportPassThrough() bool

	HandleMetrics(senderParticipantID hublive.ParticipantID, batch *hublive.MetricsBatch) error
	HandleUpdateSubscriptions(
		[]hublive.TrackID,
		[]*hublive.ParticipantTracks,
		bool,
	)
	HandleUpdateSubscriptionPermission(*hublive.SubscriptionPermission) error
	HandleSyncState(*hublive.SyncState) error
	HandleSimulateScenario(*hublive.SimulateScenario) error
	HandleLeaveRequest(reason ParticipantCloseReason)

	HandlePublishDataTrackRequest(*hublive.PublishDataTrackRequest)
	HandleUnpublishDataTrackRequest(*hublive.UnpublishDataTrackRequest)
	HandleUpdateDataSubscription(*hublive.UpdateDataSubscription)

	HandleSignalMessage(msg proto.Message) error

	PerformRpc(req *hublive.PerformRpcRequest, resultCh chan string, errorCh chan error)

	GetDataTrackTransport() DataTrackTransport

	ClearParticipantListener()

	GetNextSubscribedDataTrackHandle() uint16
}

// ---------------------------------------------

//counterfeiter:generate . ParticipantListener
type ParticipantListener interface {
	OnParticipantUpdate(Participant)
	OnTrackPublished(Participant, MediaTrack)
	OnTrackUpdated(Participant, MediaTrack)
	OnTrackUnpublished(Participant, MediaTrack)
	OnDataTrackPublished(Participant, DataTrack)
	OnDataTrackUnpublished(Participant, DataTrack)
	OnDataTrackMessage(Participant, []byte, *datatrack.Packet)
	OnMetrics(Participant, *hublive.DataPacket)
}

var _ ParticipantListener = (*NullParticipantListener)(nil)

type NullParticipantListener struct{}

func (*NullParticipantListener) OnParticipantUpdate(Participant)                           {}
func (*NullParticipantListener) OnTrackPublished(Participant, MediaTrack)                  {}
func (*NullParticipantListener) OnTrackUpdated(Participant, MediaTrack)                    {}
func (*NullParticipantListener) OnTrackUnpublished(Participant, MediaTrack)                {}
func (*NullParticipantListener) OnDataTrackPublished(Participant, DataTrack)               {}
func (*NullParticipantListener) OnDataTrackUnpublished(Participant, DataTrack)             {}
func (*NullParticipantListener) OnDataTrackMessage(Participant, []byte, *datatrack.Packet) {}
func (*NullParticipantListener) OnMetrics(Participant, *hublive.DataPacket)                {}

// ---------------------------------------------

//counterfeiter:generate . LocalParticipantListener
type LocalParticipantListener interface {
	ParticipantListener

	OnStateChange(LocalParticipant)
	OnSubscriberReady(LocalParticipant)
	OnMigrateStateChange(LocalParticipant, MigrateState)
	OnDataMessage(LocalParticipant, hublive.DataPacket_Kind, *hublive.DataPacket)
	OnDataMessageUnlabeled(LocalParticipant, []byte)
	OnSubscribeStatusChanged(LocalParticipant, hublive.ParticipantID, bool)
	OnUpdateSubscriptions(
		LocalParticipant,
		[]hublive.TrackID,
		[]*hublive.ParticipantTracks,
		bool,
	)
	OnUpdateSubscriptionPermission(LocalParticipant, *hublive.SubscriptionPermission) error
	OnUpdateDataSubscriptions(LocalParticipant, *hublive.UpdateDataSubscription)
	OnSyncState(LocalParticipant, *hublive.SyncState) error
	OnSimulateScenario(LocalParticipant, *hublive.SimulateScenario) error
	OnLeave(LocalParticipant, ParticipantCloseReason)
}

var _ LocalParticipantListener = (*NullLocalParticipantListener)(nil)

type NullLocalParticipantListener struct {
	NullParticipantListener
}

func (*NullLocalParticipantListener) OnStateChange(LocalParticipant)                      {}
func (*NullLocalParticipantListener) OnSubscriberReady(LocalParticipant)                  {}
func (*NullLocalParticipantListener) OnMigrateStateChange(LocalParticipant, MigrateState) {}
func (*NullLocalParticipantListener) OnDataMessage(LocalParticipant, hublive.DataPacket_Kind, *hublive.DataPacket) {
}
func (*NullLocalParticipantListener) OnDataMessageUnlabeled(LocalParticipant, []byte) {}
func (*NullLocalParticipantListener) OnSubscribeStatusChanged(LocalParticipant, hublive.ParticipantID, bool) {
}
func (*NullLocalParticipantListener) OnUpdateSubscriptions(
	LocalParticipant,
	[]hublive.TrackID,
	[]*hublive.ParticipantTracks,
	bool,
) {
}
func (*NullLocalParticipantListener) OnUpdateSubscriptionPermission(LocalParticipant, *hublive.SubscriptionPermission) error {
	return nil
}
func (*NullLocalParticipantListener) OnUpdateDataSubscriptions(LocalParticipant, *hublive.UpdateDataSubscription) {
}
func (*NullLocalParticipantListener) OnSyncState(LocalParticipant, *hublive.SyncState) error {
	return nil
}
func (*NullLocalParticipantListener) OnSimulateScenario(LocalParticipant, *hublive.SimulateScenario) error {
	return nil
}
func (*NullLocalParticipantListener) OnLeave(LocalParticipant, ParticipantCloseReason) {}

// ---------------------------------------------

//counterfeiter:generate . ParticipantTelemetryListener
type ParticipantTelemetryListener interface {
	OnTrackPublishRequested(pID hublive.ParticipantID, identity hublive.ParticipantIdentity, ti *hublive.TrackInfo)
	OnTrackPublished(pID hublive.ParticipantID, identity hublive.ParticipantIdentity, ti *hublive.TrackInfo, shouldSendEvent bool)
	OnTrackUnpublished(pID hublive.ParticipantID, identity hublive.ParticipantIdentity, ti *hublive.TrackInfo, shouldSendEvent bool)
	OnTrackSubscribeRequested(pID hublive.ParticipantID, ti *hublive.TrackInfo)
	OnTrackSubscribed(pID hublive.ParticipantID, ti *hublive.TrackInfo, publisherInfo *hublive.ParticipantInfo, shouldSendEvent bool)
	OnTrackUnsubscribed(pID hublive.ParticipantID, ti *hublive.TrackInfo, shouldSendEvent bool)
	OnTrackSubscribeFailed(pID hublive.ParticipantID, ti hublive.TrackID, err error, isUserError bool)
	OnTrackMuted(pID hublive.ParticipantID, ti *hublive.TrackInfo)
	OnTrackUnmuted(pID hublive.ParticipantID, ti *hublive.TrackInfo)
	OnTrackPublishedUpdate(pID hublive.ParticipantID, ti *hublive.TrackInfo)
	OnTrackMaxSubscribedVideoQuality(pID hublive.ParticipantID, ti *hublive.TrackInfo, mime mime.MimeType, maxQuality hublive.VideoQuality)
	OnTrackPublishRTPStats(pID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, layer int, stats *hublive.RTPStats)
	OnTrackSubscribeRTPStats(pID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, stats *hublive.RTPStats)

	OnTrackStats(key telemetry.StatsKey, stat *hublive.AnalyticsStat)
}

var _ ParticipantTelemetryListener = (*NullParticipantTelemetryListener)(nil)

type NullParticipantTelemetryListener struct{}

func (NullParticipantTelemetryListener) OnTrackPublishRequested(pID hublive.ParticipantID, identity hublive.ParticipantIdentity, ti *hublive.TrackInfo) {
}
func (NullParticipantTelemetryListener) OnTrackPublished(pID hublive.ParticipantID, identity hublive.ParticipantIdentity, ti *hublive.TrackInfo, shouldSendEvent bool) {
}
func (NullParticipantTelemetryListener) OnTrackUnpublished(pID hublive.ParticipantID, identity hublive.ParticipantIdentity, ti *hublive.TrackInfo, shouldSendEvent bool) {
}
func (NullParticipantTelemetryListener) OnTrackSubscribeRequested(pID hublive.ParticipantID, ti *hublive.TrackInfo) {
}
func (NullParticipantTelemetryListener) OnTrackSubscribed(pID hublive.ParticipantID, ti *hublive.TrackInfo, publisherInfo *hublive.ParticipantInfo, shouldSendEvent bool) {
}
func (NullParticipantTelemetryListener) OnTrackUnsubscribed(pID hublive.ParticipantID, ti *hublive.TrackInfo, shouldSendEvent bool) {
}
func (NullParticipantTelemetryListener) OnTrackSubscribeFailed(pID hublive.ParticipantID, ti hublive.TrackID, err error, isUserError bool) {
}
func (NullParticipantTelemetryListener) OnTrackMuted(pID hublive.ParticipantID, ti *hublive.TrackInfo) {
}
func (NullParticipantTelemetryListener) OnTrackUnmuted(pID hublive.ParticipantID, ti *hublive.TrackInfo) {
}
func (NullParticipantTelemetryListener) OnTrackPublishedUpdate(pID hublive.ParticipantID, ti *hublive.TrackInfo) {
}
func (NullParticipantTelemetryListener) OnTrackMaxSubscribedVideoQuality(pID hublive.ParticipantID, ti *hublive.TrackInfo, mime mime.MimeType, maxQuality hublive.VideoQuality) {
}
func (NullParticipantTelemetryListener) OnTrackPublishRTPStats(pID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, layer int, stats *hublive.RTPStats) {
}
func (NullParticipantTelemetryListener) OnTrackSubscribeRTPStats(pID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, stats *hublive.RTPStats) {
}

func (NullParticipantTelemetryListener) OnTrackStats(key telemetry.StatsKey, stat *hublive.AnalyticsStat) {
}

// ---------------------------------------------

// Room is a container of participants, and can provide room-level actions
//
//counterfeiter:generate . Room
type Room interface {
	Name() hublive.RoomName
	ID() hublive.RoomID
	RemoveParticipant(identity hublive.ParticipantIdentity, pID hublive.ParticipantID, reason ParticipantCloseReason)
	UpdateSubscriptions(
		participant LocalParticipant,
		trackIDs []hublive.TrackID,
		participantTracks []*hublive.ParticipantTracks,
		subscribe bool,
	)
	ResolveMediaTrackForSubscriber(sub LocalParticipant, trackID hublive.TrackID) MediaResolverResult
	ResolveDataTrackForSubscriber(sub LocalParticipant, trackID hublive.TrackID) DataResolverResult
	GetLocalParticipants() []LocalParticipant
	IsDataMessageUserPacketDuplicate(ip *hublive.UserPacket) bool
}

// MediaTrack represents a media track
//
//counterfeiter:generate . MediaTrack
type MediaTrack interface {
	ID() hublive.TrackID
	Kind() hublive.TrackType
	Name() string
	Source() hublive.TrackSource
	Stream() string

	UpdateTrackInfo(ti *hublive.TrackInfo)
	UpdateAudioTrack(update *hublive.UpdateLocalAudioTrack)
	UpdateVideoTrack(update *hublive.UpdateLocalVideoTrack)
	ToProto() *hublive.TrackInfo

	PublisherID() hublive.ParticipantID
	PublisherIdentity() hublive.ParticipantIdentity
	PublisherVersion() uint32
	Logger() logger.Logger

	IsMuted() bool
	SetMuted(muted bool)

	GetAudioLevel() (level float64, active bool)

	Close(isExpectedToResume bool)
	IsOpen() bool

	// callbacks
	AddOnClose(func(isExpectedToResume bool))

	// subscribers
	AddSubscriber(participant LocalParticipant) (SubscribedTrack, error)
	RemoveSubscriber(participantID hublive.ParticipantID, isExpectedToResume bool)
	IsSubscriber(subID hublive.ParticipantID) bool
	RevokeDisallowedSubscribers(allowedSubscriberIdentities []hublive.ParticipantIdentity) []hublive.ParticipantIdentity
	GetAllSubscribers() []hublive.ParticipantID
	GetNumSubscribers() int
	OnTrackSubscribed()

	// returns quality information that's appropriate for width & height
	GetQualityForDimension(mimeType mime.MimeType, width, height uint32) hublive.VideoQuality

	// returns temporal layer that's appropriate for fps
	GetTemporalLayerForSpatialFps(mimeType mime.MimeType, spatial int32, fps uint32) int32

	Receivers() []sfu.TrackReceiver
	ClearAllReceivers(isExpectedToResume bool)

	IsEncrypted() bool
	HasPacketTrailer() bool
}

//counterfeiter:generate . LocalMediaTrack
type LocalMediaTrack interface {
	MediaTrack

	Restart()

	HasSignalCid(cid string) bool
	HasSdpCid(cid string) bool

	GetConnectionScoreAndQuality() (float32, hublive.ConnectionQuality)
	GetTrackStats() *hublive.RTPStats

	SetRTT(rtt uint32)

	NotifySubscriberNodeMaxQuality(nodeID hublive.NodeID, qualities []SubscribedCodecQuality)
	NotifySubscriptionNode(nodeID hublive.NodeID, codecs []*hublive.SubscribedAudioCodec)
	ClearSubscriberNodes()
	NotifySubscriberNodeMediaLoss(nodeID hublive.NodeID, fractionalLoss uint8)
}

// DataTrack represents a data track
//
//counterfeiter:generate . DataTrack
type DataTrack interface {
	ID() hublive.TrackID
	PubHandle() uint16
	Name() string
	ToProto() *hublive.DataTrackInfo

	PublisherID() hublive.ParticipantID
	PublisherIdentity() hublive.ParticipantIdentity

	AddSubscriber(sub LocalParticipant) (DataDownTrack, error)
	RemoveSubscriber(participantID hublive.ParticipantID)
	IsSubscriber(subID hublive.ParticipantID) bool

	AddDataDownTrack(sender DataTrackSender) error
	DeleteDataDownTrack(subscriberID hublive.ParticipantID)

	HandlePacket(data []byte, packet *datatrack.Packet, arrivalTime int64)

	Close()
}

//counterfeiter:generate . DataDownTrack
type DataDownTrack interface {
	Close()

	Handle() uint16
	PublishDataTrack() DataTrack

	UpdateSubscriptionOptions(subscriptionOptions *hublive.DataTrackSubscriptionOptions)
}

//counterfeiter:generate . DataTrackSender
type DataTrackSender interface {
	SubscriberID() hublive.ParticipantID

	WritePacket(data []byte, packet *datatrack.Packet, arrivalTime int64)
}

//counterfeiter:generate . DataTrackTransport
type DataTrackTransport interface {
	SendDataTrackMessage(data []byte) error
}

//counterfeiter:generate . SubscribedTrack
type SubscribedTrack interface {
	AddOnBind(f func(error))
	IsBound() bool
	Close(isExpectedToResume bool)
	OnClose(f func(isExpectedToResume bool))
	ID() hublive.TrackID
	PublisherID() hublive.ParticipantID
	PublisherIdentity() hublive.ParticipantIdentity
	PublisherVersion() uint32
	SubscriberID() hublive.ParticipantID
	SubscriberIdentity() hublive.ParticipantIdentity
	Subscriber() LocalParticipant
	DownTrack() *sfu.DownTrack
	MediaTrack() MediaTrack
	RTPSender() *webrtc.RTPSender
	IsMuted() bool
	SetPublisherMuted(muted bool)
	UpdateSubscriberSettings(settings *hublive.UpdateTrackSettings, isImmediate bool)
	// selects appropriate video layer according to subscriber preferences
	UpdateVideoLayer()
	NeedsNegotiation() bool
}

type ChangeNotifier interface {
	AddObserver(key string, onChanged func())
	RemoveObserver(key string)
	HasObservers() bool
	NotifyChanged()
}

type MediaResolverResult struct {
	TrackChangedNotifier ChangeNotifier
	TrackRemovedNotifier ChangeNotifier
	Track                MediaTrack
	// is permission given to the requesting participant
	HasPermission     bool
	PublisherID       hublive.ParticipantID
	PublisherIdentity hublive.ParticipantIdentity
}

type DataResolverResult struct {
	TrackChangedNotifier ChangeNotifier
	TrackRemovedNotifier ChangeNotifier
	DataTrack            DataTrack
	PublisherID          hublive.ParticipantID
	PublisherIdentity    hublive.ParticipantIdentity
}

// MediaTrackResolver locates a specific media track for a subscriber
type MediaTrackResolver func(LocalParticipant, hublive.TrackID) MediaResolverResult

// DataTrackResolver locates a specific data track for a subscriber
type DataTrackResolver func(LocalParticipant, hublive.TrackID) DataResolverResult

// Supervisor/operation monitor related definitions
type OperationMonitorEvent int

const (
	OperationMonitorEventPublisherPeerConnectionConnected OperationMonitorEvent = iota
	OperationMonitorEventAddPendingPublication
	OperationMonitorEventSetPublicationMute
	OperationMonitorEventSetPublishedTrack
	OperationMonitorEventClearPublishedTrack
)

func (o OperationMonitorEvent) String() string {
	switch o {
	case OperationMonitorEventPublisherPeerConnectionConnected:
		return "PUBLISHER_PEER_CONNECTION_CONNECTED"
	case OperationMonitorEventAddPendingPublication:
		return "ADD_PENDING_PUBLICATION"
	case OperationMonitorEventSetPublicationMute:
		return "SET_PUBLICATION_MUTE"
	case OperationMonitorEventSetPublishedTrack:
		return "SET_PUBLISHED_TRACK"
	case OperationMonitorEventClearPublishedTrack:
		return "CLEAR_PUBLISHED_TRACK"
	default:
		return fmt.Sprintf("%d", int(o))
	}
}

type OperationMonitorData any

type OperationMonitor interface {
	PostEvent(ome OperationMonitorEvent, omd OperationMonitorData)
	Check() error
	IsIdle() bool
}
