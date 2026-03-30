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
	"io"
	"slices"
	"strings"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"

	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	protosdp "__GITHUB_HUBLIVE__protocol/sdp"
	protosignalling "__GITHUB_HUBLIVE__protocol/signalling"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/guid"

	"github.com/maxhubsv/hublive-server/pkg/sfu"
	"github.com/maxhubsv/hublive-server/pkg/sfu/buffer"
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/telemetry/prometheus"
	sutils "github.com/maxhubsv/hublive-server/pkg/utils"
)

func (p *ParticipantImpl) synthesizeAddTrackRequests(parsedOffer *sdp.SessionDescription) error {
	for _, m := range parsedOffer.MediaDescriptions {
		if !strings.EqualFold(m.MediaName.Media, "audio") && !strings.EqualFold(m.MediaName.Media, "video") {
			continue
		}

		cid := protosdp.GetMediaStreamTrack(m)
		if cid == "" {
			cid = guid.New(utils.TrackPrefix)
		}

		rids, ridsOk := protosdp.GetSimulcastRids(m)

		var (
			name        string
			trackSource hublive.TrackSource
			trackType   hublive.TrackType
		)
		if strings.EqualFold(m.MediaName.Media, "audio") {
			name = "synthesized-microphone"
			trackSource = hublive.TrackSource_MICROPHONE
			trackType = hublive.TrackType_AUDIO
		} else {
			name = "synthesized-camera"
			trackSource = hublive.TrackSource_CAMERA
			trackType = hublive.TrackType_VIDEO
		}
		req := &hublive.AddTrackRequest{
			Cid:        cid,
			Name:       name,
			Source:     trackSource,
			Type:       trackType,
			DisableDtx: true,
			Stereo:     false,
			Stream:     "camera",
		}
		if strings.EqualFold(m.MediaName.Media, "video") {
			if ridsOk {
				// add simulcast layers, NOTE: only quality can be set as dimensions/fps is not available
				n := min(len(rids), int(buffer.DefaultMaxLayerSpatial)+1)
				for i := range n {
					// WARN: casting int -> protobuf enum
					req.Layers = append(req.Layers, &hublive.VideoLayer{Quality: hublive.VideoQuality(i)})
				}
			} else {
				// dummy layer to ensure at least one layer is available
				req.Layers = []*hublive.VideoLayer{{}}
			}
		}
		p.AddTrack(req)
	}
	return nil
}

func (p *ParticipantImpl) updateRidsFromSDP(parsed *sdp.SessionDescription, unmatchVideos []*sdp.MediaDescription) {
	for _, m := range parsed.MediaDescriptions {
		if m.MediaName.Media != "video" || !slices.Contains(unmatchVideos, m) {
			continue
		}

		mst := protosdp.GetMediaStreamTrack(m)
		if mst == "" {
			continue
		}

		getRids := func(inRids buffer.VideoLayersRid) buffer.VideoLayersRid {
			var outRids buffer.VideoLayersRid
			rids, ok := protosdp.GetSimulcastRids(m)
			if ok {
				n := min(len(rids), len(inRids))
				for i := range n {
					// disabled layers will have a `~` prefix, remove it while determining actual rid
					if len(rids[i]) != 0 && rids[i][0] == '~' {
						outRids[i] = rids[i][1:]
					} else {
						outRids[i] = rids[i]
					}
				}
				for i := n; i < len(inRids); i++ {
					outRids[i] = ""
				}
				outRids = buffer.NormalizeVideoLayersRid(outRids)
			} else {
				for i := range len(inRids) {
					outRids[i] = ""
				}
			}

			return outRids
		}

		p.pendingTracksLock.Lock()
		pti := p.getPendingTrackPrimaryBySdpCid(mst)
		if pti != nil {
			pti.sdpRids = getRids(pti.sdpRids)
			p.pubLogger.Debugw(
				"pending track rids updated",
				"trackID", pti.trackInfos[0].Sid,
				"pendingTrack", pti,
			)

			ti := pti.trackInfos[0]
			for _, codec := range ti.Codecs {
				if codec.Cid == mst || codec.SdpCid == mst {
					mimeType := mime.NormalizeMimeType(codec.MimeType)
					for _, layer := range codec.Layers {
						layer.SpatialLayer = buffer.VideoQualityToSpatialLayer(mimeType, layer.Quality, ti)
						layer.Rid = buffer.VideoQualityToRid(mimeType, layer.Quality, ti, pti.sdpRids)
					}
				}
			}
		}
		p.pendingTracksLock.Unlock()

		if pti == nil {
			// track could already be published, but this could be back up codec offer,
			// so check in published tracks also
			mt := p.getPublishedTrackBySdpCid(mst)
			if mt != nil {
				mimeType := mt.(*MediaTrack).GetMimeTypeForSdpCid(mst)
				if mimeType != mime.MimeTypeUnknown {
					rids := getRids(buffer.DefaultVideoLayersRid)
					mt.(*MediaTrack).UpdateCodecRids(mimeType, rids)
					p.pubLogger.Debugw(
						"published track rids updated",
						"trackID", mt.ID(),
						"mime", mimeType,
						"track", logger.Proto(mt.ToProto()),
					)
				} else {
					p.pubLogger.Warnw(
						"could not get mime type for sdp cid", nil,
						"trackID", mt.ID(),
						"sdpCid", mst,
						"track", logger.Proto(mt.ToProto()),
					)
				}
			}
		}
	}
}

// HandleOffer an offer from remote participant, used when clients make the initial connection
func (p *ParticipantImpl) HandleOffer(sd *hublive.SessionDescription) error {
	offer, offerId, _ := protosignalling.FromProtoSessionDescription(sd)
	lgr := p.pubLogger.WithUnlikelyValues(
		"transport", hublive.SignalTarget_PUBLISHER,
		"offer", offer,
		"offerId", offerId,
	)

	lgr.Debugw("received offer")

	parsedOffer, err := offer.Unmarshal()
	if err != nil {
		lgr.Warnw("could not parse offer", err)
		return err
	}

	if p.params.UseOneShotSignallingMode {
		if err := p.synthesizeAddTrackRequests(parsedOffer); err != nil {
			lgr.Warnw("could not synthesize add track requests", err)
			return err
		}
	}

	err = p.TransportManager.HandleOffer(offer, offerId, p.MigrateState() == types.MigrateStateInit)
	if err != nil {
		lgr.Warnw("could not handle offer", err, "mungedOffer", offer)
		return err
	}

	if p.params.UseOneShotSignallingMode {
		go p.listener().OnSubscriberReady(p)
	}

	p.handlePendingRemoteTracks()
	return nil
}

func (p *ParticipantImpl) onPublisherSetRemoteDescription() {
	offer := p.TransportManager.LastPublisherOfferPending()
	parsedOffer, err := offer.Unmarshal()
	if err != nil {
		p.pubLogger.Warnw("could not parse offer", err)
		return
	}

	// set publish codec preferences after remote description is set
	// and required transceivers are created
	unmatchAudios, unmatchVideos := p.populateSdpCid(parsedOffer)
	p.setCodecPreferencesForPublisher(parsedOffer, unmatchAudios, unmatchVideos)
	p.updateRidsFromSDP(parsedOffer, unmatchVideos)
}

