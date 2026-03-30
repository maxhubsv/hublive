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
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pion/dtls/v3/pkg/crypto/elliptic"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"github.com/maxhubsv/hublive-server/pkg/config"
	"github.com/maxhubsv/hublive-server/pkg/rtc/transport"
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/sfu/buffer"
	"github.com/maxhubsv/hublive-server/pkg/sfu/bwe"
	"github.com/maxhubsv/hublive-server/pkg/sfu/bwe/remotebwe"
	"github.com/maxhubsv/hublive-server/pkg/sfu/bwe/sendsidebwe"
	"github.com/maxhubsv/hublive-server/pkg/sfu/datachannel"
	sfuinterceptor "github.com/maxhubsv/hublive-server/pkg/sfu/interceptor"
	"github.com/maxhubsv/hublive-server/pkg/sfu/pacer"
	pd "github.com/maxhubsv/hublive-server/pkg/sfu/rtpextension/playoutdelay"
	"github.com/maxhubsv/hublive-server/pkg/sfu/streamallocator"
	sfuutils "github.com/maxhubsv/hublive-server/pkg/sfu/utils"
	"github.com/maxhubsv/hublive-server/pkg/telemetry/prometheus"
	"github.com/maxhubsv/hublive-server/pkg/utils"
	lkinterceptor "__GITHUB_HUBLIVE__mediatransportutil/pkg/interceptor"
	lktwcc "__GITHUB_HUBLIVE__mediatransportutil/pkg/twcc"
	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/logger/pionlogger"
	"__GITHUB_HUBLIVE__protocol/utils/mono"
)

const (
	LossyDataChannel     = "_lossy"
	ReliableDataChannel  = "_reliable"
	DataTrackDataChannel = "_data_track"

	fastNegotiationFrequency   = 10 * time.Millisecond
	negotiationFrequency       = 150 * time.Millisecond
	negotiationFailedTimeout   = 15 * time.Second
	dtlsRetransmissionInterval = 100 * time.Millisecond
	dtlsHandshakeTimeout       = time.Minute

	iceDisconnectedTimeout = 10 * time.Second                          // compatible for ice-lite with firefox client
	iceFailedTimeout       = 5 * time.Second                           // time between disconnected and failed
	iceFailedTimeoutTotal  = iceFailedTimeout + iceDisconnectedTimeout // total time between connecting and failure
	iceKeepaliveInterval   = 2 * time.Second                           // pion's default

	minTcpICEConnectTimeout = 5 * time.Second
	maxTcpICEConnectTimeout = 12 * time.Second // js-sdk has a default 15s timeout for first connection, let server detect failure earlier before that

	minConnectTimeoutAfterICE = 10 * time.Second
	maxConnectTimeoutAfterICE = 20 * time.Second // max duration for waiting pc to connect after ICE is connected

	shortConnectionThreshold = 90 * time.Second

	dataChannelBufferSize             = 65535
	lossyDataChannelMinBufferedAmount = 8 * 1024
)

var (
	ErrNoICETransport                    = errors.New("no ICE transport")
	ErrIceRestartWithoutLocalSDP         = errors.New("ICE restart without local SDP settled")
	ErrIceRestartOnClosedPeerConnection  = errors.New("ICE restart on closed peer connection")
	ErrNoTransceiver                     = errors.New("no transceiver")
	ErrNoSender                          = errors.New("no sender")
	ErrMidNotFound                       = errors.New("mid not found")
	ErrNotSynchronousLocalCandidatesMode = errors.New("not using synchronous local candidates mode")
	ErrNoRemoteDescription               = errors.New("no remote description")
	ErrNoLocalDescription                = errors.New("no local description")
	ErrInvalidSDPFragment                = errors.New("invalid sdp fragment")
	ErrNoBundleMid                       = errors.New("could not get bundle mid")
	ErrMidMismatch                       = errors.New("media mid does not match bundle mid")
	ErrICECredentialMismatch             = errors.New("ice credential mismatch")
)

// -------------------------------------------------------------------------

type signal int

const (
	signalICEGatheringComplete signal = iota
	signalLocalICECandidate
	signalRemoteICECandidate
	signalSendOffer
	signalRemoteDescriptionReceived
	signalICERestart
)

