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

// transport_negotiation.go contains SDP negotiation and transceiver management
// methods for PCTransport. Split from transport.go for readability.

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"

	"github.com/maxhubsv/hublive-server/pkg/rtc/transport"
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/telemetry/prometheus"
	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	lksdp "__GITHUB_HUBLIVE__protocol/sdp"
)

func (t *PCTransport) RTPStreamPublished(ssrc uint32, mid, rid string) {
	t.rtxInfoExtractorFactory.SetStreamInfo(ssrc, mid, rid, "")
}

func (t *PCTransport) queueOrConfigureSender(
	transceiver *webrtc.RTPTransceiver,
	enabledCodecs []*hublive.Codec,
	rtcpFeedbackConfig RTCPFeedbackConfig,
	enableAudioStereo bool,
	enableAudioNACK bool,
) {
	params := configureSenderParams{
		transceiver,
		enabledCodecs,
		rtcpFeedbackConfig,
		!t.params.IsOfferer,
		enableAudioStereo,
		enableAudioNACK,
	}
	if !t.params.IsOfferer {
		t.sendersPendingConfigMu.Lock()
		t.sendersPendingConfig = append(t.sendersPendingConfig, params)
		t.sendersPendingConfigMu.Unlock()
		return
	}

	configureSender(params)
}

func (t *PCTransport) processSendersPendingConfig() {
	t.sendersPendingConfigMu.Lock()
	pending := t.sendersPendingConfig
	t.sendersPendingConfig = nil
	t.sendersPendingConfigMu.Unlock()

	var unprocessed []configureSenderParams
	for _, p := range pending {
		if p.transceiver.Mid() == "" {
			unprocessed = append(unprocessed, p)
			continue
		}

		configureSender(p)
	}

	if len(unprocessed) != 0 {
		t.sendersPendingConfigMu.Lock()
		t.sendersPendingConfig = append(t.sendersPendingConfig, unprocessed...)
		t.sendersPendingConfigMu.Unlock()
	}
}