func (p *ParticipantImpl) onPublisherAnswer(answer webrtc.SessionDescription, answerId uint32, midToTrackID map[string]string) error {
	if p.IsClosed() || p.IsDisconnected() {
		return nil
	}

	answer = p.configurePublisherAnswer(answer)
	p.pubLogger.Debugw(
		"sending answer",
		"transport", hublive.SignalTarget_PUBLISHER,
		"answer", answer,
		"answerId", answerId,
		"midToTrackID", midToTrackID,
	)

	return p.sendSdpAnswer(answer, answerId, midToTrackID)
}

func (p *ParticipantImpl) handleMigrateTracks() []*MediaTrack {
	// muted track won't send rtp packet, so it is required to add mediatrack manually.
	// But, synthesising track publish for unmuted tracks keeps a consistent path.
	// In both cases (muted and unmuted), when publisher sends media packets, OnTrack would register and go from there.
	var addedTracks []*MediaTrack
	p.pendingTracksLock.Lock()
	for cid, pti := range p.pendingTracks {
		if !pti.migrated {
			continue
		}

		if len(pti.trackInfos) > 1 {
			p.pubLogger.Warnw(
				"too many pending migrated tracks", nil,
				"trackID", pti.trackInfos[0].Sid,
				"count", len(pti.trackInfos),
				"cid", cid,
			)
		}

		mt := p.addMigratedTrack(cid, pti.trackInfos[0])
		if mt != nil {
			addedTracks = append(addedTracks, mt)
		} else {
			p.pubLogger.Warnw("could not find migrated track, migration failed", nil, "cid", cid)
			p.pendingTracksLock.Unlock()
			p.IssueFullReconnect(types.ParticipantCloseReasonMigrateCodecMismatch)
			return nil
		}
	}

	if len(addedTracks) != 0 {
		p.dirty.Store(true)
	}
	p.pendingTracksLock.Unlock()

	return addedTracks
}

// AddTrack is called when client intends to publish track.
// records track details and lets client know it's ok to proceed
func (p *ParticipantImpl) AddTrack(req *hublive.AddTrackRequest) {
	p.params.Logger.Debugw("add track request", "trackID", req.Cid, "request", logger.Proto(req))
	if !p.CanPublishSource(req.Source) {
		p.pubLogger.Warnw("no permission to publish track", nil, "trackID", req.Sid, "kind", req.Type)
		p.sendRequestResponse(&hublive.RequestResponse{
			Reason: hublive.RequestResponse_NOT_ALLOWED,
			Request: &hublive.RequestResponse_AddTrack{
				AddTrack: utils.CloneProto(req),
			},
		})
		return
	}

	if req.Type != hublive.TrackType_AUDIO && req.Type != hublive.TrackType_VIDEO {
		p.pubLogger.Warnw("unsupported track type", nil, "trackID", req.Sid, "kind", req.Type)
		p.sendRequestResponse(&hublive.RequestResponse{
			Reason: hublive.RequestResponse_UNSUPPORTED_TYPE,
			Request: &hublive.RequestResponse_AddTrack{
				AddTrack: utils.CloneProto(req),
			},
		})
		return
	}

	p.pendingTracksLock.Lock()
	ti := p.addPendingTrackLocked(req)
	p.pendingTracksLock.Unlock()
	if ti == nil {
		return
	}

	p.sendTrackPublished(req.Cid, ti)

	p.handlePendingRemoteTracks()
}

func (p *ParticipantImpl) setIsPublisher(isPublisher bool) {
	if p.isPublisher.Swap(isPublisher) == isPublisher {
		return
	}

	p.lock.Lock()
	p.requireBroadcast = true
	p.lock.Unlock()

	p.dirty.Store(true)

	// trigger update as well if participant is already fully connected
	if p.State() == hublive.ParticipantInfo_ACTIVE {
		p.listener().OnParticipantUpdate(p)
	}
}

func (p *ParticipantImpl) removePublishedTrack(track types.MediaTrack) {
	p.RemovePublishedTrack(track, false)
	if p.ProtocolVersion().SupportsUnpublish() {
		p.sendTrackUnpublished(track.ID())
	} else {
		// for older clients that don't support unpublish, mute to avoid them sending data
		p.sendTrackMuted(track.ID(), true)
	}
}

// when a new remoteTrack is created, creates a Track and adds it to room
func (p *ParticipantImpl) onMediaTrack(rtcTrack *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
	if p.IsDisconnected() {
		return
	}

	var codec webrtc.RTPCodecParameters
	var fromSdp bool
	if rtcTrack.Kind() == webrtc.RTPCodecTypeVideo && p.params.ClientInfo.FireTrackByRTPPacket() {
		if rtcTrack.Codec().PayloadType == 0 {
			go func() {
				// wait for the first packet to determine the codec
				bytes := make([]byte, 1500)
				_, _, err := rtcTrack.Read(bytes)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						p.params.Logger.Warnw(
							"could not read first packet to determine codec, track will be ignored", err,
							"trackID", rtcTrack.ID(),
							"StreamID", rtcTrack.StreamID(),
						)
					}
					return
				}
				p.onMediaTrack(rtcTrack, rtpReceiver)
			}()
			return
		}
		codec = rtcTrack.Codec()
	} else {
		// track fired by sdp
		codecs := rtpReceiver.GetParameters().Codecs
		if len(codecs) == 0 {
			p.pubLogger.Errorw(
				"no negotiated codecs for track, track will be ignored", nil,
				"trackID", rtcTrack.ID(),
				"StreamID", rtcTrack.StreamID(),
			)
			return
		}
		codec = codecs[0]
		fromSdp = true
	}
	p.params.Logger.Debugw(
		"onMediaTrack",
		"codec", codec,
		"payloadType", codec.PayloadType,
		"fromSdp", fromSdp,
		"parameters", rtpReceiver.GetParameters(),
	)

	var track sfu.TrackRemote = sfu.NewTrackRemoteFromSdp(rtcTrack, codec)
	publishedTrack, isNewTrack, isReceiverAdded, sdpRids := p.mediaTrackReceived(track, rtpReceiver)
	if publishedTrack == nil {
		p.pubLogger.Debugw(
			"webrtc track published but can't find MediaTrack in pendingTracks",
			"kind", track.Kind().String(),
			"webrtcTrackID", track.ID(),
			"rid", track.RID(),
			"ssrc", track.SSRC(),
			"rtxSsrc", track.RtxSSRC(),
			"mime", mime.NormalizeMimeType(codec.MimeType),
			"isReceiverAdded", isReceiverAdded,
			"sdpRids", logger.StringSlice(sdpRids[:]),
		)
		return
	}

	if !p.CanPublishSource(publishedTrack.Source()) {
		p.pubLogger.Warnw("no permission to publish mediaTrack", nil,
			"source", publishedTrack.Source(),
		)
		p.removePublishedTrack(publishedTrack)
		return
	}

	p.TransportManager.RTPStreamPublished(
		uint32(track.SSRC()),
		p.TransportManager.GetPublisherMid(rtpReceiver),
		track.RID(),
	)

	p.setIsPublisher(true)
	p.dirty.Store(true)

	p.pubLogger.Infow(
		"mediaTrack published",
		"kind", track.Kind().String(),
		"trackID", publishedTrack.ID(),
		"webrtcTrackID", track.ID(),
		"rid", track.RID(),
		"ssrc", track.SSRC(),
		"rtxSsrc", track.RtxSSRC(),
		"mime", mime.NormalizeMimeType(codec.MimeType),
		"trackInfo", logger.Proto(publishedTrack.ToProto()),
		"fromSdp", fromSdp,
		"isReceiverAdded", isReceiverAdded,
		"sdpRids", logger.StringSlice(sdpRids[:]),
	)

	if !isNewTrack && !publishedTrack.HasPendingCodec() && p.IsReady() {
		p.listener().OnTrackUpdated(p, publishedTrack)
	}
}