func (s signal) String() string {
	switch s {
	case signalICEGatheringComplete:
		return "ICE_GATHERING_COMPLETE"
	case signalLocalICECandidate:
		return "LOCAL_ICE_CANDIDATE"
	case signalRemoteICECandidate:
		return "REMOTE_ICE_CANDIDATE"
	case signalSendOffer:
		return "SEND_OFFER"
	case signalRemoteDescriptionReceived:
		return "REMOTE_DESCRIPTION_RECEIVED"
	case signalICERestart:
		return "ICE_RESTART"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}

// -------------------------------------------------------

type event struct {
	*PCTransport
	signal signal
	data   any
}

func (e event) String() string {
	return fmt.Sprintf("PCTransport:Event{signal: %s, data: %+v}", e.signal, e.data)
}

// -------------------------------------------------------------------

type trackDescription struct {
	mid    string
	sender *webrtc.RTPSender
}

// PCTransport is a wrapper around PeerConnection, with some helper methods
type PCTransport struct {
	params       TransportParams
	pc           *webrtc.PeerConnection
	iceTransport *webrtc.ICETransport
	me           *webrtc.MediaEngine

	lock sync.RWMutex

	firstOfferReceived      bool
	firstOfferNoDataChannel bool
	reliableDC              *datachannel.DataChannelWriter[*webrtc.DataChannel]
	reliableDCOpened        bool
	lossyDC                 *datachannel.DataChannelWriter[*webrtc.DataChannel]
	lossyDCOpened           bool
	dataTrackDC             *datachannel.DataChannelWriter[*webrtc.DataChannel]
	unlabeledDataChannels   []*datachannel.DataChannelWriter[*webrtc.DataChannel]

	iceStartedAt               time.Time
	iceConnectedAt             time.Time
	firstConnectedAt           time.Time
	connectedAt                time.Time
	tcpICETimer                *time.Timer
	connectAfterICETimer       *time.Timer // timer to wait for pc to connect after ice connected
	resetShortConnOnICERestart atomic.Bool
	signalingRTT               atomic.Uint32 // milliseconds

	debouncedNegotiate *sfuutils.Debouncer
	debouncePending    bool
	lastNegotiate      time.Time

	onNegotiationStateChanged func(state transport.NegotiationState)

	rtxInfoExtractorFactory *sfuinterceptor.RTXInfoExtractorFactory

	// stream allocator for subscriber PC
	streamAllocator *streamallocator.StreamAllocator

	// only for subscriber PC
	bwe   bwe.BWE
	pacer pacer.Pacer

	// transceivers (senders) waiting for SetRemoteDescription (offer) to happen before
	// SetCodecPreferences can be invoked on them.
	// Pion adapts codecs/payload types from remote description.
	// If SetCodecPreferences are done before the remote description is processed,
	// it is possible that the transceiver gets payload types from media engine.
	// Subssequently if the peer sends an offer with different payload type for the
	// same codec, there could be two payload types for the same codec and the wrong
	// one could be used in the forwarding path. So, wait for `SetRemoteDescription`
	// to happen so that remote side payload types are adapted.
	sendersPendingConfigMu sync.Mutex
	sendersPendingConfig   []configureSenderParams

	previousAnswer *webrtc.SessionDescription
	// track id -> description map in previous offer sdp
	previousTrackDescription map[string]*trackDescription
	canReuseTransceiver      bool

	preferTCP atomic.Bool
	isClosed  atomic.Bool

	// used to check for offer/answer pairing,
	// i. e. every offer should have an answer before another offer can be sent
	localOfferId   atomic.Uint32
	remoteAnswerId atomic.Uint32

	remoteOfferId atomic.Uint32
	localAnswerId atomic.Uint32

	eventsQueue *utils.TypedOpsQueue[event]

	connectionDetails      *types.ICEConnectionDetails
	selectedPair           atomic.Pointer[webrtc.ICECandidatePair]
	mayFailedICEStats      []iceCandidatePairStats
	mayFailedICEStatsTimer *time.Timer

	numOutstandingAudios uint32
	numRequestSentAudios uint32
	numOutstandingVideos uint32
	numRequestSentVideos uint32

	// the following should be accessed only in event processing go routine
	cacheLocalCandidates      bool
	cachedLocalCandidates     []*webrtc.ICECandidate
	pendingRemoteCandidates   []*webrtc.ICECandidateInit
	restartAfterGathering     bool
	restartAtNextOffer        bool
	negotiationState          transport.NegotiationState
	negotiateCounter          atomic.Int32
	signalStateCheckTimer     *time.Timer
	currentOfferIceCredential string // ice user:pwd, for publish side ice restart checking
	pendingRestartIceOffer    *webrtc.SessionDescription
}

type TransportParams struct {
	Handler                       transport.Handler
	ProtocolVersion               types.ProtocolVersion
	Config                        *WebRTCConfig
	Twcc                          *lktwcc.Responder
	DirectionConfig               DirectionConfig
	CongestionControlConfig       config.CongestionControlConfig
	EnabledCodecs                 []*hublive.Codec
	Logger                        logger.Logger
	Transport                     hublive.SignalTarget
	SimTracks                     map[uint32]sfuinterceptor.SimulcastTrackInfo
	ClientInfo                    ClientInfo
	IsOfferer                     bool
	IsSendSide                    bool
	AllowPlayoutDelay             bool
	UseOneShotSignallingMode      bool
	FireOnTrackBySdp              bool
	DataChannelMaxBufferedAmount  uint64
	DatachannelSlowThreshold      int
	DatachannelLossyTargetLatency time.Duration

	// for development test
	DatachannelMaxReceiverBufferSize int

	EnableDataTracks bool
}

func newPeerConnection(
	params TransportParams,
	onBandwidthEstimator func(estimator cc.BandwidthEstimator),
) (*webrtc.PeerConnection, *webrtc.MediaEngine, *sfuinterceptor.RTXInfoExtractorFactory, error) {
	directionConfig := params.DirectionConfig
	if params.AllowPlayoutDelay {
		directionConfig.RTPHeaderExtension.Video = append(directionConfig.RTPHeaderExtension.Video, pd.PlayoutDelayURI)
	}

	// Some of the browser clients do not handle H.264 High Profile in signalling properly.
	// They still decode if the actual stream is H.264 High Profile, but do not handle it well in signalling.
	// So, disable H.264 High Profile for SUBSCRIBER peer connection to ensure it is not offered.
	me, err := createMediaEngine(params.EnabledCodecs, directionConfig, params.IsOfferer)
	if err != nil {
		return nil, nil, nil, err
	}

	se := params.Config.SettingEngine
	se.DisableMediaEngineCopy(true)
	// simulcast layer disable/enable signalled via signalling channel,
	// so disable rid pause in SDP
	se.SetIgnoreRidPauseForRecv(true)

	// Change elliptic curve to improve connectivity
	// https://github.com/pion/dtls/pull/474
	se.SetDTLSEllipticCurves(elliptic.X25519, elliptic.P384, elliptic.P256)

	// Disable close by dtls to avoid peerconnection close too early in migration
	// https://github.com/pion/webrtc/pull/2961
	se.DisableCloseByDTLS(true)

	se.DetachDataChannels()
	if params.DatachannelSlowThreshold > 0 {
		se.EnableDataChannelBlockWrite(true)
	}
	if params.DatachannelMaxReceiverBufferSize > 0 {
		se.SetSCTPMaxReceiveBufferSize(uint32(params.DatachannelMaxReceiverBufferSize))
	}
	if params.FireOnTrackBySdp {
		se.SetFireOnTrackBeforeFirstRTP(true)
	}

	if params.ClientInfo.SupportsSctpZeroChecksum() {
		se.EnableSCTPZeroChecksum(true)
	}

	//
	// Disable SRTP replay protection (https://datatracker.ietf.org/doc/html/rfc3711#page-15).
	// Needed due to lack of RTX stream support in Pion.
	//
	// When clients probe for bandwidth, there are several possible approaches
	//   1. Use padding packet (Chrome uses this)
	//   2. Use an older packet (Firefox uses this)
	// Typically, these are sent over the RTX stream and hence SRTP replay protection will not
	// trigger. As Pion does not support RTX, when firefox uses older packet for probing, they
	// trigger the replay protection.
	//
	// That results in two issues
	//   - Firefox bandwidth probing is not successful
	//   - Pion runs out of read buffer capacity - this potentially looks like a Pion issue
	//
	// NOTE: It is not required to disable RTCP replay protection, but doing it to be symmetric.
	//
	se.DisableSRTPReplayProtection(true)
	se.DisableSRTCPReplayProtection(true)
	if !params.ProtocolVersion.SupportsICELite() || !params.ClientInfo.SupportsPrflxOverRelay() {
		// if client don't support prflx over relay which is only Firefox, disable ICE Lite to ensure that
		// aggressive nomination is handled properly. Firefox does aggressive nomination even if peer is
		// ICE Lite (see comment as to historical reasons: https://github.com/pion/ice/pull/739#issuecomment-2452245066).
		// pion/ice (as of v2.3.37) will accept all use-candidate switches when in ICE Lite mode.
		// That combined with aggressive nomination from Firefox could potentially lead to the two ends
		// ending up with different candidates.
		// As Firefox does not support migration, ICE Lite can be disabled.
		se.SetLite(false)
	}
	se.SetDTLSRetransmissionInterval(dtlsRetransmissionInterval)
	se.SetDTLSConnectContextMaker(func() (context.Context, func()) {
		return context.WithTimeout(context.Background(), dtlsHandshakeTimeout)
	})
	se.SetICETimeouts(iceDisconnectedTimeout, iceFailedTimeout, iceKeepaliveInterval)

	// if client don't support prflx over relay, we should not expose private address to it, use single external ip as host candidate
	if !params.ClientInfo.SupportsPrflxOverRelay() && len(params.Config.NAT1To1IPs) > 0 {
		var nat1to1Ips []string
		var includeIps []string
		for _, mapping := range params.Config.NAT1To1IPs {
			if ips := strings.Split(mapping, "/"); len(ips) == 2 {
				if ips[0] != ips[1] {
					nat1to1Ips = append(nat1to1Ips, mapping)
					includeIps = append(includeIps, ips[1])
				}
			}
		}
		if len(nat1to1Ips) > 0 {
			params.Logger.Infow("client doesn't support prflx over relay, use external ip only as host candidate", "ips", nat1to1Ips)
			se.SetNAT1To1IPs(nat1to1Ips, webrtc.ICECandidateTypeHost)
			se.SetIPFilter(func(ip net.IP) bool {
				if ip.To4() == nil {
					return true
				}
				ipstr := ip.String()
				return slices.Contains(includeIps, ipstr)
			})
		}
	}

	lf := pionlogger.NewLoggerFactory(params.Logger)
	if lf != nil {
		se.LoggerFactory = lf
	}

	ir := &interceptor.Registry{}
	if params.IsSendSide {
		if params.CongestionControlConfig.UseSendSideBWEInterceptor && !params.CongestionControlConfig.UseSendSideBWE {
			params.Logger.Infow("using send side BWE - interceptor")
			gf, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
				return gcc.NewSendSideBWE(
					gcc.SendSideBWEInitialBitrate(1*1000*1000),
					gcc.SendSideBWEPacer(gcc.NewNoOpPacer()),
				)
			})
			if err == nil {
				gf.OnNewPeerConnection(func(id string, estimator cc.BandwidthEstimator) {
					if onBandwidthEstimator != nil {
						onBandwidthEstimator(estimator)
					}
				})
				ir.Add(gf)

				tf, err := twcc.NewHeaderExtensionInterceptor()
				if err == nil {
					ir.Add(tf)
				}
			}
		}
	}
	if !params.IsOfferer {
		// sfu only use interceptor to send XR but don't read response from it (use buffer instead),
		// so use a empty callback here
		ir.Add(lkinterceptor.NewRTTFromXRFactory(func(rtt uint32) {}))
	}
	if len(params.SimTracks) > 0 {
		f, err := sfuinterceptor.NewUnhandleSimulcastInterceptorFactory(sfuinterceptor.UnhandleSimulcastTracks(params.Logger, params.SimTracks))
		if err != nil {
			params.Logger.Warnw("NewUnhandleSimulcastInterceptorFactory failed", err)
		} else {
			ir.Add(f)
		}
	}

	setTWCCForVideo := func(info *interceptor.StreamInfo) {
		if !mime.IsMimeTypeStringVideo(info.MimeType) {
			return
		}
		// rtx stream don't have rtcp feedback, always set twcc for rtx stream
		twccFb := mime.GetMimeTypeCodec(info.MimeType) == mime.MimeTypeCodecRTX
		if !twccFb {
			for _, fb := range info.RTCPFeedback {
				if fb.Type == webrtc.TypeRTCPFBTransportCC {
					twccFb = true
					break
				}
			}
		}
		if !twccFb {
			return
		}

		twccExtID := sfuutils.GetHeaderExtensionID(info.RTPHeaderExtensions, webrtc.RTPHeaderExtensionCapability{URI: sdp.TransportCCURI})
		if twccExtID != 0 {
			if buffer := params.Config.BufferFactory.GetBuffer(info.SSRC); buffer != nil {
				params.Logger.Debugw(
					"set twcc and ext id",
					"ssrc", info.SSRC,
					"isRTX", mime.GetMimeTypeCodec(info.MimeType) == mime.MimeTypeCodecRTX,
					"twccExtID", twccExtID,
				)
				buffer.SetTWCCAndExtID(params.Twcc, uint8(twccExtID))
			} else {
				params.Logger.Warnw("failed to get buffer for stream", nil, "ssrc", info.SSRC)
			}
		}
	}
	rtxInfoExtractorFactory := sfuinterceptor.NewRTXInfoExtractorFactory(
		setTWCCForVideo,
		func(repair, base uint32, rsid string) {
			params.Logger.Debugw("rtx pair found from extension", "repair", repair, "base", base, "rsid", rsid)
			params.Config.BufferFactory.SetRTXPair(repair, base, rsid)
		},
		params.Logger,
	)
	// put rtx interceptor behind unhandle simulcast interceptor so it can get the correct mid & rid
	ir.Add(rtxInfoExtractorFactory)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(me),
		webrtc.WithSettingEngine(se),
		webrtc.WithInterceptorRegistry(ir),
	)
	pc, err := api.NewPeerConnection(params.Config.Configuration)
	return pc, me, rtxInfoExtractorFactory, err
}