func (t *PCTransport) AddTransceiverFromTrack(
	trackLocal webrtc.TrackLocal,
	params types.AddTrackParams,
	enabledCodecs []*hublive.Codec,
	rtcpFeedbackConfig RTCPFeedbackConfig,
) (sender *webrtc.RTPSender, transceiver *webrtc.RTPTransceiver, err error) {
	transceiver, err = t.pc.AddTransceiverFromTrack(trackLocal)
	if err != nil {
		return
	}

	sender = transceiver.Sender()
	if sender == nil {
		err = ErrNoSender
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

func (t *PCTransport) AddTransceiverFromKind(
	kind webrtc.RTPCodecType,
	init webrtc.RTPTransceiverInit,
) (*webrtc.RTPTransceiver, error) {
	return t.pc.AddTransceiverFromKind(kind, init)
}

func (t *PCTransport) RemoveTrack(sender *webrtc.RTPSender) error {
	return t.pc.RemoveTrack(sender)
}

func (t *PCTransport) CurrentLocalDescription() *webrtc.SessionDescription {
	cld := t.pc.CurrentLocalDescription()
	if cld == nil {
		return nil
	}

	ld := *cld
	return &ld
}

func (t *PCTransport) CurrentRemoteDescription() *webrtc.SessionDescription {
	crd := t.pc.CurrentRemoteDescription()
	if crd == nil {
		return nil
	}

	rd := *crd
	return &rd
}

func (t *PCTransport) PendingRemoteDescription() *webrtc.SessionDescription {
	prd := t.pc.PendingRemoteDescription()
	if prd == nil {
		return nil
	}

	rd := *prd
	return &rd
}

func (t *PCTransport) GetMid(rtpReceiver *webrtc.RTPReceiver) string {
	tr := rtpReceiver.RTPTransceiver()
	if tr != nil {
		return tr.Mid()
	}

	return ""
}

func (t *PCTransport) GetRTPTransceiver(mid string) *webrtc.RTPTransceiver {
	for _, tr := range t.pc.GetTransceivers() {
		if tr.Mid() == mid {
			return tr
		}
	}

	return nil
}

func (t *PCTransport) GetRTPReceiver(mid string) *webrtc.RTPReceiver {
	for _, tr := range t.pc.GetTransceivers() {
		if tr.Mid() == mid {
			return tr.Receiver()
		}
	}

	return nil
}

func (t *PCTransport) getNumUnmatchedTransceivers() (uint32, uint32) {
	if t.isClosed.Load() || t.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		return 0, 0
	}

	numAudios := uint32(0)
	numVideos := uint32(0)
	for _, tr := range t.pc.GetTransceivers() {
		if tr.Mid() != "" {
			continue
		}

		switch tr.Kind() {
		case webrtc.RTPCodecTypeAudio:
			numAudios++

		case webrtc.RTPCodecTypeVideo:
			numVideos++
		}
	}

	return numAudios, numVideos
}

func (t *PCTransport) HandleRemoteDescription(sd webrtc.SessionDescription, remoteId uint32) error {
	if t.params.UseOneShotSignallingMode {
		if sd.Type == webrtc.SDPTypeOffer {
			remoteOfferId := t.remoteOfferId.Load()
			if remoteOfferId != 0 && remoteOfferId != t.localAnswerId.Load() {
				t.params.Logger.Warnw(
					"sdp state: multiple offers without answer", nil,
					"remoteOfferId", remoteOfferId,
					"localAnswerId", t.localAnswerId.Load(),
					"receivedRemoteOfferId", remoteId,
				)
			}
			t.remoteOfferId.Store(remoteId)
		} else {
			if remoteId != 0 && remoteId != t.localOfferId.Load() {
				t.params.Logger.Warnw("sdp state: answer id mismatch", nil, "expected", t.localOfferId.Load(), "got", remoteId)
			}
			t.remoteAnswerId.Store(remoteId)
		}

		// add remote candidates to ICE connection details
		parsed, err := sd.Unmarshal()
		if err == nil {
			addRemoteICECandidates := func(attrs []sdp.Attribute) {
				for _, a := range attrs {
					if a.IsICECandidate() {
						c, err := ice.UnmarshalCandidate(a.Value)
						if err != nil {
							continue
						}
						t.connectionDetails.AddRemoteICECandidate(c, false, false, false)
					}
				}
			}

			addRemoteICECandidates(parsed.Attributes)
			for _, m := range parsed.MediaDescriptions {
				addRemoteICECandidates(m.Attributes)
			}
		}

		err = t.pc.SetRemoteDescription(sd)
		if err != nil {
			t.params.Logger.Errorw("could not set remote description on synchronous mode peer connection", err)
			return err
		}

		rtxRepairs := nonSimulcastRTXRepairsFromSDP(parsed, t.params.Logger)
		if len(rtxRepairs) > 0 {
			t.params.Logger.Debugw("rtx pairs found from sdp", "ssrcs", rtxRepairs)
			for repair, base := range rtxRepairs {
				t.params.Config.BufferFactory.SetRTXPair(repair, base, "")
			}
		}
		return nil
	}

	t.postEvent(event{
		signal: signalRemoteDescriptionReceived,
		data: remoteDescriptionData{
			sessionDescription: &sd,
			remoteId:           remoteId,
		},
	})
	return nil
}

func (t *PCTransport) GetAnswer() (webrtc.SessionDescription, uint32, error) {
	if !t.params.UseOneShotSignallingMode {
		return webrtc.SessionDescription{}, 0, ErrNotSynchronousLocalCandidatesMode
	}

	prd := t.pc.PendingRemoteDescription()
	if prd == nil || prd.Type != webrtc.SDPTypeOffer {
		return webrtc.SessionDescription{}, 0, ErrNoRemoteDescription
	}

	answer, err := t.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, 0, err
	}

	if err = t.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, 0, err
	}

	// wait for gathering to complete to include all candidates in the answer
	<-webrtc.GatheringCompletePromise(t.pc)

	cld := t.pc.CurrentLocalDescription()

	// add local candidates to ICE connection details
	parsed, err := cld.Unmarshal()
	if err == nil {
		addLocalICECandidates := func(attrs []sdp.Attribute) {
			for _, a := range attrs {
				if a.IsICECandidate() {
					c, err := ice.UnmarshalCandidate(a.Value)
					if err != nil {
						continue
					}
					t.connectionDetails.AddLocalICECandidate(c, false, false)
				}
			}
		}

		addLocalICECandidates(parsed.Attributes)
		for _, m := range parsed.MediaDescriptions {
			addLocalICECandidates(m.Attributes)
		}
	}

	answerId := t.remoteOfferId.Load()
	t.localAnswerId.Store(answerId)

	return *cld, answerId, nil
}

func (t *PCTransport) OnNegotiationStateChanged(f func(state transport.NegotiationState)) {
	t.lock.Lock()
	t.onNegotiationStateChanged = f
	t.lock.Unlock()
}

func (t *PCTransport) getOnNegotiationStateChanged() func(state transport.NegotiationState) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.onNegotiationStateChanged
}

func (t *PCTransport) Negotiate(force bool) {
	if t.isClosed.Load() {
		return
	}

	var postEvent bool
	t.lock.Lock()
	if force {
		t.debouncedNegotiate.Add(func() {
			// no op to cancel pending negotiation
		})
		t.debouncePending = false
		t.updateLastNegotiateLocked()

		postEvent = true
	} else {
		if !t.debouncePending {
			if time.Since(t.lastNegotiate) > negotiationFrequency {
				t.debouncedNegotiate.SetDuration(fastNegotiationFrequency)
			} else {
				t.debouncedNegotiate.SetDuration(negotiationFrequency)
			}

			t.debouncedNegotiate.Add(func() {
				t.lock.Lock()
				t.debouncePending = false
				t.updateLastNegotiateLocked()
				t.lock.Unlock()

				t.postEvent(event{
					signal: signalSendOffer,
				})
			})
			t.debouncePending = true
		}
	}
	t.lock.Unlock()

	if postEvent {
		t.postEvent(event{
			signal: signalSendOffer,
		})
	}
}

func (t *PCTransport) updateLastNegotiateLocked() {
	if now := time.Now(); now.After(t.lastNegotiate) {
		t.lastNegotiate = now
	}
}