func (p *ParticipantImpl) handlePendingRemoteTracks() {
	p.pendingTracksLock.Lock()
	pendingTracks := p.pendingRemoteTracks
	p.pendingRemoteTracks = nil
	p.pendingTracksLock.Unlock()
	for _, rt := range pendingTracks {
		p.onMediaTrack(rt.track, rt.receiver)
	}
}

func (p *ParticipantImpl) onPublisherInitialConnected() {
	p.SetMigrateState(types.MigrateStateComplete)

	if p.supervisor != nil {
		p.supervisor.SetPublisherPeerConnectionConnected(true)
	}

	if p.params.UseOneShotSignallingMode || p.params.UseSinglePeerConnection {
		go p.subscriberRTCPWorker()

		p.setDownTracksConnected()
	}

	p.pubRTCPQueue.Start()
}

func (p *ParticipantImpl) addPendingTrackLocked(req *hublive.AddTrackRequest) *hublive.TrackInfo {
	if req.Sid != "" {
		track := p.GetPublishedTrack(hublive.TrackID(req.Sid))
		if track == nil {
			p.pubLogger.Infow("could not find existing track for multi-codec simulcast", "trackID", req.Sid)
			p.sendRequestResponse(&hublive.RequestResponse{
				Reason: hublive.RequestResponse_NOT_FOUND,
				Request: &hublive.RequestResponse_AddTrack{
					AddTrack: utils.CloneProto(req),
				},
			})
			return nil
		}

		track.(*MediaTrack).UpdateCodecInfo(req.SimulcastCodecs)
		return track.ToProto()
	}

	backupCodecPolicy := req.BackupCodecPolicy

	// enable simulcast codec for audio by default
	if (backupCodecPolicy != hublive.BackupCodecPolicy_REGRESSION && req.Type == hublive.TrackType_AUDIO) ||
		(backupCodecPolicy != hublive.BackupCodecPolicy_SIMULCAST && p.params.DisableCodecRegression) {
		backupCodecPolicy = hublive.BackupCodecPolicy_SIMULCAST
	}

	cloneLayers := func(layers []*hublive.VideoLayer) []*hublive.VideoLayer {
		if len(layers) == 0 {
			return nil
		}

		clonedLayers := make([]*hublive.VideoLayer, 0, len(layers))
		for _, l := range layers {
			clonedLayers = append(clonedLayers, utils.CloneProto(l))
		}
		slices.SortFunc(clonedLayers, func(i, j *hublive.VideoLayer) int {
			return int(i.Quality) - int(j.Quality)
		})
		return clonedLayers
	}

	ti := &hublive.TrackInfo{
		Type:                  req.Type,
		Name:                  req.Name,
		Width:                 req.Width,
		Height:                req.Height,
		Muted:                 req.Muted,
		DisableDtx:            req.DisableDtx,
		Source:                req.Source,
		Layers:                cloneLayers(req.Layers),
		DisableRed:            req.DisableRed,
		Stereo:                req.Stereo,
		Encryption:            req.Encryption,
		Stream:                req.Stream,
		BackupCodecPolicy:     backupCodecPolicy,
		AudioFeatures:         sutils.DedupeSlice(req.AudioFeatures),
		PacketTrailerFeatures: sutils.DedupeSlice(req.PacketTrailerFeatures),
	}
	if req.Stereo && !slices.Contains(ti.AudioFeatures, hublive.AudioTrackFeature_TF_STEREO) {
		ti.AudioFeatures = append(ti.AudioFeatures, hublive.AudioTrackFeature_TF_STEREO)
	}
	if req.DisableDtx && !slices.Contains(ti.AudioFeatures, hublive.AudioTrackFeature_TF_NO_DTX) {
		ti.AudioFeatures = append(ti.AudioFeatures, hublive.AudioTrackFeature_TF_NO_DTX)
	}
	if ti.Stream == "" {
		ti.Stream = StreamFromTrackSource(ti.Source)
	}
	p.setTrackID(req.Cid, ti)

	if len(req.SimulcastCodecs) == 0 {
		// clients not supporting simulcast codecs, synthesise a codec
		videoLayerMode := hublive.VideoLayer_MODE_UNUSED
		if p.params.ClientInfo.isOBS() {
			videoLayerMode = hublive.VideoLayer_ONE_SPATIAL_LAYER_PER_STREAM_INCOMPLETE_RTCP_SR
		}
		ti.Codecs = append(ti.Codecs, &hublive.SimulcastCodecInfo{
			Cid:            req.Cid,
			Layers:         cloneLayers(req.Layers),
			VideoLayerMode: videoLayerMode,
		})
	} else {
		seenCodecs := make(map[string]struct{})
		for _, codec := range req.SimulcastCodecs {
			if codec.Codec == "" {
				p.pubLogger.Warnw(
					"simulcast codec without mime type", nil,
					"trackID", ti.Sid,
					"track", logger.Proto(ti),
					"addTrackRequest", logger.Proto(req),
				)
			}

			mimeType := codec.Codec
			videoLayerMode := codec.VideoLayerMode
			switch req.Type {
			case hublive.TrackType_VIDEO:
				if !mime.IsMimeTypeStringVideo(mimeType) {
					mimeType = mime.MimeTypePrefixVideo + mimeType
				}
				if !IsCodecEnabled(p.enabledPublishCodecs, webrtc.RTPCodecCapability{MimeType: mimeType}) {
					altCodec := selectAlternativeVideoCodec(p.enabledPublishCodecs)
					p.pubLogger.Infow(
						"falling back to alternative video codec",
						"codec", mimeType,
						"altCodec", altCodec,
						"enabledPublishCodecs", logger.ProtoSlice(p.enabledPublishCodecs),
						"trackID", ti.Sid,
					)
					// select an alternative MIME type that's generally supported
					mimeType = altCodec
				}
				if videoLayerMode == hublive.VideoLayer_MODE_UNUSED {
					if mime.IsMimeTypeStringSVCCapable(mimeType) {
						videoLayerMode = hublive.VideoLayer_MULTIPLE_SPATIAL_LAYERS_PER_STREAM
					} else {
						if p.params.ClientInfo.isOBS() {
							videoLayerMode = hublive.VideoLayer_ONE_SPATIAL_LAYER_PER_STREAM_INCOMPLETE_RTCP_SR
						} else {
							videoLayerMode = hublive.VideoLayer_ONE_SPATIAL_LAYER_PER_STREAM
						}
					}
				}

			case hublive.TrackType_AUDIO:
				if !mime.IsMimeTypeStringAudio(mimeType) {
					mimeType = mime.MimeTypePrefixAudio + mimeType
				}
				if !IsCodecEnabled(p.enabledPublishCodecs, webrtc.RTPCodecCapability{MimeType: mimeType}) {
					altCodec := selectAlternativeAudioCodec(p.enabledPublishCodecs)
					p.pubLogger.Infow(
						"falling back to alternative audio codec",
						"codec", mimeType,
						"altCodec", altCodec,
						"enabledPublishCodecs", logger.ProtoSlice(p.enabledPublishCodecs),
						"trackID", ti.Sid,
					)
					// select an alternative MIME type that's generally supported
					mimeType = altCodec
				}
			}

			if _, ok := seenCodecs[mimeType]; ok || mimeType == "" {
				continue
			}
			seenCodecs[mimeType] = struct{}{}

			ti.Codecs = append(ti.Codecs, &hublive.SimulcastCodecInfo{
				MimeType:       mimeType,
				Cid:            codec.Cid,
				VideoLayerMode: videoLayerMode,
			})
		}

		// set up layers with codec specific layers,
		// fall back to common layers if codec specific layer is not available
		for idx, codec := range ti.Codecs {
			found := false
			for _, simulcastCodec := range req.SimulcastCodecs {
				if mime.GetMimeTypeCodec(codec.MimeType) != mime.NormalizeMimeTypeCodec(simulcastCodec.Codec) {
					continue
				}

				if len(simulcastCodec.Layers) != 0 {
					codec.Layers = cloneLayers(simulcastCodec.Layers)
				} else {
					codec.Layers = cloneLayers(req.Layers)
				}
				found = true
				break
			}

			if !found {
				// could happen if an alternate codec is selected and that is not in the simulcast codecs list
				codec.Layers = cloneLayers(req.Layers)
			}

			// populate simulcast flag for compatibility, true if primary codec is not SVC and has multiple layers
			if idx == 0 && codec.VideoLayerMode != hublive.VideoLayer_MULTIPLE_SPATIAL_LAYERS_PER_STREAM && len(codec.Layers) > 1 {
				ti.Simulcast = true
			}
		}
	}

	p.params.TelemetryListener.OnTrackPublishRequested(p.ID(), p.Identity(), utils.CloneProto(ti))

	if p.supervisor != nil {
		p.supervisor.AddPublication(hublive.TrackID(ti.Sid))
		p.supervisor.SetPublicationMute(hublive.TrackID(ti.Sid), ti.Muted)
	}

	if p.getPublishedTrackBySignalCid(req.Cid) != nil || p.getPublishedTrackBySdpCid(req.Cid) != nil || p.pendingTracks[req.Cid] != nil {
		if p.pendingTracks[req.Cid] == nil {
			pti := &pendingTrackInfo{
				trackInfos: []*hublive.TrackInfo{ti},
				createdAt:  time.Now(),
				queued:     true,
			}
			if ti.Type == hublive.TrackType_VIDEO {
				pti.sdpRids = buffer.DefaultVideoLayersRid // could get updated from SDP
			}
			p.pendingTracks[req.Cid] = pti
		} else {
			p.pendingTracks[req.Cid].trackInfos = append(p.pendingTracks[req.Cid].trackInfos, ti)
		}
		p.pubLogger.Infow(
			"pending track queued",
			"trackID", ti.Sid,
			"request", logger.Proto(req),
			"pendingTrack", p.pendingTracks[req.Cid],
		)
		p.sendRequestResponse(&hublive.RequestResponse{
			Reason: hublive.RequestResponse_QUEUED,
			Request: &hublive.RequestResponse_AddTrack{
				AddTrack: utils.CloneProto(req),
			},
		})
		return nil
	}

	pti := &pendingTrackInfo{
		trackInfos: []*hublive.TrackInfo{ti},
		createdAt:  time.Now(),
	}
	if ti.Type == hublive.TrackType_VIDEO {
		pti.sdpRids = buffer.DefaultVideoLayersRid // could get updated from SDP
	}
	p.pendingTracks[req.Cid] = pti
	p.pubLogger.Debugw(
		"pending track added",
		"trackID", ti.Sid,
		"request", logger.Proto(req),
		"pendingTrack", p.pendingTracks[req.Cid],
	)
	return ti
}

