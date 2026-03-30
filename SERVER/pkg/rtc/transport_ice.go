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

// transport_ice.go contains ICE/TURN management methods for PCTransport.
// Split from transport.go for readability — same package, same types.

import (
	"fmt"
	"strings"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	"github.com/maxhubsv/hublive-server/pkg/rtc/transport"
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/telemetry/prometheus"
	lksdp "__GITHUB_HUBLIVE__protocol/sdp"
)

// -------------------------------------------------------------------

type wrappedICECandidatePairLogger struct {
	pair *webrtc.ICECandidatePair
}

func (w wrappedICECandidatePairLogger) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if w.pair == nil {
		return nil
	}

	if w.pair.Local != nil {
		e.AddString("localProtocol", w.pair.Local.Protocol.String())
		e.AddString("localCandidateType", w.pair.Local.Typ.String())
		e.AddString("localAddress", w.pair.Local.Address)
		e.AddUint16("localPort", w.pair.Local.Port)
	}
	if w.pair.Remote != nil {
		e.AddString("remoteProtocol", w.pair.Remote.Protocol.String())
		e.AddString("remoteCandidateType", w.pair.Remote.Typ.String())
		e.AddString("remoteAddress", MaybeTruncateIP(w.pair.Remote.Address))
		e.AddUint16("remotePort", w.pair.Remote.Port)
		if w.pair.Remote.RelatedAddress != "" {
			e.AddString("relatedAddress", MaybeTruncateIP(w.pair.Remote.RelatedAddress))
			e.AddUint16("relatedPort", w.pair.Remote.RelatedPort)
		}
	}
	return nil
}

// -------------------------------------------------------------------

func (t *PCTransport) setICEStartedAt(at time.Time) {
	t.lock.Lock()
	if t.iceStartedAt.IsZero() {
		t.iceStartedAt = at

		// checklist of ice agent will be cleared on ice failed, get stats before that
		t.mayFailedICEStatsTimer = time.AfterFunc(iceFailedTimeoutTotal-time.Second, t.logMayFailedICEStats)

		// set failure timer for tcp ice connection based on signaling RTT
		if t.preferTCP.Load() {
			signalingRTT := t.signalingRTT.Load()
			if signalingRTT < 1000 {
				tcpICETimeout := time.Duration(signalingRTT*8) * time.Millisecond
				if tcpICETimeout < minTcpICEConnectTimeout {
					tcpICETimeout = minTcpICEConnectTimeout
				} else if tcpICETimeout > maxTcpICEConnectTimeout {
					tcpICETimeout = maxTcpICEConnectTimeout
				}
				t.params.Logger.Debugw("set TCP ICE connect timer", "timeout", tcpICETimeout, "signalRTT", signalingRTT)
				t.tcpICETimer = time.AfterFunc(tcpICETimeout, func() {
					if t.pc.ICEConnectionState() == webrtc.ICEConnectionStateChecking {
						t.params.Logger.Infow("TCP ICE connect timeout", "timeout", tcpICETimeout, "signalRTT", signalingRTT)
						t.logMayFailedICEStats()
						t.handleConnectionFailed(true)
					}
				})
			}
		}
	}
	t.lock.Unlock()
}

func (t *PCTransport) setICEConnectedAt(at time.Time) {
	t.lock.Lock()
	if t.iceConnectedAt.IsZero() {
		//
		// Record initial connection time.
		// This prevents reset of connected at time if ICE goes `Connected` -> `Disconnected` -> `Connected`.
		//
		t.iceConnectedAt = at

		// set failure timer for dtls handshake
		iceDuration := at.Sub(t.iceStartedAt)
		connTimeoutAfterICE := min(max(minConnectTimeoutAfterICE, 3*iceDuration), maxConnectTimeoutAfterICE)
		t.params.Logger.Debugw("setting connection timer after ICE connected", "timeout", connTimeoutAfterICE, "iceDuration", iceDuration)
		t.connectAfterICETimer = time.AfterFunc(connTimeoutAfterICE, func() {
			state := t.pc.ConnectionState()
			// if pc is still checking or connected but not fully established after timeout, then fire connection fail
			if state != webrtc.PeerConnectionStateClosed && state != webrtc.PeerConnectionStateFailed && !t.isFullyEstablished() {
				t.params.Logger.Infow("connect timeout after ICE connected", "timeout", connTimeoutAfterICE, "iceDuration", iceDuration)
				t.handleConnectionFailed(false)
			}
		})

		// clear tcp ice connect timer
		if t.tcpICETimer != nil {
			t.tcpICETimer.Stop()
			t.tcpICETimer = nil
		}
	}

	if t.mayFailedICEStatsTimer != nil {
		t.mayFailedICEStatsTimer.Stop()
		t.mayFailedICEStatsTimer = nil
	}
	t.mayFailedICEStats = nil
	t.lock.Unlock()
}

