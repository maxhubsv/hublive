package rtc

// participant_subscriber.go contains subscribing-related methods for ParticipantImpl.
// Split from participant.go for readability — same package, same types.

import (
	"os"
	"strings"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"

	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"

	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/sfu"
	"github.com/maxhubsv/hublive-server/pkg/sfu/buffer"
	"github.com/maxhubsv/hublive-server/pkg/sfu/streamallocator"
)

func (p *ParticipantImpl) IsPublisher() bool {
	return p.isPublisher.Load()
}

func (p *ParticipantImpl) Verify() bool {
	state := p.State()
	isActive := state != hublive.ParticipantInfo_JOINING && state != hublive.ParticipantInfo_JOINED
	if p.params.UseOneShotSignallingMode {
		isActive = isActive && p.TransportManager.HasPublisherEverConnected()
	}

	return isActive
}

func (p *ParticipantImpl) VerifySubscribeParticipantInfo(pID hublive.ParticipantID, version uint32) {
	if !p.IsReady() {
		// we have not sent a JoinResponse yet. metadata would be covered in JoinResponse
		return
	}
	if info, ok := p.updateCache.Get(pID); ok && info.version >= version {
		return
	}

	if info := p.helper().GetParticipantInfo(pID); info != nil {
		_ = p.SendParticipantUpdate([]*hublive.ParticipantInfo{info})
	}
}

// onTrackSubscribed handles post-processing after a track is subscribed
func (p *ParticipantImpl) onTrackSubscribed(subTrack types.SubscribedTrack) {
	if p.params.ClientInfo.FireTrackByRTPPacket() {
		subTrack.DownTrack().SetActivePaddingOnMuteUpTrack()
	}

	subTrack.AddOnBind(func(err error) {
		if err != nil {
			return
		}
		if p.params.UseOneShotSignallingMode {
			if p.TransportManager.HasPublisherEverConnected() {
				dt := subTrack.DownTrack()
				dt.SeedState(sfu.DownTrackState{ForwarderState: p.getAndDeleteForwarderState(subTrack.ID())})
				dt.SetConnected()
			}
			// ONE-SHOT-SIGNALLING-MODE-TODO: video support should add to publisher PC for congestion control
		} else {
			if p.TransportManager.HasSubscriberEverConnected() {
				dt := subTrack.DownTrack()
				dt.SeedState(sfu.DownTrackState{ForwarderState: p.getAndDeleteForwarderState(subTrack.ID())})
				dt.SetConnected()
			}
			p.TransportManager.AddSubscribedTrack(subTrack)
		}
	})
}

// onTrackUnsubscribed handles post-processing after a track is unsubscribed
func (p *ParticipantImpl) onTrackUnsubscribed(subTrack types.SubscribedTrack) {
	p.TransportManager.RemoveSubscribedTrack(subTrack)
}

func (p *ParticipantImpl) onSubscriberOffer(offer webrtc.SessionDescription, offerId uint32, midToTrackID map[string]string) error {
	p.subLogger.Debugw(
		"sending offer",
		"transport", hublive.SignalTarget_SUBSCRIBER,
		"offer", offer,
		"offerId", offerId,
		"midToTrackID", midToTrackID,
	)
	return p.sendSdpOffer(offer, offerId, midToTrackID)
}

func (p *ParticipantImpl) onSubscriberInitialConnected() {
	go p.subscriberRTCPWorker()

	p.setDownTracksConnected()
}