func (p *ParticipantImpl) GetPendingTrack(trackID hublive.TrackID) *hublive.TrackInfo {
	p.pendingTracksLock.RLock()
	defer p.pendingTracksLock.RUnlock()

	for _, t := range p.pendingTracks {
		if hublive.TrackID(t.trackInfos[0].Sid) == trackID {
			return t.trackInfos[0]
		}
	}

	return nil
}

func (p *ParticipantImpl) SetTrackMuted(mute *hublive.MuteTrackRequest, fromAdmin bool) *hublive.TrackInfo {
	// when request is coming from admin, send message to current participant
	if fromAdmin {
		p.sendTrackMuted(hublive.TrackID(mute.Sid), mute.Muted)
	}

	return p.setTrackMuted(mute, fromAdmin)
}

func (p *ParticipantImpl) setTrackMuted(mute *hublive.MuteTrackRequest, fromAdmin bool) *hublive.TrackInfo {
	trackID := hublive.TrackID(mute.Sid)
	p.dirty.Store(true)
	if p.supervisor != nil {
		p.supervisor.SetPublicationMute(trackID, mute.Muted)
	}

	track, changed := p.UpTrackManager.SetPublishedTrackMuted(trackID, mute.Muted)
	var trackInfo *hublive.TrackInfo
	if track != nil {
		trackInfo = track.ToProto()
	}

	// update mute status in any pending/queued add track requests too
	p.pendingTracksLock.RLock()
	for _, pti := range p.pendingTracks {
		for i, ti := range pti.trackInfos {
			if hublive.TrackID(ti.Sid) == trackID {
				ti = utils.CloneProto(ti)
				changed = changed || ti.Muted != mute.Muted
				ti.Muted = mute.Muted
				pti.trackInfos[i] = ti
				if trackInfo == nil {
					trackInfo = ti
				}
			}
		}
	}
	p.pendingTracksLock.RUnlock()

	if trackInfo != nil && changed {
		if mute.Muted {
			p.params.TelemetryListener.OnTrackMuted(p.ID(), trackInfo)
		} else {
			p.params.TelemetryListener.OnTrackUnmuted(p.ID(), trackInfo)
		}
	}

	if trackInfo == nil && !fromAdmin {
		p.sendRequestResponse(&hublive.RequestResponse{
			Reason: hublive.RequestResponse_NOT_FOUND,
			Request: &hublive.RequestResponse_Mute{
				Mute: utils.CloneProto(mute),
			},
		})
	}

	return trackInfo
}