func (t *PCTransport) logMayFailedICEStats() {
	if t.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		return
	}

	var candidatePairStats []webrtc.ICECandidatePairStats
	pairStats := t.pc.GetStats()
	candidateStats := make(map[string]webrtc.ICECandidateStats)
	for _, stat := range pairStats {
		switch stat := stat.(type) {
		case webrtc.ICECandidatePairStats:
			candidatePairStats = append(candidatePairStats, stat)
		case webrtc.ICECandidateStats:
			candidateStats[stat.ID] = stat
		}
	}

	iceStats := make([]iceCandidatePairStats, 0, len(candidatePairStats))
	for _, pairStat := range candidatePairStats {
		iceStat := iceCandidatePairStats{ICECandidatePairStats: pairStat}
		if local, ok := candidateStats[pairStat.LocalCandidateID]; ok {
			iceStat.local = local
		}
		if remote, ok := candidateStats[pairStat.RemoteCandidateID]; ok {
			remote.IP = MaybeTruncateIP(remote.IP)
			iceStat.remote = remote
		}
		iceStats = append(iceStats, iceStat)
	}

	t.lock.Lock()
	t.mayFailedICEStats = iceStats
	t.lock.Unlock()
}

func (t *PCTransport) onICEGatheringStateChange(state webrtc.ICEGatheringState) {
	t.params.Logger.Debugw("ice gathering state change", "state", state.String())
	if state != webrtc.ICEGatheringStateComplete {
		return
	}

	t.postEvent(event{
		signal: signalICEGatheringComplete,
	})
}

func (t *PCTransport) onICECandidateTrickle(c *webrtc.ICECandidate) {
	t.postEvent(event{
		signal: signalLocalICECandidate,
		data:   c,
	})
}

func (t *PCTransport) onICEConnectionStateChange(state webrtc.ICEConnectionState) {
	t.params.Logger.Debugw("ice connection state change", "state", state.String())
	switch state {
	case webrtc.ICEConnectionStateConnected:
		t.setICEConnectedAt(time.Now())

	case webrtc.ICEConnectionStateChecking:
		t.setICEStartedAt(time.Now())
	}
}

func (t *PCTransport) outputAndClearICEStats() {
	t.lock.Lock()
	stats := t.mayFailedICEStats
	t.mayFailedICEStats = nil
	t.lock.Unlock()

	if len(stats) > 0 {
		t.params.Logger.Infow("ICE candidate pair stats", "stats", iceCandidatePairStatsEncoder{stats})
	}
}

// -------------------------------------------------------------------

func (t *PCTransport) SetPreferTCP(preferTCP bool) {
	t.preferTCP.Store(preferTCP)
}

func (t *PCTransport) AddICECandidate(candidate webrtc.ICECandidateInit) {
	t.postEvent(event{
		signal: signalRemoteICECandidate,
		data:   &candidate,
	})
}

func (t *PCTransport) GetICEConnectionInfo() *types.ICEConnectionInfo {
	return t.connectionDetails.GetInfo()
}

func (t *PCTransport) GetICEConnectionType() types.ICEConnectionType {
	return t.connectionDetails.GetConnectionType()
}