func (t *PCTransport) preparePC(previousAnswer webrtc.SessionDescription) error {
	// sticky data channel to first m-lines, if someday we don't send sdp without media streams to
	// client's subscribe pc after joining, should change this step
	parsed, err := previousAnswer.Unmarshal()
	if err != nil {
		return err
	}
	fp, fpHahs, err := lksdp.ExtractFingerprint(parsed)
	if err != nil {
		return err
	}

	offer, err := t.pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := t.pc.SetLocalDescription(offer); err != nil {
		return err
	}

	//
	// Simulate client side peer connection and set DTLS role from previous answer.
	// Role needs to be set properly (one side needs to be server and the other side
	// needs to be the client) for DTLS connection to form properly. As this is
	// trying to replicate previous setup, read from previous answer and use that role.
	//
	se := webrtc.SettingEngine{}
	_ = se.SetAnsweringDTLSRole(lksdp.ExtractDTLSRole(parsed))
	se.SetIgnoreRidPauseForRecv(true)
	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(se),
		webrtc.WithMediaEngine(t.me),
	)
	pc2, err := api.NewPeerConnection(webrtc.Configuration{
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlan,
	})
	if err != nil {
		return err
	}
	defer pc2.Close()

	if err := pc2.SetRemoteDescription(offer); err != nil {
		return err
	}
	ans, err := pc2.CreateAnswer(nil)
	if err != nil {
		return err
	}

	// replace client's fingerprint into dummy pc's answer, for pion's dtls process, it will
	// keep the fingerprint at first call of SetRemoteDescription, if dummy pc and client pc use
	// different fingerprint, that will cause pion denied dtls data after handshake with client
	// complete (can't pass fingerprint change).
	// in this step, we don't established connection with dummy pc(no candidate swap), just use
	// sdp negotiation to sticky data channel and keep client's fingerprint
	parsedAns, _ := ans.Unmarshal()
	fpLine := fpHahs + " " + fp
	replaceFP := func(attrs []sdp.Attribute, fpLine string) {
		for k := range attrs {
			if attrs[k].Key == "fingerprint" {
				attrs[k].Value = fpLine
			}
		}
	}
	replaceFP(parsedAns.Attributes, fpLine)
	for _, m := range parsedAns.MediaDescriptions {
		replaceFP(m.Attributes, fpLine)
	}
	bytes, err := parsedAns.Marshal()
	if err != nil {
		return err
	}
	ans.SDP = string(bytes)

	return t.pc.SetRemoteDescription(ans)
}

func (t *PCTransport) initPCWithPreviousAnswer(previousAnswer webrtc.SessionDescription) (map[string]*webrtc.RTPSender, error) {
	senders := make(map[string]*webrtc.RTPSender)
	parsed, err := previousAnswer.Unmarshal()
	if err != nil {
		return senders, err
	}
	for _, m := range parsed.MediaDescriptions {
		var codecType webrtc.RTPCodecType
		switch m.MediaName.Media {
		case "video":
			codecType = webrtc.RTPCodecTypeVideo
		case "audio":
			codecType = webrtc.RTPCodecTypeAudio
		case "application":
			if t.params.IsOfferer {
				// for pion generate unmatched sdp, it always appends data channel to last m-lines,
				// that not consistent with our previous answer that data channel might at middle-line
				// because sdp can negotiate multi times before migration.(it will sticky to the last m-line at first negotiate)
				// so use a dummy pc to negotiate sdp to fixed the datachannel's mid at same position with previous answer
				if err := t.preparePC(previousAnswer); err != nil {
					t.params.Logger.Warnw("prepare pc for migration failed", err)
					return senders, err
				}
			}
			continue
		default:
			continue
		}

		if !t.params.IsOfferer {
			// `sendrecv` or `sendonly` means this transceiver is used for sending

			// Note that a transceiver previously used to send could be `inactive`.
			// Let those transceivers be created when remote description is set.
			_, ok1 := m.Attribute(webrtc.RTPTransceiverDirectionSendrecv.String())
			_, ok2 := m.Attribute(webrtc.RTPTransceiverDirectionSendonly.String())
			if !ok1 && !ok2 {
				continue
			}
		}

		tr, err := t.pc.AddTransceiverFromKind(
			codecType,
			webrtc.RTPTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionSendonly,
			},
		)
		if err != nil {
			return senders, err
		}
		mid := lksdp.GetMidValue(m)
		if mid == "" {
			return senders, ErrMidNotFound
		}
		tr.SetMid(mid)

		// save mid -> senders for migration reuse
		sender := tr.Sender()
		senders[mid] = sender

		// set transceiver to inactive
		tr.SetSender(sender, nil)
	}
	return senders, nil
}