func (p *ParticipantImpl) mediaTrackReceived(
	track sfu.TrackRemote,
	rtpReceiver *webrtc.RTPReceiver,
) (*MediaTrack, bool, bool, buffer.VideoLayersRid) {
	p.pendingTracksLock.Lock()
	newTrack := false

	mid := p.TransportManager.GetPublisherMid(rtpReceiver)
	p.pubLogger.Debugw(
		"media track received",
		"kind", track.Kind().String(),
		"trackID", track.ID(),
		"rid", track.RID(),
		"ssrc", track.SSRC(),
		"rtxSsrc", track.RtxSSRC(),
		"mime", mime.NormalizeMimeType(track.Codec().MimeType),
		"mid", mid,
	)
	if mid == "" {
		p.pendingRemoteTracks = append(
			p.pendingRemoteTracks,
			&pendingRemoteTrack{track: track.RTCTrack(), receiver: rtpReceiver},
		)
		p.pendingTracksLock.Unlock()
		p.pubLogger.Warnw("could not get mid for track", nil, "trackID", track.ID())
		return nil, false, false, buffer.VideoLayersRid{}
	}

	// use existing media track to handle simulcast
	var pubTime time.Duration
	var isMigrated bool
	var ridsFromSdp buffer.VideoLayersRid
	mt, ok := p.getPublishedTrackBySdpCid(track.ID()).(*MediaTrack)
	if !ok {
		signalCid, ti, sdpRids, migrated, createdAt := p.getPendingTrack(track.ID(), ToProtoTrackKind(track.Kind()), true)
		ridsFromSdp = sdpRids
		if ti == nil {
			p.pendingRemoteTracks = append(
				p.pendingRemoteTracks,
				&pendingRemoteTrack{track: track.RTCTrack(), receiver: rtpReceiver},
			)
			p.pendingTracksLock.Unlock()
			return nil, false, false, ridsFromSdp
		}
		isMigrated = migrated

		// check if the migrated track has correct codec
		if migrated && len(ti.Codecs) > 0 {
			parameters := rtpReceiver.GetParameters()
			var codecFound int
			for _, c := range ti.Codecs {
				for _, nc := range parameters.Codecs {
					if mime.IsMimeTypeStringEqual(nc.MimeType, c.MimeType) {
						codecFound++
						break
					}
				}
			}
			if codecFound != len(ti.Codecs) {
				p.pubLogger.Warnw("migrated track codec mismatched", nil, "track", logger.Proto(ti), "webrtcCodec", parameters)
				p.pendingTracksLock.Unlock()
				p.IssueFullReconnect(types.ParticipantCloseReasonMigrateCodecMismatch)
				return nil, false, false, ridsFromSdp
			}
		}

		ti.MimeType = track.Codec().MimeType
		// set mime_type for tracks that the AddTrackRequest do not have simulcast_codecs set
		if len(ti.Codecs) == 1 && ti.Codecs[0].MimeType == "" {
			ti.Codecs[0].MimeType = track.Codec().MimeType
		}
		if utils.TimedVersionFromProto(ti.Version).IsZero() {
			// only assign version on a fresh publish, i. e. avoid updating version in scenarios like migration
			ti.Version = p.params.VersionGenerator.Next().ToProto()
		}

		mimeType := mime.NormalizeMimeType(ti.MimeType)
		for _, layer := range ti.Layers {
			layer.SpatialLayer = buffer.VideoQualityToSpatialLayer(mimeType, layer.Quality, ti)
			layer.Rid = buffer.VideoQualityToRid(mimeType, layer.Quality, ti, sdpRids)
		}

		mt = p.addMediaTrack(signalCid, ti)
		newTrack = true

		// if the addTrackRequest is sent before participant active then it means the client tries to publish
		// before fully connected, in this case we only record the time when the participant is active since
		// we want this metric to represent the time cost by publishing.
		if activeAt := p.lastActiveAt.Load(); activeAt != nil && createdAt.Before(*activeAt) {
			createdAt = *activeAt
		}
		pubTime = time.Since(createdAt)
		p.dirty.Store(true)
	}
	p.pendingTracksLock.Unlock()

	_, isReceiverAdded := mt.AddReceiver(rtpReceiver, track, mid)

	if newTrack {
		go func() {
			// TODO: remove this after we know where the high delay is coming from
			if pubTime > 3*time.Second {
				p.pubLogger.Infow(
					"track published with high delay",
					"trackID", mt.ID(),
					"track", logger.Proto(mt.ToProto()),
					"cost", pubTime.Milliseconds(),
					"rid", track.RID(),
					"mime", track.Codec().MimeType,
					"isMigrated", isMigrated,
				)
			} else {
				p.pubLogger.Debugw(
					"track published",
					"trackID", mt.ID(),
					"track", logger.Proto(mt.ToProto()),
					"cost", pubTime.Milliseconds(),
					"isMigrated", isMigrated,
				)
			}

			prometheus.RecordPublishTime(
				p.params.Country,
				mt.Source(),
				mt.Kind(),
				pubTime,
				p.GetClientInfo().GetSdk(),
				p.Kind(),
			)
			p.handleTrackPublished(mt, isMigrated)
		}()
	}

	return mt, newTrack, isReceiverAdded, ridsFromSdp
}