func (t *PCTransport) GetICESessionUfrag() (string, error) {
	cld := t.pc.CurrentLocalDescription()
	if cld == nil {
		return "", ErrNoLocalDescription
	}

	parsed, err := cld.Unmarshal()
	if err != nil {
		return "", err
	}

	ufrag, _, err := lksdp.ExtractICECredential(parsed)
	if err != nil {
		return "", err
	}

	return ufrag, nil
}

// Handles SDP Fragment for ICE Trickle in WHIP
func (t *PCTransport) HandleICETrickleSDPFragment(sdpFragment string) error {
	if !t.params.UseOneShotSignallingMode {
		return ErrNotSynchronousLocalCandidatesMode
	}

	parsedFragment := &lksdp.SDPFragment{}
	if err := parsedFragment.Unmarshal(sdpFragment); err != nil {
		t.params.Logger.Warnw("could not parse SDP fragment", err, "sdpFragment", sdpFragment)
		return ErrInvalidSDPFragment
	}

	crd := t.pc.CurrentRemoteDescription()
	if crd == nil {
		t.params.Logger.Warnw("no remote description", nil)
		return ErrNoRemoteDescription
	}

	parsedRemote, err := crd.Unmarshal()
	if err != nil {
		t.params.Logger.Warnw("could not parse remote description", err, "offer", crd)
		return err
	}

	// check if BUNDLE mid matches the "mid" in the SDP fragment
	bundleMid, found := lksdp.GetBundleMid(parsedRemote)
	if !found {
		return ErrNoBundleMid
	}

	if parsedFragment.Mid() != bundleMid {
		t.params.Logger.Warnw("incorrect mid", nil, "sdpFragment", sdpFragment)
		return ErrMidMismatch
	}

	fragmentICEUfrag, fragmentICEPwd, err := parsedFragment.ExtractICECredential()
	if err != nil {
		t.params.Logger.Warnw(
			"could not get ICE crendential from fragment", err,
			"sdpFragment", sdpFragment,
		)
		return ErrInvalidSDPFragment
	}
	remoteICEUfrag, remoteICEPwd, err := lksdp.ExtractICECredential(parsedRemote)
	if err != nil {
		t.params.Logger.Warnw("could not get ICE crendential from remote description", err, "sdpFragment", sdpFragment, "remoteDescription", crd)
		return err
	}
	if fragmentICEUfrag != "" && fragmentICEUfrag != remoteICEUfrag {
		t.params.Logger.Warnw(
			"ice ufrag mismatch", nil,
			"remoteICEUfrag", remoteICEUfrag,
			"fragmentICEUfrag", fragmentICEUfrag,
			"sdpFragment", sdpFragment,
			"remoteDescription", crd,
		)
		return ErrICECredentialMismatch
	}
	if fragmentICEPwd != "" && fragmentICEPwd != remoteICEPwd {
		t.params.Logger.Warnw(
			"ice pwd mismatch", nil,
			"remoteICEPwd", remoteICEPwd,
			"fragmentICEPwd", fragmentICEPwd,
			"sdpFragment", sdpFragment,
			"remoteDescription", crd,
		)
		return ErrICECredentialMismatch
	}

	// add candidates from media description
	for _, ic := range parsedFragment.Candidates() {
		c, err := ice.UnmarshalCandidate(ic)
		if err == nil {
			t.connectionDetails.AddRemoteICECandidate(c, false, false, false)
		}

		candidate := webrtc.ICECandidateInit{
			Candidate: ic,
		}
		if err := t.pc.AddICECandidate(candidate); err != nil {
			t.params.Logger.Warnw("failed to add ICE candidate", err, "candidate", candidate)
		} else {
			t.params.Logger.Debugw("added ICE candidate", "candidate", candidate)
		}
	}
	return nil
}