func NewPCTransport(params TransportParams) (*PCTransport, error) {
	if params.Logger == nil {
		params.Logger = logger.GetLogger()
	}
	t := &PCTransport{
		params:             params,
		debouncedNegotiate: sfuutils.NewDebouncer(negotiationFrequency),
		negotiationState:   transport.NegotiationStateNone,
		eventsQueue: utils.NewTypedOpsQueue[event](utils.OpsQueueParams{
			Name:    "transport",
			MinSize: 64,
			Logger:  params.Logger,
		}),
		previousTrackDescription: make(map[string]*trackDescription),
		canReuseTransceiver:      true,
		connectionDetails:        types.NewICEConnectionDetails(params.Transport, params.Logger),
		lastNegotiate:            time.Now(),
	}
	t.localOfferId.Store(uint32(rand.Intn(1<<8) + 1))

	bwe, err := t.createPeerConnection()
	if err != nil {
		return nil, err
	}

	if params.IsSendSide {
		if params.CongestionControlConfig.UseSendSideBWE {
			params.Logger.Infow("using send side BWE", "pacerBehavior", params.CongestionControlConfig.SendSideBWEPacer)
			t.bwe = sendsidebwe.NewSendSideBWE(sendsidebwe.SendSideBWEParams{
				Config: params.CongestionControlConfig.SendSideBWE,
				Logger: params.Logger,
			})
			switch pacer.PacerBehavior(params.CongestionControlConfig.SendSideBWEPacer) {
			case pacer.PacerBehaviorPassThrough:
				t.pacer = pacer.NewPassThrough(params.Logger, t.bwe)
			case pacer.PacerBehaviorNoQueue:
				t.pacer = pacer.NewNoQueue(params.Logger, t.bwe)
			default:
				t.pacer = pacer.NewNoQueue(params.Logger, t.bwe)
			}
		} else {
			t.bwe = remotebwe.NewRemoteBWE(remotebwe.RemoteBWEParams{
				Config: params.CongestionControlConfig.RemoteBWE,
				Logger: params.Logger,
			})
			t.pacer = pacer.NewPassThrough(params.Logger, nil)
		}

		t.streamAllocator = streamallocator.NewStreamAllocator(streamallocator.StreamAllocatorParams{
			Config:    params.CongestionControlConfig.StreamAllocator,
			BWE:       t.bwe,
			Pacer:     t.pacer,
			RTTGetter: t.GetRTT,
			Logger:    params.Logger.WithComponent(utils.ComponentCongestionControl),
		}, params.CongestionControlConfig.Enabled, params.CongestionControlConfig.AllowPause)
		t.streamAllocator.OnStreamStateChange(params.Handler.OnStreamStateChange)
		t.streamAllocator.Start()

		if bwe != nil {
			t.streamAllocator.SetSendSideBWEInterceptor(bwe)
		}
	}

	t.eventsQueue.Start()

	return t, nil
}