func (p *ParticipantImpl) addMigratedTrack(cid string, ti *hublive.TrackInfo) *MediaTrack {
	p.pubLogger.Infow("add migrated track", "cid", cid, "trackID", ti.Sid, "track", logger.Proto(ti))
	rtpReceiver := p.TransportManager.GetPublisherRTPReceiver(ti.Mid)
	if rtpReceiver == nil {
		p.pubLogger.Errorw(
			"could not find receiver for migrated track", nil,
			"trackID", ti.Sid,
			"mid", ti.Mid,
		)
		return nil
	}

	mt := p.addMediaTrack(cid, ti)

	potentialCodecs := make([]webrtc.RTPCodecParameters, 0, len(ti.Codecs))
	parameters := rtpReceiver.GetParameters()
	for _, c := range ti.Codecs {
		for _, nc := range parameters.Codecs {
			if mime.IsMimeTypeStringEqual(nc.MimeType, c.MimeType) {
				potentialCodecs = append(potentialCodecs, nc)
				break
			}
		}
	}
	// check for mime_type for tracks that do not have simulcast_codecs set
	if ti.MimeType != "" {
		for _, nc := range parameters.Codecs {
			if mime.IsMimeTypeStringEqual(nc.MimeType, ti.MimeType) {
				alreadyAdded := false
				for _, pc := range potentialCodecs {
					if mime.IsMimeTypeStringEqual(pc.MimeType, ti.MimeType) {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					potentialCodecs = append(potentialCodecs, nc)
				}
				break
			}
		}
	}
	mt.SetPotentialCodecs(potentialCodecs, parameters.HeaderExtensions)

	for _, codec := range ti.Codecs {
		for ssrc, info := range p.params.SimTracks {
			if info.Mid == codec.Mid && !info.IsRepairStream {
				mt.SetLayerSsrcsForRid(mime.NormalizeMimeType(codec.MimeType), info.StreamID, ssrc, info.RepairSSRC)
			}
		}
	}

	return mt
}

func (p *ParticipantImpl) addMediaTrack(signalCid string, ti *hublive.TrackInfo) *MediaTrack {
	mt := NewMediaTrack(MediaTrackParams{
		ParticipantID:         p.ID,
		ParticipantIdentity:   p.params.Identity,
		ParticipantVersion:    p.version.Load(),
		ParticipantCountry:    p.params.Country,
		BufferFactory:         p.params.Config.BufferFactory,
		ReceiverConfig:        p.params.Config.Receiver,
		AudioConfig:           p.params.AudioConfig,
		VideoConfig:           p.params.VideoConfig,
		TelemetryListener:     p.params.TelemetryListener,
		Logger:                LoggerWithTrack(p.pubLogger, hublive.TrackID(ti.Sid), false),
		Reporter:              p.params.Reporter.WithTrack(ti.Sid),
		SubscriberConfig:      p.params.Config.Subscriber,
		PLIThrottleConfig:     p.params.PLIThrottleConfig,
		SimTracks:             p.params.SimTracks,
		OnRTCP:                p.postRtcp,
		ForwardStats:          p.params.ForwardStats,
		OnTrackEverSubscribed: p.sendTrackHasBeenSubscribed,
		ShouldRegressCodec: func() bool {
			return p.helper().ShouldRegressCodec()
		},
		PreferVideoSizeFromMedia:         p.params.PreferVideoSizeFromMedia,
		EnableRTPStreamRestartDetection:  p.params.EnableRTPStreamRestartDetection,
		UpdateTrackInfoByVideoSizeChange: p.params.UseOneShotSignallingMode,
		ForceBackupCodecPolicySimulcast:  p.params.ForceBackupCodecPolicySimulcast,
	}, ti)

	mt.OnSubscribedMaxQualityChange(p.onSubscribedMaxQualityChange)
	mt.OnSubscribedAudioCodecChange(p.onSubscribedAudioCodecChange)

	// add to published and clean up pending
	if p.supervisor != nil {
		p.supervisor.SetPublishedTrack(hublive.TrackID(ti.Sid), mt)
	}
	p.UpTrackManager.AddPublishedTrack(mt)

	pti := p.pendingTracks[signalCid]
	if pti != nil {
		if p.pendingPublishingTracks[hublive.TrackID(ti.Sid)] != nil {
			p.pubLogger.Infow("unexpected pending publish track", "trackID", ti.Sid)
		}
		p.pendingPublishingTracks[hublive.TrackID(ti.Sid)] = &pendingTrackInfo{
			trackInfos: []*hublive.TrackInfo{pti.trackInfos[0]},
			migrated:   pti.migrated,
		}
	}

	p.pendingTracks[signalCid].trackInfos = p.pendingTracks[signalCid].trackInfos[1:]
	if len(p.pendingTracks[signalCid].trackInfos) == 0 {
		delete(p.pendingTracks, signalCid)
	} else {
		p.pendingTracks[signalCid].queued = true
		p.pendingTracks[signalCid].createdAt = time.Now()
	}

	trackID := hublive.TrackID(ti.Sid)
	mt.AddOnClose(func(isExpectedToResume bool) {
		if p.supervisor != nil {
			p.supervisor.ClearPublishedTrack(trackID, mt)
		}

		p.params.TelemetryListener.OnTrackUnpublished(
			p.ID(),
			p.Identity(),
			mt.ToProto(),
			!isExpectedToResume,
		)

		p.pendingTracksLock.Lock()
		if pti := p.pendingTracks[signalCid]; pti != nil {
			p.sendTrackPublished(signalCid, pti.trackInfos[0])
			pti.queued = false
		}
		p.pendingTracksLock.Unlock()
		p.handlePendingRemoteTracks()

		p.dirty.Store(true)

		p.pubLogger.Debugw(
			"track unpublished",
			"trackID", ti.Sid,
			"expectedToResume", isExpectedToResume,
			"track", logger.Proto(ti),
		)
		p.listener().OnTrackUnpublished(p, mt)
	})

	return mt
}

func (p *ParticipantImpl) handleTrackPublished(track types.MediaTrack, isMigrated bool) {
	p.listener().OnTrackPublished(p, track)

	// send webhook after callbacks are complete, persistence and state handling happens
	// in `onTrackPublished` cb
	p.params.TelemetryListener.OnTrackPublished(
		p.ID(),
		p.Identity(),
		track.ToProto(),
		!isMigrated,
	)

	p.pendingTracksLock.Lock()
	delete(p.pendingPublishingTracks, track.ID())
	p.pendingTracksLock.Unlock()
}

func (p *ParticipantImpl) hasPendingMigratedTrack() bool {
	p.pendingTracksLock.RLock()
	defer p.pendingTracksLock.RUnlock()

	for _, t := range p.pendingTracks {
		if t.migrated {
			return true
		}
	}

	for _, t := range p.pendingPublishingTracks {
		if t.migrated {
			return true
		}
	}

	return false
}

func (p *ParticipantImpl) onUpTrackManagerClose() {
	p.pubRTCPQueue.Stop()
}

func (p *ParticipantImpl) getPendingTrack(clientId string, kind hublive.TrackType, skipQueued bool) (string, *hublive.TrackInfo, buffer.VideoLayersRid, bool, time.Time) {
	signalCid := clientId
	pendingInfo := p.pendingTracks[clientId]
	if pendingInfo == nil {
	track_loop:
		for cid, pti := range p.pendingTracks {
			ti := pti.trackInfos[0]
			for _, c := range ti.Codecs {
				if c.Cid == clientId {
					pendingInfo = pti
					signalCid = cid
					break track_loop
				}
			}
		}

		if pendingInfo == nil {
			//
			// If no match on client id, find first one matching type
			// as MediaStreamTrack can change client id when transceiver
			// is added to peer connection.
			//
			for cid, pti := range p.pendingTracks {
				ti := pti.trackInfos[0]
				if ti.Type == kind {
					pendingInfo = pti
					signalCid = cid
					break
				}
			}
		}
	}

	// if still not found, we are done
	if pendingInfo == nil || (skipQueued && pendingInfo.queued) {
		return signalCid, nil, buffer.VideoLayersRid{}, false, time.Time{}
	}

	return signalCid, utils.CloneProto(pendingInfo.trackInfos[0]), pendingInfo.sdpRids, pendingInfo.migrated, pendingInfo.createdAt
}

func (p *ParticipantImpl) getPendingTrackPrimaryBySdpCid(sdpCid string) *pendingTrackInfo {
	for _, pti := range p.pendingTracks {
		ti := pti.trackInfos[0]
		if len(ti.Codecs) == 0 {
			continue
		}
		if ti.Codecs[0].Cid == sdpCid || ti.Codecs[0].SdpCid == sdpCid {
			return pti
		}
	}

	return nil
}

// setTrackID either generates a new TrackID for an AddTrackRequest
func (p *ParticipantImpl) setTrackID(cid string, info *hublive.TrackInfo) {
	var trackID string
	// if already pending, use the same SID
	// it is possible to have multiple AddTrackRequests for the same track
	if pti := p.pendingTracks[cid]; pti != nil {
		trackID = pti.trackInfos[0].Sid
	}

	// otherwise generate
	if trackID == "" {
		trackPrefix := utils.TrackPrefix
		switch info.Type {
		case hublive.TrackType_VIDEO:
			trackPrefix += "V"
		case hublive.TrackType_AUDIO:
			trackPrefix += "A"
		}

		switch info.Source {
		case hublive.TrackSource_CAMERA:
			trackPrefix += "C"
		case hublive.TrackSource_MICROPHONE:
			trackPrefix += "M"
		case hublive.TrackSource_SCREEN_SHARE:
			trackPrefix += "S"
		case hublive.TrackSource_SCREEN_SHARE_AUDIO:
			trackPrefix += "s"
		}
		trackID = guid.New(trackPrefix)
	}
	info.Sid = trackID
}

func (p *ParticipantImpl) getPublishedTrackBySignalCid(clientId string) types.MediaTrack {
	for _, publishedTrack := range p.GetPublishedTracks() {
		if publishedTrack.(types.LocalMediaTrack).HasSignalCid(clientId) {
			p.pubLogger.Debugw("found track by signal cid", "signalCid", clientId, "trackID", publishedTrack.ID())
			return publishedTrack
		}
	}

	return nil
}

func (p *ParticipantImpl) getPublishedTrackBySdpCid(clientId string) types.MediaTrack {
	for _, publishedTrack := range p.GetPublishedTracks() {
		if publishedTrack.(types.LocalMediaTrack).HasSdpCid(clientId) {
			p.pubLogger.Debugw("found track by SDP cid", "sdpCid", clientId, "trackID", publishedTrack.ID())
			return publishedTrack
		}
	}

	return nil
}

func (p *ParticipantImpl) postRtcp(pkts []rtcp.Packet) {
	p.lock.RLock()
	migrationTimer := p.migrationTimer
	p.lock.RUnlock()

	// Once migration out is active, layers getting added would not be communicated to
	// where the publisher is migrating to. Without SSRC, `UnhandleSimulcastInterceptor`
	// cannot be set up on the migrating in node. Without that interceptor, simulcast
	// probing will fail.
	//
	// Clients usually send `rid` RTP header extension till they get an RTCP Receiver Report
	// from the remote side. So, by curbing RTCP when migration is active, even if a new layer
	// get published to this node, client should continue to send `rid` to the new node
	// post migration and the new node can do regular simulcast probing (without the
	// `UnhandleSimulcastInterceptor`) to fire `OnTrack` on that layer. And when the new node
	// sends RTCP Receiver Report back to the client, client will stop `rid`.
	if migrationTimer != nil {
		return
	}

	p.pubRTCPQueue.Enqueue(func(op postRtcpOp) {
		if err := op.TransportManager.WritePublisherRTCP(op.pkts); err != nil && !IsEOF(err) {
			op.pubLogger.Errorw("could not write RTCP to participant", err)
		}
	}, postRtcpOp{p, pkts})
}

func (p *ParticipantImpl) setDownTracksConnected() {
	for _, t := range p.SubscriptionManager.GetSubscribedTracks() {
		if dt := t.DownTrack(); dt != nil {
			dt.SeedState(sfu.DownTrackState{ForwarderState: p.getAndDeleteForwarderState(t.ID())})
			dt.SetConnected()
		}
	}
}

func (p *ParticipantImpl) cacheForwarderState() {
	// if migrating in, get forwarder state from migrating out node to facilitate resume
	if fs, err := p.helper().GetSubscriberForwarderState(p); err == nil && fs != nil {
		p.lock.Lock()
		p.forwarderState = fs
		p.lock.Unlock()

		for _, t := range p.SubscriptionManager.GetSubscribedTracks() {
			if dt := t.DownTrack(); dt != nil {
				dt.SeedState(sfu.DownTrackState{ForwarderState: p.getAndDeleteForwarderState(t.ID())})
			}
		}
	}
}

func (p *ParticipantImpl) getAndDeleteForwarderState(trackID hublive.TrackID) *hublive.RTPForwarderState {
	p.lock.Lock()
	fs := p.forwarderState[trackID]
	delete(p.forwarderState, trackID)
	p.lock.Unlock()

	return fs
}

func (p *ParticipantImpl) CacheDownTrack(trackID hublive.TrackID, rtpTransceiver *webrtc.RTPTransceiver, downTrack sfu.DownTrackState) {
	p.lock.Lock()
	if existing := p.cachedDownTracks[trackID]; existing != nil && existing.transceiver != rtpTransceiver {
		p.subLogger.Warnw("cached transceiver changed", nil, "trackID", trackID)
	}
	p.cachedDownTracks[trackID] = &downTrackState{transceiver: rtpTransceiver, downTrack: downTrack}
	p.subLogger.Debugw("caching downtrack", "trackID", trackID)
	p.lock.Unlock()
}

func (p *ParticipantImpl) UncacheDownTrack(rtpTransceiver *webrtc.RTPTransceiver) {
	p.lock.Lock()
	for trackID, dts := range p.cachedDownTracks {
		if dts.transceiver == rtpTransceiver {
			if dts := p.cachedDownTracks[trackID]; dts != nil {
				p.subLogger.Debugw("uncaching downtrack", "trackID", trackID)
			}
			delete(p.cachedDownTracks, trackID)
			break
		}
	}
	p.lock.Unlock()
}

func (p *ParticipantImpl) GetCachedDownTrack(trackID hublive.TrackID) (*webrtc.RTPTransceiver, sfu.DownTrackState) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if dts := p.cachedDownTracks[trackID]; dts != nil {
		return dts.transceiver, dts.downTrack
	}

	return nil, sfu.DownTrackState{}
}