// Handles SDP Fragment for ICE Restart in WHIP
func (t *PCTransport) HandleICERestartSDPFragment(sdpFragment string) (string, error) {
	if !t.params.UseOneShotSignallingMode {
		return "", ErrNotSynchronousLocalCandidatesMode
	}

	parsedFragment := &lksdp.SDPFragment{}
	if err := parsedFragment.Unmarshal(sdpFragment); err != nil {
		t.params.Logger.Warnw("could not parse SDP fragment", err, "sdpFragment", sdpFragment)
		return "", ErrInvalidSDPFragment
	}

	crd := t.pc.CurrentRemoteDescription()
	if crd == nil {
		t.params.Logger.Warnw("no remote description", nil)
		return "", ErrNoRemoteDescription
	}

	parsedRemote, err := crd.Unmarshal()
	if err != nil {
		t.params.Logger.Warnw("could not parse remote description", err, "offer", crd)
		return "", err
	}

	if err := parsedFragment.PatchICECredentialAndCandidatesIntoSDP(parsedRemote); err != nil {
		t.params.Logger.Warnw("could not patch SDP fragment into remote description", err, "offer", crd, "sdpFragment", sdpFragment)
		return "", err
	}

	bytes, err := parsedRemote.Marshal()
	if err != nil {
		t.params.Logger.Warnw("could not marshal SDP with patched remote", err)
		return "", err
	}
	sd := webrtc.SessionDescription{
		SDP:  string(bytes),
		Type: webrtc.SDPTypeOffer,
	}
	if err := t.pc.SetRemoteDescription(sd); err != nil {
		t.params.Logger.Warnw("could not set remote description", err)
		return "", err
	}

	// clear out connection details on ICE restart and re-populate
	t.connectionDetails.Clear()
	for _, candidate := range parsedFragment.Candidates() {
		c, err := ice.UnmarshalCandidate(candidate)
		if err != nil {
			continue
		}
		t.connectionDetails.AddRemoteICECandidate(c, false, false, false)
	}

	ans, err := t.pc.CreateAnswer(nil)
	if err != nil {
		t.params.Logger.Warnw("could not create answer", err)
		return "", err
	}

	if err = t.pc.SetLocalDescription(ans); err != nil {
		t.params.Logger.Warnw("could not set local description", err)
		return "", err
	}

	// wait for gathering to complete to include all candidates in the answer
	<-webrtc.GatheringCompletePromise(t.pc)

	cld := t.pc.CurrentLocalDescription()

	// add local candidates to ICE connection details
	parsedAnswer, err := cld.Unmarshal()
	if err != nil {
		t.params.Logger.Warnw("could not parse local description", err)
		return "", err
	}

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

	addLocalICECandidates(parsedAnswer.Attributes)
	for _, m := range parsedAnswer.MediaDescriptions {
		addLocalICECandidates(m.Attributes)
	}

	parsedFragmentAnswer, err := lksdp.ExtractSDPFragment(parsedAnswer)
	if err != nil {
		t.params.Logger.Warnw("could not extract SDP fragment", err)
		return "", err
	}

	answerFragment, err := parsedFragmentAnswer.Marshal()
	if err != nil {
		t.params.Logger.Warnw("could not marshal answer SDP fragment", err)
		return "", err
	}

	return answerFragment, nil
}

func (t *PCTransport) handleICEGatheringComplete(_ event) error {
	if t.params.IsOfferer {
		return t.handleICEGatheringCompleteOfferer()
	} else {
		return t.handleICEGatheringCompleteAnswerer()
	}
}

func (t *PCTransport) handleLocalICECandidate(e event) error {
	c := e.data.(*webrtc.ICECandidate)

	filtered := false
	if c != nil {
		if t.preferTCP.Load() && c.Protocol != webrtc.ICEProtocolTCP {
			t.params.Logger.Debugw("filtering out local candidate", "candidate", c.String())
			filtered = true
		}
		t.connectionDetails.AddLocalCandidate(c, filtered, true)
	}

	if filtered {
		return nil
	}

	if t.cacheLocalCandidates {
		t.cachedLocalCandidates = append(t.cachedLocalCandidates, c)
		return nil
	}

	if err := t.params.Handler.OnICECandidate(c, t.params.Transport); err != nil {
		t.params.Logger.Warnw("failed to send ICE candidate", err, "candidate", c)
		return err
	}

	return nil
}