func (p *ParticipantImpl) subscriberRTCPWorker() {
	defer func() {
		if r := Recover(p.GetLogger()); r != nil {
			os.Exit(1)
		}
	}()
	for {
		if p.IsDisconnected() {
			return
		}

		subscribedTracks := p.SubscriptionManager.GetSubscribedTracks()

		// send in batches of sdBatchSize
		batchSize := 0
		var pkts []rtcp.Packet
		var sd []rtcp.SourceDescriptionChunk
		for _, subTrack := range subscribedTracks {
			sr := subTrack.DownTrack().CreateSenderReport()
			chunks := subTrack.DownTrack().CreateSourceDescriptionChunks()
			if sr == nil || chunks == nil {
				continue
			}

			pkts = append(pkts, sr)
			sd = append(sd, chunks...)
			numItems := 0
			for _, chunk := range chunks {
				numItems += len(chunk.Items)
			}
			batchSize = batchSize + 1 + numItems
			if batchSize >= sdBatchSize {
				if len(sd) != 0 {
					pkts = append(pkts, &rtcp.SourceDescription{Chunks: sd})
				}
				if err := p.TransportManager.WriteSubscriberRTCP(pkts); err != nil {
					if IsEOF(err) {
						return
					}
					p.subLogger.Errorw("could not send down track reports", err)
				}

				pkts = pkts[:0]
				sd = sd[:0]
				batchSize = 0
			}
		}

		if len(pkts) != 0 || len(sd) != 0 {
			if len(sd) != 0 {
				pkts = append(pkts, &rtcp.SourceDescription{Chunks: sd})
			}
			if err := p.TransportManager.WriteSubscriberRTCP(pkts); err != nil {
				if IsEOF(err) {
					return
				}
				p.subLogger.Errorw("could not send down track reports", err)
			}
		}

		time.Sleep(3 * time.Second)
	}
}

func (p *ParticipantImpl) onStreamStateChange(update *streamallocator.StreamStateUpdate) error {
	if len(update.StreamStates) == 0 {
		return nil
	}

	streamStateUpdate := &hublive.StreamStateUpdate{}
	for _, streamStateInfo := range update.StreamStates {
		state := hublive.StreamState_ACTIVE
		if streamStateInfo.State == streamallocator.StreamStatePaused {
			state = hublive.StreamState_PAUSED
		}
		streamStateUpdate.StreamStates = append(streamStateUpdate.StreamStates, &hublive.StreamStateInfo{
			ParticipantSid: string(streamStateInfo.ParticipantID),
			TrackSid:       string(streamStateInfo.TrackID),
			State:          state,
		})
	}

	return p.sendStreamStateUpdate(streamStateUpdate)
}

func (p *ParticipantImpl) onSubscribedMaxQualityChange(
	trackID hublive.TrackID,
	trackInfo *hublive.TrackInfo,
	subscribedQualities []*hublive.SubscribedCodec,
	maxSubscribedQualities []types.SubscribedCodecQuality,
) error {
	if p.params.DisableDynacast {
		return nil
	}

	if len(subscribedQualities) == 0 {
		return nil
	}

	// send layer info about max subscription changes to telemetry
	for _, maxSubscribedQuality := range maxSubscribedQualities {
		ti := &hublive.TrackInfo{
			Sid:  trackInfo.Sid,
			Type: trackInfo.Type,
		}
		for _, layer := range buffer.GetVideoLayersForMimeType(maxSubscribedQuality.CodecMime, trackInfo) {
			if layer.Quality == maxSubscribedQuality.Quality {
				ti.Width = layer.Width
				ti.Height = layer.Height
				break
			}
		}
		p.params.TelemetryListener.OnTrackMaxSubscribedVideoQuality(
			p.ID(),
			ti,
			maxSubscribedQuality.CodecMime,
			maxSubscribedQuality.Quality,
		)
	}

	// normalize the codec name
	for _, subscribedQuality := range subscribedQualities {
		subscribedQuality.Codec = strings.ToLower(strings.TrimPrefix(subscribedQuality.Codec, mime.MimeTypePrefixVideo))
	}

	subscribedQualityUpdate := &hublive.SubscribedQualityUpdate{
		TrackSid:            string(trackID),
		SubscribedQualities: subscribedQualities[0].Qualities, // for compatible with old client
		SubscribedCodecs:    subscribedQualities,
	}

	p.pubLogger.Debugw(
		"sending max subscribed quality",
		"trackID", trackID,
		"qualities", subscribedQualities,
		"max", maxSubscribedQualities,
	)
	return p.sendSubscribedQualityUpdate(subscribedQualityUpdate)
}