func (t *PCTransport) SetPreviousSdp(localDescription, remoteDescription *webrtc.SessionDescription) {
	// when there is no answer, cannot migrate, force a full reconnect
	if (t.params.IsOfferer && remoteDescription == nil) || (!t.params.IsOfferer && localDescription == nil) {
		t.onNegotiationFailed(true, "no previous answer")
		return
	}

	t.lock.Lock()
	var (
		senders   map[string]*webrtc.RTPSender
		err       error
		parseMids bool
	)
	if t.params.IsOfferer {
		if t.pc.RemoteDescription() == nil && t.previousAnswer == nil {
			t.previousAnswer = remoteDescription
			senders, err = t.initPCWithPreviousAnswer(*remoteDescription)
			parseMids = true
		}
	} else {
		if t.pc.LocalDescription() == nil {
			senders, err = t.initPCWithPreviousAnswer(*localDescription)
			parseMids = true
		}
	}
	if err != nil {
		t.lock.Unlock()
		t.onNegotiationFailed(true, fmt.Sprintf("initPCWithPreviousAnswer failed, error: %s", err))
		return
	}

	if localDescription != nil && parseMids {
		// in migration case, can't reuse transceiver before negotiating excepted tracks
		// that were subscribed at previous node
		t.canReuseTransceiver = false
		if err := t.parseTrackMid(*localDescription, senders); err != nil {
			t.params.Logger.Warnw(
				"parse previous local description failed", err,
				"localDescription", localDescription.SDP,
			)
		}
	}

	if t.params.IsOfferer {
		// disable fast negotiation temporarily after migration to avoid sending offer
		// contains part of subscribed tracks before migration, let the subscribed track
		// resume at the same time.
		t.lastNegotiate = time.Now().Add(iceFailedTimeoutTotal)
	}
	t.lock.Unlock()
}

func (t *PCTransport) parseTrackMid(sd webrtc.SessionDescription, senders map[string]*webrtc.RTPSender) error {
	parsed, err := sd.Unmarshal()
	if err != nil {
		return err
	}

	t.previousTrackDescription = make(map[string]*trackDescription)
	for _, m := range parsed.MediaDescriptions {
		msid, ok := m.Attribute(sdp.AttrKeyMsid)
		if !ok {
			continue
		}

		if split := strings.Split(msid, " "); len(split) == 2 {
			trackID := split[1]
			mid := lksdp.GetMidValue(m)
			if mid == "" {
				return ErrMidNotFound
			}
			if sender, ok := senders[mid]; ok {
				t.previousTrackDescription[trackID] = &trackDescription{mid, sender}
			}
		}
	}
	return nil
}

func (t *PCTransport) handleICEGatheringCompleteOfferer() error {
	if !t.restartAfterGathering {
		return nil
	}

	t.params.Logger.Debugw("restarting ICE after ICE gathering")
	t.restartAfterGathering = false
	return t.doICERestart()
}

func (t *PCTransport) handleICEGatheringCompleteAnswerer() error {
	if t.pendingRestartIceOffer == nil {
		return nil
	}

	offer := *t.pendingRestartIceOffer
	t.pendingRestartIceOffer = nil

	t.params.Logger.Debugw("accept remote restart ice offer after ICE gathering")
	if err := t.setRemoteDescription(offer); err != nil {
		return err
	}
	t.params.Handler.OnSetRemoteDescriptionOffer()
	t.processSendersPendingConfig()

	return t.createAndSendAnswer()
}

func (t *PCTransport) localDescriptionSent() error {
	if !t.cacheLocalCandidates {
		return nil
	}

	t.cacheLocalCandidates = false

	cachedLocalCandidates := t.cachedLocalCandidates
	t.cachedLocalCandidates = nil

	for _, c := range cachedLocalCandidates {
		if err := t.params.Handler.OnICECandidate(c, t.params.Transport); err != nil {
			t.params.Logger.Warnw("failed to send cached ICE candidate", err, "candidate", c)
			return err
		}
	}
	return nil
}

func (t *PCTransport) clearLocalDescriptionSent() {
	t.cacheLocalCandidates = true
	t.cachedLocalCandidates = nil
	t.connectionDetails.Clear()
}

func (t *PCTransport) setNegotiationState(state transport.NegotiationState) {
	t.negotiationState = state
	if onNegotiationStateChanged := t.getOnNegotiationStateChanged(); onNegotiationStateChanged != nil {
		onNegotiationStateChanged(t.negotiationState)
	}
}

func (t *PCTransport) clearSignalStateCheckTimer() {
	if t.signalStateCheckTimer != nil {
		t.signalStateCheckTimer.Stop()
		t.signalStateCheckTimer = nil
	}
}

func (t *PCTransport) setupSignalStateCheckTimer() {
	t.clearSignalStateCheckTimer()

	negotiateVersion := t.negotiateCounter.Inc()
	t.signalStateCheckTimer = time.AfterFunc(negotiationFailedTimeout, func() {
		t.clearSignalStateCheckTimer()

		failed := t.negotiationState != transport.NegotiationStateNone

		if t.negotiateCounter.Load() == negotiateVersion && failed && t.pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
			t.onNegotiationFailed(false, "negotiation timed out")
		}
	})
}

func (t *PCTransport) adjustNumOutstandingMedia(transceiver *webrtc.RTPTransceiver) {
	if transceiver.Mid() != "" {
		return
	}

	t.lock.Lock()
	if transceiver.Kind() == webrtc.RTPCodecTypeAudio {
		t.numOutstandingAudios++
	} else {
		t.numOutstandingVideos++
	}
	t.lock.Unlock()
}

func (t *PCTransport) sendUnmatchedMediaRequirement(force bool) error {
	// if there are unmatched media sections, notify remote peer to generate offer with
	// enough media section in subsequent offers
	t.lock.Lock()
	numAudios := t.numOutstandingAudios - t.numRequestSentAudios
	t.numRequestSentAudios += numAudios

	numVideos := t.numOutstandingVideos - t.numRequestSentVideos
	t.numRequestSentVideos += numVideos
	t.lock.Unlock()

	if force || (numAudios+numVideos) != 0 {
		if err := t.params.Handler.OnUnmatchedMedia(numAudios, numVideos); err != nil {
			return errors.Wrap(err, "could not send unmatched media requirements")
		}
	}

	return nil
}