func (t *PCTransport) handleRemoteICECandidate(e event) error {
	c := e.data.(*webrtc.ICECandidateInit)

	filtered := false
	if t.preferTCP.Load() && !strings.Contains(strings.ToLower(c.Candidate), "tcp") {
		t.params.Logger.Debugw("filtering out remote candidate", "candidate", c.Candidate)
		filtered = true
	}

	if !t.params.Config.UseMDNS && types.IsCandidateMDNS(*c) {
		t.params.Logger.Debugw("ignoring mDNS candidate", "candidate", c.Candidate)
		filtered = true
	}

	t.connectionDetails.AddRemoteCandidate(*c, filtered, true, false)
	if filtered {
		return nil
	}

	if t.pc.RemoteDescription() == nil {
		t.pendingRemoteCandidates = append(t.pendingRemoteCandidates, c)
		return nil
	}

	if err := t.pc.AddICECandidate(*c); err != nil {
		t.params.Logger.Warnw("failed to add ICE candidate", err, "candidate", c)
		// ignore ParseAddr error as it does not affect ICE connectivity
		if !strings.Contains(err.Error(), "ParseAddr") {
			return errors.Wrap(err, "add ice candidate failed")
		}
	} else {
		t.params.Logger.Debugw("added ICE candidate", "candidate", c)
	}

	return nil
}

func (t *PCTransport) filterCandidates(sd webrtc.SessionDescription, preferTCP, isLocal bool) webrtc.SessionDescription {
	parsed, err := sd.Unmarshal()
	if err != nil {
		t.params.Logger.Warnw("could not unmarshal SDP to filter candidates", err)
		return sd
	}

	filterAttributes := func(attrs []sdp.Attribute) []sdp.Attribute {
		filteredAttrs := make([]sdp.Attribute, 0, len(attrs))
		for _, a := range attrs {
			if a.IsICECandidate() {
				c, err := ice.UnmarshalCandidate(a.Value)
				if err != nil {
					t.params.Logger.Errorw("failed to unmarshal candidate in sdp", err, "isLocal", isLocal, "sdp", sd.SDP)
					filteredAttrs = append(filteredAttrs, a)
					continue
				}
				excluded := preferTCP && !c.NetworkType().IsTCP()
				if !excluded {
					if !t.params.Config.UseMDNS && types.IsICECandidateMDNS(c) {
						excluded = true
					}
				}
				if !excluded {
					filteredAttrs = append(filteredAttrs, a)
				}

				if isLocal {
					t.connectionDetails.AddLocalICECandidate(c, excluded, false)
				} else {
					t.connectionDetails.AddRemoteICECandidate(c, excluded, false, false)
				}
			} else {
				filteredAttrs = append(filteredAttrs, a)
			}
		}

		return filteredAttrs
	}

	parsed.Attributes = filterAttributes(parsed.Attributes)
	for _, m := range parsed.MediaDescriptions {
		m.Attributes = filterAttributes(m.Attributes)
	}

	bytes, err := parsed.Marshal()
	if err != nil {
		t.params.Logger.Warnw("could not marshal SDP to filter candidates", err)
		return sd
	}
	sd.SDP = string(bytes)
	return sd
}

// -------------------------------------------------------------------

func (t *PCTransport) ICERestart() error {
	if t.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		t.params.Logger.Warnw("trying to restart ICE on closed peer connection", nil)
		return ErrIceRestartOnClosedPeerConnection
	}

	t.postEvent(event{
		signal: signalICERestart,
	})
	return nil
}

func (t *PCTransport) ResetShortConnOnICERestart() {
	t.resetShortConnOnICERestart.Store(true)
}