func (p *ParticipantImpl) onPublicationError(trackID hublive.TrackID) {
	if p.params.ReconnectOnPublicationError {
		p.pubLogger.Infow("issuing full reconnect on publication error", "trackID", trackID)
		p.IssueFullReconnect(types.ParticipantCloseReasonPublicationError)
	}
}

func (p *ParticipantImpl) UpdateSubscribedQuality(nodeID hublive.NodeID, trackID hublive.TrackID, maxQualities []types.SubscribedCodecQuality) error {
	track := p.GetPublishedTrack(trackID)
	if track == nil {
		p.pubLogger.Debugw("could not find track", "trackID", trackID)
		return errors.New("could not find published track")
	}

	track.(types.LocalMediaTrack).NotifySubscriberNodeMaxQuality(nodeID, maxQualities)
	return nil
}

func (p *ParticipantImpl) UpdateSubscribedAudioCodecs(nodeID hublive.NodeID, trackID hublive.TrackID, codecs []*hublive.SubscribedAudioCodec) error {
	track := p.GetPublishedTrack(trackID)
	if track == nil {
		p.pubLogger.Debugw("could not find track", "trackID", trackID)
		return errors.New("could not find published track")
	}

	track.(types.LocalMediaTrack).NotifySubscriptionNode(nodeID, codecs)
	return nil
}