func (t *PCTransport) createPeerConnection() (cc.BandwidthEstimator, error) {
	var bwe cc.BandwidthEstimator
	pc, me, rtxInfoExtractorFactory, err := newPeerConnection(t.params, func(estimator cc.BandwidthEstimator) {
		bwe = estimator
	})
	if err != nil {
		return bwe, err
	}

	t.pc = pc
	if !t.params.UseOneShotSignallingMode {
		// one shot signalling mode gathers all candidates and sends in answer
		t.pc.OnICEGatheringStateChange(t.onICEGatheringStateChange)
		t.pc.OnICECandidate(t.onICECandidateTrickle)
	}
	t.pc.OnICEConnectionStateChange(t.onICEConnectionStateChange)
	t.pc.OnConnectionStateChange(t.onPeerConnectionStateChange)

	t.pc.OnDataChannel(t.onDataChannel)
	t.pc.OnTrack(t.params.Handler.OnTrack)

	t.iceTransport = t.pc.SCTP().Transport().ICETransport()
	if t.iceTransport == nil {
		return bwe, ErrNoICETransport
	}
	t.iceTransport.OnSelectedCandidatePairChange(func(pair *webrtc.ICECandidatePair) {
		t.params.Logger.Debugw("selected ICE candidate pair changed", "pair", wrappedICECandidatePairLogger{pair})
		t.connectionDetails.SetSelectedPair(pair)
		existingPair := t.selectedPair.Load()
		if existingPair != nil {
			t.params.Logger.Infow(
				"ice reconnected or switched pair",
				"existingPair", wrappedICECandidatePairLogger{existingPair},
				"newPair", wrappedICECandidatePairLogger{pair})
		}
		t.selectedPair.Store(pair)
	})

	t.me = me

	t.rtxInfoExtractorFactory = rtxInfoExtractorFactory
	return bwe, nil
}