func (t *PCTransport) createAndSendOffer(options *webrtc.OfferOptions) error {
	if t.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		t.params.Logger.Warnw("trying to send offer on closed peer connection", nil)
		return nil
	}

	// when there's an ongoing negotiation, let it finish and not disrupt its state
	if t.negotiationState == transport.NegotiationStateRemote {
		t.params.Logger.Debugw("skipping negotiation, trying again later")
		t.setNegotiationState(transport.NegotiationStateRetry)
		return nil
	} else if t.negotiationState == transport.NegotiationStateRetry {
		// already set to retry, we can safely skip this attempt
		return nil
	}

	ensureICERestart := func(options *webrtc.OfferOptions) *webrtc.OfferOptions {
		if options == nil {
			options = &webrtc.OfferOptions{}
		}
		options.ICERestart = true
		return options
	}

	t.lock.Lock()
	if t.previousAnswer != nil {
		t.previousAnswer = nil
		options = ensureICERestart(options)
		t.params.Logger.Infow("ice restart due to previous answer")
	}
	t.lock.Unlock()

	if t.restartAtNextOffer {
		t.restartAtNextOffer = false
		options = ensureICERestart(options)
		t.params.Logger.Infow("ice restart at next offer")
	}

	if options != nil && options.ICERestart {
		t.clearLocalDescriptionSent()
	}

	offer, err := t.pc.CreateOffer(options)
	if err != nil {
		if errors.Is(err, webrtc.ErrConnectionClosed) {
			t.params.Logger.Warnw("trying to create offer on closed peer connection", nil)
			return nil
		}

		prometheus.RecordServiceOperationError("offer", "create")
		return errors.Wrap(err, "create offer failed")
	}

	preferTCP := t.preferTCP.Load()
	if preferTCP {
		t.params.Logger.Debugw("local offer (unfiltered)", "sdp", offer.SDP)
	}

	err = t.pc.SetLocalDescription(offer)
	if err != nil {
		if errors.Is(err, webrtc.ErrConnectionClosed) {
			t.params.Logger.Warnw("trying to set local description on closed peer connection", nil)
			return nil
		}

		prometheus.RecordServiceOperationError("offer", "local_description")
		return errors.Wrap(err, "setting local description failed")
	}

	//
	// Filter after setting local description as pion expects the offer
	// to match between CreateOffer and SetLocalDescription.
	// Filtered offer is sent to remote so that remote does not
	// see filtered candidates.
	//
	offer = t.filterCandidates(offer, preferTCP, true)
	if preferTCP {
		t.params.Logger.Debugw("local offer (filtered)", "sdp", offer.SDP)
	}

	// indicate waiting for remote
	t.setNegotiationState(transport.NegotiationStateRemote)

	t.setupSignalStateCheckTimer()

	remoteAnswerId := t.remoteAnswerId.Load()
	if remoteAnswerId != 0 && remoteAnswerId != t.localOfferId.Load() {
		if options == nil || !options.ICERestart {
			t.params.Logger.Warnw(
				"sdp state: sending offer before receiving answer", nil,
				"localOfferId", t.localOfferId.Load(),
				"remoteAnswerId", remoteAnswerId,
			)
		}
	}

	if err := t.params.Handler.OnOffer(offer, t.localOfferId.Inc(), t.getMidToTrackIDMapping()); err != nil {
		prometheus.RecordServiceOperationError("offer", "write_message")
		return errors.Wrap(err, "could not send offer")
	}
	prometheus.RecordServiceOperationSuccess("offer")

	return t.localDescriptionSent()
}

func (t *PCTransport) handleSendOffer(_ event) error {
	if !t.params.IsOfferer {
		return t.sendUnmatchedMediaRequirement(true)
	}

	return t.createAndSendOffer(nil)
}

type remoteDescriptionData struct {
	sessionDescription *webrtc.SessionDescription
	remoteId           uint32
}

func (t *PCTransport) handleRemoteDescriptionReceived(e event) error {
	rdd := e.data.(remoteDescriptionData)
	if rdd.sessionDescription.Type == webrtc.SDPTypeOffer {
		return t.handleRemoteOfferReceived(rdd.sessionDescription, rdd.remoteId)
	} else {
		return t.handleRemoteAnswerReceived(rdd.sessionDescription, rdd.remoteId)
	}
}

func (t *PCTransport) isRemoteOfferRestartICE(parsed *sdp.SessionDescription) (string, bool, error) {
	user, pwd, err := lksdp.ExtractICECredential(parsed)
	if err != nil {
		return "", false, err
	}

	credential := fmt.Sprintf("%s:%s", user, pwd)
	// ice credential changed, remote offer restart ice
	restartICE := t.currentOfferIceCredential != "" && t.currentOfferIceCredential != credential
	return credential, restartICE, nil
}