func (p *ParticipantImpl) UpdateMediaLoss(nodeID hublive.NodeID, trackID hublive.TrackID, fractionalLoss uint32) error {
	track := p.GetPublishedTrack(trackID)
	if track == nil {
		p.pubLogger.Debugw("could not find track", "trackID", trackID)
		return errors.New("could not find published track")
	}

	track.(types.LocalMediaTrack).NotifySubscriberNodeMediaLoss(nodeID, uint8(fractionalLoss))
	return nil
}

func (p *ParticipantImpl) setupEnabledCodecs(publishEnabledCodecs []*hublive.Codec, subscribeEnabledCodecs []*hublive.Codec, disabledCodecs *hublive.DisabledCodecs) {
	shouldDisable := func(c *hublive.Codec, disabled []*hublive.Codec) bool {
		for _, disableCodec := range disabled {
			// disable codec's fmtp is empty means disable this codec entirely
			if mime.IsMimeTypeStringEqual(c.Mime, disableCodec.Mime) {
				return true
			}
		}
		return false
	}

	publishCodecsAudio := make([]*hublive.Codec, 0, len(publishEnabledCodecs))
	publishCodecsVideo := make([]*hublive.Codec, 0, len(publishEnabledCodecs))
	for _, c := range publishEnabledCodecs {
		if shouldDisable(c, disabledCodecs.GetCodecs()) || shouldDisable(c, disabledCodecs.GetPublish()) {
			continue
		}

		// sort by compatibility, since we will look for backups in these.
		if mime.IsMimeTypeStringVP8(c.Mime) {
			if len(p.enabledPublishCodecs) > 0 {
				p.enabledPublishCodecs = slices.Insert(p.enabledPublishCodecs, 0, c)
			} else {
				p.enabledPublishCodecs = append(p.enabledPublishCodecs, c)
			}
		} else if mime.IsMimeTypeStringH264(c.Mime) {
			p.enabledPublishCodecs = append(p.enabledPublishCodecs, c)
		} else {
			if mime.IsMimeTypeStringAudio(c.Mime) {
				publishCodecsAudio = append(publishCodecsAudio, c)
			} else {
				publishCodecsVideo = append(publishCodecsVideo, c)
			}
		}
	}
	// list all video first and then audio to work around a client side issue with Flutter SDK 2.4.2
	p.enabledPublishCodecs = append(p.enabledPublishCodecs, publishCodecsVideo...)
	p.enabledPublishCodecs = append(p.enabledPublishCodecs, publishCodecsAudio...)

	subscribeCodecs := make([]*hublive.Codec, 0, len(subscribeEnabledCodecs))
	for _, c := range subscribeEnabledCodecs {
		if shouldDisable(c, disabledCodecs.GetCodecs()) {
			continue
		}
		subscribeCodecs = append(subscribeCodecs, c)
	}
	p.enabledSubscribeCodecs = subscribeCodecs
	p.params.Logger.Debugw(
		"setup enabled codecs",
		"publish", logger.ProtoSlice(p.enabledPublishCodecs),
		"subscribe", logger.ProtoSlice(p.enabledSubscribeCodecs),
		"disabled", logger.Proto(disabledCodecs),
	)
}

func (p *ParticipantImpl) GetEnabledPublishCodecs() []*hublive.Codec {
	codecs := make([]*hublive.Codec, 0, len(p.enabledPublishCodecs))
	for _, c := range p.enabledPublishCodecs {
		if mime.IsMimeTypeStringRTX(c.Mime) {
			continue
		}
		codecs = append(codecs, c)
	}
	return codecs
}

func (p *ParticipantImpl) UpdateAudioTrack(update *hublive.UpdateLocalAudioTrack) error {
	if track := p.UpTrackManager.UpdatePublishedAudioTrack(update); track != nil {
		return nil
	}

	isPending := false
	p.pendingTracksLock.RLock()
	for _, pti := range p.pendingTracks {
		for _, ti := range pti.trackInfos {
			if ti.Sid == update.TrackSid {
				isPending = true

				ti.AudioFeatures = sutils.DedupeSlice(update.Features)
				ti.Stereo = false
				ti.DisableDtx = false
				for _, feature := range update.Features {
					switch feature {
					case hublive.AudioTrackFeature_TF_STEREO:
						ti.Stereo = true
					case hublive.AudioTrackFeature_TF_NO_DTX:
						ti.DisableDtx = true
					}
				}

				p.pubLogger.Debugw("updated pending track", "trackID", ti.Sid, "trackInfo", logger.Proto(ti))
			}
		}
	}
	p.pendingTracksLock.RUnlock()
	if isPending {
		return nil
	}

	p.pubLogger.Debugw("could not locate track", "trackID", update.TrackSid)
	p.sendRequestResponse(&hublive.RequestResponse{
		Reason: hublive.RequestResponse_NOT_FOUND,
		Request: &hublive.RequestResponse_UpdateAudioTrack{
			UpdateAudioTrack: utils.CloneProto(update),
		},
	})
	return errors.New("could not find track")
}

func (p *ParticipantImpl) UpdateVideoTrack(update *hublive.UpdateLocalVideoTrack) error {
	if track := p.UpTrackManager.UpdatePublishedVideoTrack(update); track != nil {
		return nil
	}

	isPending := false
	p.pendingTracksLock.RLock()
	for _, pti := range p.pendingTracks {
		for _, ti := range pti.trackInfos {
			if ti.Sid == update.TrackSid {
				isPending = true

				ti.Width = update.Width
				ti.Height = update.Height
			}
		}
	}
	p.pendingTracksLock.RUnlock()
	if isPending {
		return nil
	}

	p.pubLogger.Debugw("could not locate track", "trackID", update.TrackSid)
	p.sendRequestResponse(&hublive.RequestResponse{
		Reason: hublive.RequestResponse_NOT_FOUND,
		Request: &hublive.RequestResponse_UpdateVideoTrack{
			UpdateVideoTrack: utils.CloneProto(update),
		},
	})
	return errors.New("could not find track")
}