func (t *PCTransport) GetPacer() pacer.Pacer {
	return t.pacer
}

func (t *PCTransport) SetSignalingRTT(rtt uint32) {
	t.signalingRTT.Store(rtt)
}

func (t *PCTransport) resetShortConn() {
	t.params.Logger.Infow("resetting short connection on ICE restart")
	t.lock.Lock()
	t.iceStartedAt = time.Time{}
	t.iceConnectedAt = time.Time{}
	t.connectedAt = time.Time{}
	if t.connectAfterICETimer != nil {
		t.connectAfterICETimer.Stop()
		t.connectAfterICETimer = nil
	}
	if t.tcpICETimer != nil {
		t.tcpICETimer.Stop()
		t.tcpICETimer = nil
	}
	t.lock.Unlock()
}

func (t *PCTransport) IsShortConnection(at time.Time) (bool, time.Duration) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if t.iceConnectedAt.IsZero() {
		return false, 0
	}

	duration := at.Sub(t.iceConnectedAt)
	return duration < shortConnectionThreshold, duration
}

func (t *PCTransport) setConnectedAt(at time.Time) bool {
	t.lock.Lock()
	t.connectedAt = at
	if !t.firstConnectedAt.IsZero() {
		t.lock.Unlock()
		return false
	}

	t.firstConnectedAt = at
	prometheus.RecordServiceOperationSuccess("peer_connection")
	t.lock.Unlock()
	return true
}

func (t *PCTransport) handleConnectionFailed(forceShortConn bool) {
	isShort := forceShortConn
	if !isShort {
		var duration time.Duration
		isShort, duration = t.IsShortConnection(time.Now())
		if isShort {
			t.params.Logger.Debugw("short ICE connection", "pair", wrappedICECandidatePairLogger{t.selectedPair.Load()}, "duration", duration)
		}
	}

	t.params.Handler.OnFailed(isShort, t.GetICEConnectionInfo())
}

func (t *PCTransport) onPeerConnectionStateChange(state webrtc.PeerConnectionState) {
	t.params.Logger.Debugw("peer connection state change", "state", state.String())
	switch state {
	case webrtc.PeerConnectionStateConnected:
		t.clearConnTimer()
		isInitialConnection := t.setConnectedAt(time.Now())
		if isInitialConnection {
			t.params.Handler.OnInitialConnected()

			t.maybeNotifyFullyEstablished()
		}
	case webrtc.PeerConnectionStateFailed:
		t.clearConnTimer()
		t.handleConnectionFailed(false)
	}
}

func (t *PCTransport) onDataChannel(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		t.params.Logger.Debugw(dc.Label() + " data channel open")
		var kind hublive.DataPacket_Kind
		var isDataTrack bool
		var isUnlabeled bool
		switch dc.Label() {
		case ReliableDataChannel:
			kind = hublive.DataPacket_RELIABLE

		case LossyDataChannel:
			kind = hublive.DataPacket_LOSSY

		case DataTrackDataChannel:
			isDataTrack = true

		default:
			t.params.Logger.Infow("unlabeled datachannel added", "label", dc.Label())
			isUnlabeled = true
		}

		rawDC, err := dc.DetachWithDeadline()
		if err != nil {
			t.params.Logger.Errorw("failed to detach data channel", err, "label", dc.Label())
			return
		}

		isHandled := true
		t.lock.Lock()
		switch {
		case isUnlabeled:
			t.unlabeledDataChannels = append(
				t.unlabeledDataChannels,
				datachannel.NewDataChannelWriterReliable(dc, rawDC, t.params.DatachannelSlowThreshold),
			)

		case isDataTrack:
			if !t.params.EnableDataTracks {
				t.params.Logger.Debugw("data tracks not enabled")
				isHandled = false
			} else {
				if t.dataTrackDC != nil {
					t.dataTrackDC.Close()
				}
				t.dataTrackDC = datachannel.NewDataChannelWriterUnreliable(dc, rawDC, 0, 0)
			}

		case kind == hublive.DataPacket_RELIABLE:
			if t.reliableDC != nil {
				t.reliableDC.Close()
			}
			t.reliableDC = datachannel.NewDataChannelWriterReliable(dc, rawDC, t.params.DatachannelSlowThreshold)
			t.reliableDCOpened = true

		case kind == hublive.DataPacket_LOSSY:
			if t.lossyDC != nil {
				t.lossyDC.Close()
			}
			t.lossyDC = datachannel.NewDataChannelWriterUnreliable(dc, rawDC, t.params.DatachannelLossyTargetLatency, uint64(lossyDataChannelMinBufferedAmount))
			t.lossyDCOpened = true
		}
		t.lock.Unlock()

		if !isHandled {
			rawDC.Close()
			return
		}

		go func() {
			defer rawDC.Close()
			buffer := make([]byte, dataChannelBufferSize)
			for {
				n, _, err := rawDC.ReadDataChannel(buffer)
				if err != nil {
					if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "state=Closed") {
						t.params.Logger.Warnw("error reading data channel", err, "label", dc.Label())
					}
					return
				}

				switch {
				case isUnlabeled:
					t.params.Handler.OnDataMessageUnlabeled(buffer[:n])

				case isDataTrack:
					t.params.Handler.OnDataTrackMessage(buffer[:n], mono.UnixNano())

				default:
					t.params.Handler.OnDataMessage(kind, buffer[:n])
				}
			}
		}()

		t.maybeNotifyFullyEstablished()
	})
}