func (t *PCTransport) doICERestart() error {
	if t.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		t.params.Logger.Warnw("trying to restart ICE on closed peer connection", nil)
		return nil
	}

	// if restart is requested, but negotiation never started
	iceGatheringState := t.pc.ICEGatheringState()
	if iceGatheringState == webrtc.ICEGatheringStateNew {
		t.params.Logger.Debugw("skipping ICE restart on not yet started peer connection")
		return nil
	}

	// if restart is requested, and we are not ready, then continue afterwards
	if iceGatheringState == webrtc.ICEGatheringStateGathering {
		t.params.Logger.Debugw("deferring ICE restart to after gathering")
		t.restartAfterGathering = true
		return nil
	}

	if t.resetShortConnOnICERestart.CompareAndSwap(true, false) {
		t.resetShortConn()
	}

	if t.negotiationState == transport.NegotiationStateNone {
		t.outputAndClearICEStats()
		return t.createAndSendOffer(&webrtc.OfferOptions{ICERestart: true})
	}

	currentRemoteDescription := t.pc.CurrentRemoteDescription()
	if currentRemoteDescription == nil {
		// restart without current remote description, send current local description again to try recover
		offer := t.pc.LocalDescription()
		if offer == nil {
			// it should not happen, log just in case
			t.params.Logger.Warnw("ice restart without local offer", nil)
			return ErrIceRestartWithoutLocalSDP
		} else {
			t.params.Logger.Infow("deferring ice restart to next offer")
			t.setNegotiationState(transport.NegotiationStateRetry)
			t.restartAtNextOffer = true

			remoteAnswerId := t.remoteAnswerId.Load()
			if remoteAnswerId != 0 && remoteAnswerId != t.localOfferId.Load() {
				t.params.Logger.Warnw(
					"sdp state: answer not received in ICE restart", nil,
					"localOfferId", t.localOfferId.Load(),
					"remoteAnswerId", remoteAnswerId,
				)
			}

			err := t.params.Handler.OnOffer(*offer, t.localOfferId.Inc(), t.getMidToTrackIDMapping())
			if err != nil {
				prometheus.RecordServiceOperationError("offer", "write_message")
			} else {
				prometheus.RecordServiceOperationSuccess("offer")
			}
			return err
		}
	} else {
		// recover by re-applying the last answer
		t.params.Logger.Infow("recovering from client negotiation state on ICE restart")
		if err := t.pc.SetRemoteDescription(*currentRemoteDescription); err != nil {
			prometheus.RecordServiceOperationError("offer", "remote_description")
			return errors.Wrap(err, "set remote description failed")
		} else {
			t.setNegotiationState(transport.NegotiationStateNone)
			t.outputAndClearICEStats()
			return t.createAndSendOffer(&webrtc.OfferOptions{ICERestart: true})
		}
	}
}

func (t *PCTransport) handleICERestart(_ event) error {
	return t.doICERestart()
}

// ----------------------

type iceCandidatePairStatsEncoder struct {
	stats []iceCandidatePairStats
}

func (e iceCandidatePairStatsEncoder) MarshalLogArray(arr zapcore.ArrayEncoder) error {
	for _, s := range e.stats {
		if err := arr.AppendObject(s); err != nil {
			return err
		}
	}
	return nil
}

type iceCandidatePairStats struct {
	webrtc.ICECandidatePairStats
	local, remote webrtc.ICECandidateStats
}

func (r iceCandidatePairStats) MarshalLogObject(e zapcore.ObjectEncoder) error {
	candidateToString := func(c webrtc.ICECandidateStats) string {
		return fmt.Sprintf("%s:%d %s type(%s/%s), priority(%d)", c.IP, c.Port, c.Protocol, c.CandidateType, c.RelayProtocol, c.Priority)
	}
	e.AddString("state", string(r.State))
	e.AddBool("nominated", r.Nominated)
	e.AddString("local", candidateToString(r.local))
	e.AddString("remote", candidateToString(r.remote))
	e.AddUint64("requestsSent", r.RequestsSent)
	e.AddUint64("responsesReceived", r.ResponsesReceived)
	e.AddUint64("requestsReceived", r.RequestsReceived)
	e.AddUint64("responsesSent", r.ResponsesSent)
	e.AddTime("firstRequestSentAt", r.FirstRequestTimestamp.Time())
	e.AddTime("lastRequestSentAt", r.LastRequestTimestamp.Time())
	e.AddTime("firstResponseReceivedAt", r.FirstResponseTimestamp.Time())
	e.AddTime("lastResponseReceivedAt", r.LastResponseTimestamp.Time())
	e.AddTime("firstRequestReceivedAt", r.FirstRequestReceivedTimestamp.Time())
	e.AddTime("lastRequestReceivedAt", r.LastRequestReceivedTimestamp.Time())

	return nil
}