func (t *PCTransport) setRemoteDescription(sd webrtc.SessionDescription) error {
	// filter before setting remote description so that pion does not see filtered remote candidates
	preferTCP := t.preferTCP.Load()
	if preferTCP {
		t.params.Logger.Debugw("remote description (unfiltered)", "type", sd.Type, "sdp", sd.SDP)
	}
	sd = t.filterCandidates(sd, preferTCP, false)
	if preferTCP {
		t.params.Logger.Debugw("remote description (filtered)", "type", sd.Type, "sdp", sd.SDP)
	}

	if err := t.pc.SetRemoteDescription(sd); err != nil {
		if errors.Is(err, webrtc.ErrConnectionClosed) {
			t.params.Logger.Warnw("trying to set remote description on closed peer connection", nil)
			return nil
		}

		sdpType := "offer"
		if sd.Type == webrtc.SDPTypeAnswer {
			sdpType = "answer"
		}
		prometheus.RecordServiceOperationError(sdpType, "remote_description")
		return errors.Wrap(err, "setting remote description failed")
	} else if sd.Type == webrtc.SDPTypeAnswer {
		t.lock.Lock()
		if !t.canReuseTransceiver {
			t.canReuseTransceiver = true
			t.previousTrackDescription = make(map[string]*trackDescription)
		}
		t.lock.Unlock()
	}

	for _, c := range t.pendingRemoteCandidates {
		if err := t.pc.AddICECandidate(*c); err != nil {
			t.params.Logger.Warnw("failed to add cached ICE candidate", err, "candidate", c)
			return errors.Wrap(err, "add ice candidate failed")
		} else {
			t.params.Logger.Debugw("added cached ICE candidate", "candidate", c)
		}
	}
	t.pendingRemoteCandidates = nil

	return nil
}

func (t *PCTransport) createAndSendAnswer() error {
	numOutstandingAudios, numOutstandingVideos := t.getNumUnmatchedTransceivers()
	t.lock.Lock()
	t.numOutstandingAudios, t.numOutstandingVideos = numOutstandingAudios, numOutstandingVideos
	t.numRequestSentAudios, t.numRequestSentVideos = 0, 0
	t.lock.Unlock()

	answer, err := t.pc.CreateAnswer(nil)
	if err != nil {
		if errors.Is(err, webrtc.ErrConnectionClosed) {
			t.params.Logger.Warnw("trying to create answer on closed peer connection", nil)
			return nil
		}

		prometheus.RecordServiceOperationError("answer", "create")
		return errors.Wrap(err, "create answer failed")
	}

	preferTCP := t.preferTCP.Load()
	if preferTCP {
		t.params.Logger.Debugw("local answer (unfiltered)", "sdp", answer.SDP)
	}

	if err = t.pc.SetLocalDescription(answer); err != nil {
		prometheus.RecordServiceOperationError("answer", "local_description")
		return errors.Wrap(err, "setting local description failed")
	}

	//
	// Filter after setting local description as pion expects the answer
	// to match between CreateAnswer and SetLocalDescription.
	// Filtered answer is sent to remote so that remote does not
	// see filtered candidates.
	//
	answer = t.filterCandidates(answer, preferTCP, true)
	if preferTCP {
		t.params.Logger.Debugw("local answer (filtered)", "sdp", answer.SDP)
	}

	localAnswerId := t.localAnswerId.Load()
	if localAnswerId != 0 && localAnswerId >= t.remoteOfferId.Load() {
		t.params.Logger.Warnw(
			"sdp state: duplicate answer", nil,
			"localAnswerId", localAnswerId,
			"remoteOfferId", t.remoteOfferId.Load(),
		)
	}

	answerId := t.remoteOfferId.Load()

	if err := t.params.Handler.OnAnswer(answer, answerId, t.getMidToTrackIDMapping()); err != nil {
		prometheus.RecordServiceOperationError("answer", "write_message")
		return errors.Wrap(err, "could not send answer")
	}
	t.localAnswerId.Store(answerId)
	prometheus.RecordServiceOperationSuccess("asnwer")

	if err := t.sendUnmatchedMediaRequirement(false); err != nil {
		return err
	}

	t.lock.Lock()
	if !t.canReuseTransceiver {
		t.canReuseTransceiver = true
		t.previousTrackDescription = make(map[string]*trackDescription)
	}
	t.lock.Unlock()

	return t.localDescriptionSent()
}