func (t *PCTransport) maybeNotifyFullyEstablished() {
	if t.isFullyEstablished() {
		t.params.Handler.OnFullyEstablished()
	}
}

func (t *PCTransport) isFullyEstablished() bool {
	t.lock.RLock()
	defer t.lock.RUnlock()

	dataChannelReady := t.params.UseOneShotSignallingMode || t.firstOfferNoDataChannel || (t.reliableDCOpened && t.lossyDCOpened)

	return dataChannelReady && !t.connectedAt.IsZero()
}

func (t *PCTransport) AddTrack(
	trackLocal webrtc.TrackLocal,
	params types.AddTrackParams,
	enabledCodecs []*hublive.Codec,
	rtcpFeedbackConfig RTCPFeedbackConfig,
) (sender *webrtc.RTPSender, transceiver *webrtc.RTPTransceiver, err error) {
	t.lock.Lock()
	canReuse := t.canReuseTransceiver
	td, ok := t.previousTrackDescription[trackLocal.ID()]
	if ok {
		delete(t.previousTrackDescription, trackLocal.ID())
	}
	t.lock.Unlock()

	// keep track use same mid after migration if possible
	if td != nil && td.sender != nil {
		for _, tr := range t.pc.GetTransceivers() {
			if tr.Mid() == td.mid {
				return td.sender, tr, tr.SetSender(td.sender, trackLocal)
			}
		}
	}

	// if never negotiated with client, can't reuse transceiver for track not subscribed before migration
	if !canReuse {
		return t.AddTransceiverFromTrack(trackLocal, params, enabledCodecs, rtcpFeedbackConfig)
	}

	sender, err = t.pc.AddTrack(trackLocal)
	if err != nil {
		return
	}

	for _, tr := range t.pc.GetTransceivers() {
		if tr.Sender() == sender {
			transceiver = tr
			break
		}
	}

	if transceiver == nil {
		err = ErrNoTransceiver
		return
	}

	t.queueOrConfigureSender(
		transceiver,
		enabledCodecs,
		rtcpFeedbackConfig,
		params.Stereo,
		!params.Red || !t.params.ClientInfo.SupportsAudioRED(),
	)

	t.adjustNumOutstandingMedia(transceiver)
	return
}

func (t *PCTransport) CreateDataChannel(label string, dci *webrtc.DataChannelInit) error {
	if label == DataTrackDataChannel && !t.params.EnableDataTracks {
		t.params.Logger.Debugw("data tracks not enabled")
		return nil
	}

	dc, err := t.pc.CreateDataChannel(label, dci)
	if err != nil {
		return err
	}
	var (
		dcPtr       **datachannel.DataChannelWriter[*webrtc.DataChannel]
		dcReady     *bool
		isDataTrack bool
		isUnlabeled bool
		kind        hublive.DataPacket_Kind
	)
	switch dc.Label() {
	default:
		isUnlabeled = true
		t.params.Logger.Infow("unlabeled datachannel added", "label", dc.Label())

	case ReliableDataChannel:
		dcPtr = &t.reliableDC
		dcReady = &t.reliableDCOpened
		kind = hublive.DataPacket_RELIABLE

	case LossyDataChannel:
		dcPtr = &t.lossyDC
		dcReady = &t.lossyDCOpened
		kind = hublive.DataPacket_LOSSY

	case DataTrackDataChannel:
		dcPtr = &t.dataTrackDC
		isDataTrack = true
	}

	dc.OnOpen(func() {
		rawDC, err := dc.DetachWithDeadline()
		if err != nil {
			t.params.Logger.Warnw("failed to detach data channel", err)
			return
		}

		var slowThreshold int
		if dc.Label() == ReliableDataChannel || isUnlabeled {
			slowThreshold = t.params.DatachannelSlowThreshold
		}

		t.lock.Lock()
		if isUnlabeled {
			t.unlabeledDataChannels = append(
				t.unlabeledDataChannels,
				datachannel.NewDataChannelWriterReliable(dc, rawDC, slowThreshold),
			)
		} else {
			if *dcPtr != nil {
				(*dcPtr).Close()
			}
			switch {
			case dcPtr == &t.reliableDC:
				*dcPtr = datachannel.NewDataChannelWriterReliable(dc, rawDC, slowThreshold)
			case dcPtr == &t.lossyDC:
				*dcPtr = datachannel.NewDataChannelWriterUnreliable(dc, rawDC, t.params.DatachannelLossyTargetLatency, uint64(lossyDataChannelMinBufferedAmount))
			case dcPtr == &t.dataTrackDC:
				*dcPtr = datachannel.NewDataChannelWriterUnreliable(dc, rawDC, 0, 0)
			}
			if dcReady != nil {
				*dcReady = true
			}
		}
		t.lock.Unlock()
		t.params.Logger.Debugw(dc.Label() + " data channel open")

		go func() {
			defer rawDC.Close()
			buffer := make([]byte, dataChannelBufferSize)
			for {
				n, _, err := rawDC.ReadDataChannel(buffer)
				if err != nil {
					if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "state=Closed") {
						t.params.Logger.Warnw("error reading data channel", err, "label", dc.Label())
					}
					return
				}

				switch {
				case isUnlabeled:
					t.params.Handler.OnDataMessageUnlabeled(buffer[:n])

				case isDataTrack:
					t.params.Handler.OnDataTrackMessage(buffer[:n], mono.UnixNano())

				default:
					t.params.Handler.OnDataMessage(kind, buffer[:n])
				}
			}
		}()

		t.maybeNotifyFullyEstablished()
	})

	return nil
}