func (p *ParticipantImpl) onSubscribedAudioCodecChange(
	trackID hublive.TrackID,
	codecs []*hublive.SubscribedAudioCodec,
) error {
	if p.params.DisableDynacast {
		return nil
	}

	if len(codecs) == 0 {
		return nil
	}

	// normalize the codec name
	for _, codec := range codecs {
		codec.Codec = strings.ToLower(strings.TrimPrefix(codec.Codec, mime.MimeTypePrefixAudio))
	}

	subscribedAudioCodecUpdate := &hublive.SubscribedAudioCodecUpdate{
		TrackSid:              string(trackID),
		SubscribedAudioCodecs: codecs,
	}
	p.pubLogger.Debugw(
		"sending subscribed audio codec update",
		"trackID", trackID,
		"update", logger.Proto(subscribedAudioCodecUpdate),
	)
	return p.sendSubscribedAudioCodecUpdate(subscribedAudioCodecUpdate)
}

func (p *ParticipantImpl) onSubscriptionError(trackID hublive.TrackID, fatal bool, err error) {
	signalErr := hublive.SubscriptionError_SE_UNKNOWN
	switch {
	case errors.Is(err, webrtc.ErrUnsupportedCodec):
		signalErr = hublive.SubscriptionError_SE_CODEC_UNSUPPORTED
	case errors.Is(err, ErrTrackNotFound):
		signalErr = hublive.SubscriptionError_SE_TRACK_NOTFOUND
	}

	p.sendSubscriptionResponse(trackID, signalErr)

	if p.params.ReconnectOnSubscriptionError && fatal {
		p.subLogger.Infow("issuing full reconnect on subscription error", "trackID", trackID)
		p.IssueFullReconnect(types.ParticipantCloseReasonSubscriptionError)
	}
}

func (p *ParticipantImpl) HandleUpdateSubscriptions(
	trackIDs []hublive.TrackID,
	participantTracks []*hublive.ParticipantTracks,
	subscribe bool,
) {
	p.listener().OnUpdateSubscriptions(p, trackIDs, participantTracks, subscribe)
}

func (p *ParticipantImpl) HandleUpdateSubscriptionPermission(subscriptionPermission *hublive.SubscriptionPermission) error {
	return p.listener().OnUpdateSubscriptionPermission(p, subscriptionPermission)
}

func (p *ParticipantImpl) HandleSyncState(syncState *hublive.SyncState) error {
	return p.listener().OnSyncState(p, syncState)
}

func (p *ParticipantImpl) AddTrackLocal(
	trackLocal webrtc.TrackLocal,
	params types.AddTrackParams,
) (*webrtc.RTPSender, *webrtc.RTPTransceiver, error) {
	if p.params.UseSinglePeerConnection {
		return p.TransportManager.AddTrackLocal(
			trackLocal,
			params,
			p.enabledSubscribeCodecs,
			p.params.Config.Subscriber.RTCPFeedback,
		)
	} else {
		return p.TransportManager.AddTrackLocal(trackLocal, params, nil, RTCPFeedbackConfig{})
	}
}

func (p *ParticipantImpl) AddTransceiverFromTrackLocal(
	trackLocal webrtc.TrackLocal,
	params types.AddTrackParams,
) (*webrtc.RTPSender, *webrtc.RTPTransceiver, error) {
	if p.params.UseSinglePeerConnection {
		return p.TransportManager.AddTransceiverFromTrackLocal(
			trackLocal,
			params,
			p.enabledSubscribeCodecs,
			p.params.Config.Subscriber.RTCPFeedback,
		)
	} else {
		return p.TransportManager.AddTransceiverFromTrackLocal(
			trackLocal,
			params,
			nil,
			RTCPFeedbackConfig{},
		)
	}
}