func (t *PCTransport) handleRemoteOfferReceived(sd *webrtc.SessionDescription, offerId uint32) error {
	t.params.Logger.Debugw("processing offer", "offerId", offerId)
	remoteOfferId := t.remoteOfferId.Load()
	if remoteOfferId != 0 && remoteOfferId != t.localAnswerId.Load() {
		t.params.Logger.Warnw(
			"sdp state: multiple offers without answer", nil,
			"remoteOfferId", remoteOfferId,
			"localAnswerId", t.localAnswerId.Load(),
			"receivedRemoteOfferId", offerId,
		)
	}
	t.remoteOfferId.Store(offerId)

	parsed, err := sd.Unmarshal()
	if err != nil {
		return err
	}

	t.lock.Lock()
	if !t.firstOfferReceived {
		t.firstOfferReceived = true
		var dataChannelFound bool
		for _, media := range parsed.MediaDescriptions {
			if strings.EqualFold(media.MediaName.Media, "application") {
				dataChannelFound = true
				break
			}
		}
		t.firstOfferNoDataChannel = !dataChannelFound
	}
	t.lock.Unlock()

	iceCredential, offerRestartICE, err := t.isRemoteOfferRestartICE(parsed)
	if err != nil {
		return errors.Wrap(err, "check remote offer restart ice failed")
	}

	if offerRestartICE && t.pendingRestartIceOffer == nil {
		t.clearLocalDescriptionSent()
	}

	if offerRestartICE && t.pc.ICEGatheringState() == webrtc.ICEGatheringStateGathering {
		t.params.Logger.Debugw("remote offer restart ice while ice gathering")
		t.pendingRestartIceOffer = sd
		return nil
	}

	if offerRestartICE && t.resetShortConnOnICERestart.CompareAndSwap(true, false) {
		t.resetShortConn()
	}

	if offerRestartICE {
		t.outputAndClearICEStats()
	}

	if err := t.setRemoteDescription(*sd); err != nil {
		return err
	}
	t.params.Handler.OnSetRemoteDescriptionOffer()
	t.processSendersPendingConfig()

	rtxRepairs := nonSimulcastRTXRepairsFromSDP(parsed, t.params.Logger)
	if len(rtxRepairs) > 0 {
		t.params.Logger.Debugw("rtx pairs found from sdp", "ssrcs", rtxRepairs)
		for repair, base := range rtxRepairs {
			t.params.Config.BufferFactory.SetRTXPair(repair, base, "")
		}
	}

	if t.currentOfferIceCredential == "" || offerRestartICE {
		t.currentOfferIceCredential = iceCredential
	}

	return t.createAndSendAnswer()
}

func (t *PCTransport) handleRemoteAnswerReceived(sd *webrtc.SessionDescription, answerId uint32) error {
	t.params.Logger.Debugw("processing answer", "answerId", answerId)
	if answerId != 0 && answerId != t.localOfferId.Load() {
		t.params.Logger.Warnw(
			"sdp state: answer id mismatch", nil,
			"expected", t.localOfferId.Load(),
			"got", answerId,
		)
	}
	t.remoteAnswerId.Store(answerId)

	t.clearSignalStateCheckTimer()

	if err := t.setRemoteDescription(*sd); err != nil {
		// Pion will call RTPSender.Send method for each new added Downtrack, and return error if the DownTrack.Bind
		// returns error. In case of Downtrack.Bind returns ErrUnsupportedCodec, the signal state will be stable as negotiation is aleady compelted
		// before startRTPSenders, and the peerconnection state can be recovered by next negotiation which will be triggered
		// by the SubscriptionManager unsubscribe the failure DownTrack. So don't treat this error as negotiation failure.
		if !errors.Is(err, webrtc.ErrUnsupportedCodec) {
			return err
		}
	}

	if t.negotiationState == transport.NegotiationStateRetry {
		t.setNegotiationState(transport.NegotiationStateNone)

		t.params.Logger.Debugw("re-negotiate after receiving answer")
		return t.createAndSendOffer(nil)
	}

	t.setNegotiationState(transport.NegotiationStateNone)
	return nil
}

func (t *PCTransport) onNegotiationFailed(warning bool, reason string) {
	logFields := []any{
		"reason", reason,
		"localCurrent", t.pc.CurrentLocalDescription(),
		"localPending", t.pc.PendingLocalDescription(),
		"remoteCurrent", t.pc.CurrentRemoteDescription(),
		"remotePending", t.pc.PendingRemoteDescription(),
	}
	if warning {
		t.params.Logger.Warnw(
			"negotiation failed",
			nil,
			logFields...,
		)
	} else {
		t.params.Logger.Infow("negotiation failed", logFields...)
	}
	t.params.Handler.OnNegotiationFailed()
}

func (t *PCTransport) getMidToTrackIDMapping() map[string]string {
	transceivers := t.pc.GetTransceivers()
	midToTrackID := make(map[string]string, len(transceivers))
	for _, tr := range transceivers {
		if mid := tr.Mid(); mid != "" {
			if sender := tr.Sender(); sender != nil {
				if track := sender.Track(); track != nil {
					midToTrackID[mid] = track.ID()
				}
			}
		}
	}
	return midToTrackID
}

// ----------------------

type configureSenderParams struct {
	transceiver              *webrtc.RTPTransceiver
	enabledCodecs            []*hublive.Codec
	rtcpFeedbackConfig       RTCPFeedbackConfig
	filterOutH264HighProfile bool
	enableAudioStereo        bool
	enableAudioNACK          bool
}

func configureSender(params configureSenderParams) {
	configureSenderCodecs(
		params.transceiver,
		params.enabledCodecs,
		params.rtcpFeedbackConfig,
		params.filterOutH264HighProfile,
	)

	if params.transceiver.Kind() == webrtc.RTPCodecTypeAudio {
		configureSenderAudio(params.transceiver, params.enableAudioStereo, params.enableAudioNACK)
	}
}