// for testing only
func (t *PCTransport) CreateReadableDataChannel(label string, dci *webrtc.DataChannelInit) error {
	dc, err := t.pc.CreateDataChannel(label, dci)
	if err != nil {
		return err
	}

	dc.OnOpen(func() {
		t.params.Logger.Debugw(dc.Label() + " data channel open")
		rawDC, err := dc.DetachWithDeadline()
		if err != nil {
			t.params.Logger.Errorw("failed to detach data channel", err, "label", dc.Label())
			return
		}

		t.lock.Lock()
		t.unlabeledDataChannels = append(
			t.unlabeledDataChannels,
			datachannel.NewDataChannelWriterReliable(dc, rawDC, t.params.DatachannelSlowThreshold),
		)
		t.lock.Unlock()

		go func() {
			defer rawDC.Close()
			buffer := make([]byte, dataChannelBufferSize)
			for {
				n, _, err := rawDC.ReadDataChannel(buffer)
				if err != nil {
					if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "state=Closed") {
						t.params.Logger.Warnw("error reading data channel", err, "label", dc.Label())
					}
					return
				}

				t.params.Handler.OnDataMessageUnlabeled(buffer[:n])
			}
		}()
	})
	return nil
}

func (t *PCTransport) CreateDataChannelIfEmpty(dcLabel string, dci *webrtc.DataChannelInit) (label string, id uint16, existing bool, err error) {
	if dcLabel == DataTrackDataChannel && !t.params.EnableDataTracks {
		t.params.Logger.Debugw("data tracks not enabled")
		err = errors.New("data tracks not enabled")
		return
	}

	t.lock.RLock()
	var dcw *datachannel.DataChannelWriter[*webrtc.DataChannel]
	switch dcLabel {
	case ReliableDataChannel:
		dcw = t.reliableDC
	case LossyDataChannel:
		dcw = t.lossyDC
	case DataTrackDataChannel:
		dcw = t.dataTrackDC
	default:
		t.params.Logger.Warnw("unknown data channel label", nil, "label", label)
		err = errors.New("unknown data channel label")
	}
	t.lock.RUnlock()
	if err != nil {
		return
	}

	if dcw != nil {
		dc := dcw.BufferedAmountGetter()
		return dc.Label(), *dc.ID(), true, nil
	}

	dc, err := t.pc.CreateDataChannel(dcLabel, dci)
	if err != nil {
		return
	}

	t.onDataChannel(dc)
	return dc.Label(), *dc.ID(), false, nil
}

func (t *PCTransport) GetRTT() (float64, bool) {
	scps, ok := t.iceTransport.GetSelectedCandidatePairStats()
	if !ok {
		return 0.0, false
	}

	return scps.CurrentRoundTripTime, true
}

func (t *PCTransport) IsEstablished() bool {
	return t.pc.ConnectionState() != webrtc.PeerConnectionStateNew
}

func (t *PCTransport) HasEverConnected() bool {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return !t.firstConnectedAt.IsZero()
}

func (t *PCTransport) WriteRTCP(pkts []rtcp.Packet) error {
	return t.pc.WriteRTCP(pkts)
}

func (t *PCTransport) SendDataMessage(kind hublive.DataPacket_Kind, data []byte) error {
	convertFromUserPacket := false
	var dc *datachannel.DataChannelWriter[*webrtc.DataChannel]
	t.lock.RLock()
	if t.params.UseOneShotSignallingMode {
		if len(t.unlabeledDataChannels) > 0 {
			// use the first unlabeled to send
			dc = t.unlabeledDataChannels[0]
		}
		convertFromUserPacket = true
	} else {
		if kind == hublive.DataPacket_RELIABLE {
			dc = t.reliableDC
		} else {
			dc = t.lossyDC
		}
	}
	t.lock.RUnlock()

	if convertFromUserPacket {
		dp := &hublive.DataPacket{}
		if err := proto.Unmarshal(data, dp); err != nil {
			return err
		}

		switch payload := dp.Value.(type) {
		case *hublive.DataPacket_User:
			return t.sendDataMessage(dc, payload.User.Payload)
		default:
			return errors.New("cannot forward non user data packet")
		}
	}

	return t.sendDataMessage(dc, data)
}

func (t *PCTransport) SendDataMessageUnlabeled(data []byte, useRaw bool, sender hublive.ParticipantIdentity) error {
	convertToUserPacket := false
	var dc *datachannel.DataChannelWriter[*webrtc.DataChannel]
	t.lock.RLock()
	if t.params.UseOneShotSignallingMode || useRaw {
		if len(t.unlabeledDataChannels) > 0 {
			// use the first unlabeled to send
			dc = t.unlabeledDataChannels[0]
		}
	} else {
		if t.reliableDC != nil {
			dc = t.reliableDC
		} else if t.lossyDC != nil {
			dc = t.lossyDC
		}

		convertToUserPacket = true
	}
	t.lock.RUnlock()

	if convertToUserPacket {
		dpData, err := proto.Marshal(&hublive.DataPacket{
			ParticipantIdentity: string(sender),
			Value: &hublive.DataPacket_User{
				User: &hublive.UserPacket{Payload: data},
			},
		})
		if err != nil {
			return err
		}
		return t.sendDataMessage(dc, dpData)
	}

	return t.sendDataMessage(dc, data)
}

func (t *PCTransport) SendDataTrackMessage(data []byte) error {
	t.lock.RLock()
	dc := t.dataTrackDC
	t.lock.RUnlock()

	return t.sendDataMessage(dc, data)
}

func (t *PCTransport) sendDataMessage(dc *datachannel.DataChannelWriter[*webrtc.DataChannel], data []byte) error {
	if dc == nil {
		return ErrDataChannelUnavailable
	}

	if t.pc.ConnectionState() == webrtc.PeerConnectionStateFailed {
		return ErrTransportFailure
	}

	if t.params.DatachannelSlowThreshold == 0 && t.params.DataChannelMaxBufferedAmount > 0 && dc.BufferedAmountGetter().BufferedAmount() > t.params.DataChannelMaxBufferedAmount {
		return ErrDataChannelBufferFull
	}
	_, err := dc.Write(data)
	return err
}

func (t *PCTransport) Close() {
	if t.isClosed.Swap(true) {
		return
	}

	<-t.eventsQueue.Stop()
	t.clearSignalStateCheckTimer()

	if t.streamAllocator != nil {
		t.streamAllocator.Stop()
	}

	if t.pacer != nil {
		t.pacer.Stop()
	}

	t.clearConnTimer()

	t.lock.Lock()
	if t.mayFailedICEStatsTimer != nil {
		t.mayFailedICEStatsTimer.Stop()
		t.mayFailedICEStatsTimer = nil
	}

	if t.reliableDC != nil {
		t.reliableDC.Close()
		t.reliableDC = nil
	}

	if t.lossyDC != nil {
		t.lossyDC.Close()
		t.lossyDC = nil
	}

	if t.dataTrackDC != nil {
		t.dataTrackDC.Close()
		t.dataTrackDC = nil
	}

	for _, dc := range t.unlabeledDataChannels {
		dc.Close()
	}
	t.unlabeledDataChannels = nil
	t.lock.Unlock()

	if err := t.pc.Close(); err != nil {
		t.params.Logger.Warnw("unclean close of peer connection", err)
	}

	t.outputAndClearICEStats()
}

func (t *PCTransport) clearConnTimer() {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.connectAfterICETimer != nil {
		t.connectAfterICETimer.Stop()
		t.connectAfterICETimer = nil
	}

	if t.tcpICETimer != nil {
		t.tcpICETimer.Stop()
		t.tcpICETimer = nil
	}
}

func (t *PCTransport) AddTrackToStreamAllocator(subTrack types.SubscribedTrack) {
	if t.streamAllocator == nil {
		return
	}

	layers := buffer.GetVideoLayersForMimeType(
		subTrack.DownTrack().Mime(),
		subTrack.MediaTrack().ToProto(),
	)
	t.streamAllocator.AddTrack(subTrack.DownTrack(), streamallocator.AddTrackParams{
		Source:         subTrack.MediaTrack().Source(),
		IsMultiLayered: len(layers) > 1,
		PublisherID:    subTrack.MediaTrack().PublisherID(),
	})
}

func (t *PCTransport) RemoveTrackFromStreamAllocator(subTrack types.SubscribedTrack) {
	if t.streamAllocator == nil {
		return
	}

	t.streamAllocator.RemoveTrack(subTrack.DownTrack())
}

func (t *PCTransport) SetAllowPauseOfStreamAllocator(allowPause bool) {
	if t.streamAllocator == nil {
		return
	}

	t.streamAllocator.SetAllowPause(allowPause)
}

func (t *PCTransport) SetChannelCapacityOfStreamAllocator(channelCapacity int64) {
	if t.streamAllocator == nil {
		return
	}

	t.streamAllocator.SetChannelCapacity(channelCapacity)
}

func (t *PCTransport) postEvent(e event) {
	e.PCTransport = t
	t.eventsQueue.Enqueue(func(e event) {
		var err error
		switch e.signal {
		case signalICEGatheringComplete:
			err = e.handleICEGatheringComplete(e)
		case signalLocalICECandidate:
			err = e.handleLocalICECandidate(e)
		case signalRemoteICECandidate:
			err = e.handleRemoteICECandidate(e)
		case signalSendOffer:
			err = e.handleSendOffer(e)
		case signalRemoteDescriptionReceived:
			err = e.handleRemoteDescriptionReceived(e)
		case signalICERestart:
			err = e.handleICERestart(e)
		}
		if err != nil {
			if !e.isClosed.Load() {
				e.onNegotiationFailed(true, fmt.Sprintf("error handling event. err: %s, event: %s", err, e))
			}
		}
	}, e)
}