// configure subscriber transceiver for audio stereo and nack
// pion doesn't support per transciver codec configuration, so the nack of this session will be disabled
// forever once it is first disabled by a transceiver.
func configureSenderAudio(tr *webrtc.RTPTransceiver, stereo bool, nack bool) {
	sender := tr.Sender()
	if sender == nil {
		return
	}

	// enable stereo
	codecs := sender.GetParameters().Codecs
	configCodecs := make([]webrtc.RTPCodecParameters, 0, len(codecs))
	for _, c := range codecs {
		if mime.IsMimeTypeStringOpus(c.MimeType) {
			c.SDPFmtpLine = strings.ReplaceAll(c.SDPFmtpLine, ";sprop-stereo=1", "")
			if stereo {
				c.SDPFmtpLine += ";sprop-stereo=1"
			}
			if !nack {
				for i, fb := range c.RTCPFeedback {
					if fb.Type == webrtc.TypeRTCPFBNACK {
						c.RTCPFeedback = append(c.RTCPFeedback[:i], c.RTCPFeedback[i+1:]...)
						break
					}
				}
			}
		}
		configCodecs = append(configCodecs, c)
	}

	tr.SetCodecPreferences(configCodecs)
}

// In single peer connection mode, set up enebled codecs for sender.
// The config provides config of direction.
// For publisher peer connection those are publish enabled codecs
// and for subscriber peer connection those are subscribe enabled codecs.
//
// But, in single peer connection mode, if setting up a transceiver where the media is
// flowing in the other direction, the other direction codec config needs to be set.
func configureSenderCodecs(
	tr *webrtc.RTPTransceiver,
	enabledCodecs []*hublive.Codec,
	rtcpFeedbackConfig RTCPFeedbackConfig,
	filterOutH264HighProfile bool,
) {
	if len(enabledCodecs) == 0 {
		return
	}

	sender := tr.Sender()
	if sender == nil {
		return
	}

	filteredCodecs := filterCodecs(
		sender.GetParameters().Codecs,
		enabledCodecs,
		rtcpFeedbackConfig,
		filterOutH264HighProfile,
	)
	tr.SetCodecPreferences(filteredCodecs)
}

func configureReceiverCodecs(
	tr *webrtc.RTPTransceiver,
	preferredMimeType string,
	compliesWithCodecOrderInSDPAnswer bool,
) {
	receiver := tr.Receiver()
	if receiver == nil {
		return
	}

	var preferredCodecs, leftCodecs []webrtc.RTPCodecParameters
	for _, c := range receiver.GetParameters().Codecs {
		if tr.Kind() == webrtc.RTPCodecTypeAudio {
			nackFound := false
			for _, fb := range c.RTCPFeedback {
				if fb.Type == webrtc.TypeRTCPFBNACK {
					nackFound = true
					break
				}
			}

			if !nackFound {
				c.RTCPFeedback = append(c.RTCPFeedback, webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBNACK})
			}
		}

		if mime.GetMimeTypeCodec(preferredMimeType) == mime.GetMimeTypeCodec(c.RTPCodecCapability.MimeType) {
			preferredCodecs = append(preferredCodecs, c)
		} else {
			leftCodecs = append(leftCodecs, c)
		}
	}
	if len(preferredCodecs) == 0 {
		return
	}

	reorderedCodecs := append([]webrtc.RTPCodecParameters{}, preferredCodecs...)
	if tr.Kind() == webrtc.RTPCodecTypeVideo {
		// if the client don't comply with codec order in SDP answer, only keep preferred codecs to force client to use it
		if compliesWithCodecOrderInSDPAnswer {
			reorderedCodecs = append(reorderedCodecs, leftCodecs...)
		}
	} else {
		reorderedCodecs = append(reorderedCodecs, leftCodecs...)
	}
	tr.SetCodecPreferences(reorderedCodecs)
}

func nonSimulcastRTXRepairsFromSDP(s *sdp.SessionDescription, logger logger.Logger) map[uint32]uint32 {
	rtxRepairFlows := map[uint32]uint32{}
	for _, media := range s.MediaDescriptions {
		// extract rtx repair flows from the media section for non-simulcast stream,
		// pion will handle simulcast streams by rid probe, don't need handle it here.
		var ridFound bool
		rtxPairs := make(map[uint32]uint32)
	findRTX:
		for _, attr := range media.Attributes {
			switch attr.Key {
			case "rid":
				ridFound = true
				break findRTX
			case sdp.AttrKeySSRCGroup:
				split := strings.Split(attr.Value, " ")
				if split[0] == sdp.SemanticTokenFlowIdentification {
					// Essentially lines like `a=ssrc-group:FID 2231627014 632943048` are processed by this section
					// as this declares that the second SSRC (632943048) is a rtx repair flow (RFC4588) for the first
					// (2231627014) as specified in RFC5576
					if len(split) == 3 {
						baseSsrc, err := strconv.ParseUint(split[1], 10, 32)
						if err != nil {
							logger.Warnw("Failed to parse SSRC", err, "ssrc", split[1])
							continue
						}
						rtxRepairFlow, err := strconv.ParseUint(split[2], 10, 32)
						if err != nil {
							logger.Warnw("Failed to parse SSRC", err, "ssrc", split[2])
							continue
						}
						rtxPairs[uint32(rtxRepairFlow)] = uint32(baseSsrc)
					}
				}
			}
		}
		if !ridFound {
			maps.Copy(rtxRepairFlows, rtxPairs)
		}
	}

	return rtxRepairFlows
}
