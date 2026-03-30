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

package telemetry

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/maxhubsv/hublive-server/pkg/telemetry/prometheus"
	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/egress"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/utils/guid"
	"__GITHUB_HUBLIVE__protocol/webhook"
)

func (t *telemetryService) NotifyEvent(ctx context.Context, event *hublive.WebhookEvent, opts ...webhook.NotifyOption) {
	if t.notifier == nil {
		return
	}

	event.CreatedAt = time.Now().Unix()
	event.Id = guid.New("EV_")

	if err := t.notifier.QueueNotify(ctx, event, opts...); err != nil {
		logger.Warnw("failed to notify webhook", err, "event", event.Event)
	}
}

func (t *telemetryService) RoomStarted(ctx context.Context, room *hublive.Room) {
	t.enqueue(func() {
		t.NotifyEvent(ctx, &hublive.WebhookEvent{
			Event: webhook.EventRoomStarted,
			Room:  room,
		})

		t.SendEvent(ctx, &hublive.AnalyticsEvent{
			Type:      hublive.AnalyticsEventType_ROOM_CREATED,
			Timestamp: &timestamppb.Timestamp{Seconds: room.CreationTime},
			Room:      room,
		})
	})
}

func (t *telemetryService) RoomEnded(ctx context.Context, room *hublive.Room) {
	t.enqueue(func() {
		t.NotifyEvent(ctx, &hublive.WebhookEvent{
			Event: webhook.EventRoomFinished,
			Room:  room,
		})

		t.SendEvent(ctx, &hublive.AnalyticsEvent{
			Type:      hublive.AnalyticsEventType_ROOM_ENDED,
			Timestamp: timestamppb.Now(),
			RoomId:    room.Sid,
			Room:      room,
		})
	})
}

func (t *telemetryService) ParticipantJoined(
	ctx context.Context,
	room *hublive.Room,
	participant *hublive.ParticipantInfo,
	clientInfo *hublive.ClientInfo,
	clientMeta *hublive.AnalyticsClientMeta,
	shouldSendEvent bool,
	guard *ReferenceGuard,
) {
	t.enqueue(func() {
		_, found := t.getOrCreateWorker(
			ctx,
			hublive.RoomID(room.Sid),
			hublive.RoomName(room.Name),
			hublive.ParticipantID(participant.Sid),
			hublive.ParticipantIdentity(participant.Identity),
			guard,
		)
		if !found {
			prometheus.IncrementParticipantRtcConnected(1)
			prometheus.AddParticipant()
		}

		if shouldSendEvent {
			ev := newParticipantEvent(hublive.AnalyticsEventType_PARTICIPANT_JOINED, room, participant)
			ev.ClientInfo = clientInfo
			ev.ClientMeta = clientMeta
			t.SendEvent(ctx, ev)
		}
	})
}

func (t *telemetryService) ParticipantActive(
	ctx context.Context,
	room *hublive.Room,
	participant *hublive.ParticipantInfo,
	clientMeta *hublive.AnalyticsClientMeta,
	isMigration bool,
	guard *ReferenceGuard,
) {
	t.enqueue(func() {
		if !isMigration {
			// a participant is considered "joined" only when they become "active"
			t.NotifyEvent(ctx, &hublive.WebhookEvent{
				Event:       webhook.EventParticipantJoined,
				Room:        room,
				Participant: participant,
			})
		}

		worker, found := t.getOrCreateWorker(
			ctx,
			hublive.RoomID(room.Sid),
			hublive.RoomName(room.Name),
			hublive.ParticipantID(participant.Sid),
			hublive.ParticipantIdentity(participant.Identity),
			guard,
		)
		if !found {
			prometheus.AddParticipant()
		}
		worker.SetConnected()

		ev := newParticipantEvent(hublive.AnalyticsEventType_PARTICIPANT_ACTIVE, room, participant)
		ev.ClientMeta = clientMeta
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) ParticipantResumed(
	ctx context.Context,
	room *hublive.Room,
	participant *hublive.ParticipantInfo,
	nodeID hublive.NodeID,
	reason hublive.ReconnectReason,
) {
	t.enqueue(func() {
		// create a worker if needed.
		//
		// Signalling channel stats collector and media channel stats collector could both call
		// ParticipantJoined and ParticipantLeft.
		//
		// On a resume, the signalling channel collector would call `ParticipantLeft` which would close
		// the corresponding participant's stats worker.
		//
		// So, on a successful resume, create the worker if needed.
		_, found := t.getOrCreateWorker(
			ctx,
			hublive.RoomID(room.Sid),
			hublive.RoomName(room.Name),
			hublive.ParticipantID(participant.Sid),
			hublive.ParticipantIdentity(participant.Identity),
			nil,
		)
		if !found {
			prometheus.AddParticipant()
		}

		ev := newParticipantEvent(hublive.AnalyticsEventType_PARTICIPANT_RESUMED, room, participant)
		ev.ClientMeta = &hublive.AnalyticsClientMeta{
			Node:            string(nodeID),
			ReconnectReason: reason,
		}
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) ParticipantLeft(ctx context.Context,
	room *hublive.Room,
	participant *hublive.ParticipantInfo,
	shouldSendEvent bool,
	guard *ReferenceGuard,
) {
	t.enqueue(func() {
		isConnected := false
		if worker, ok := t.getWorker(hublive.RoomID(room.Sid), hublive.ParticipantID(participant.Sid)); ok {
			isConnected = worker.IsConnected()
			if worker.Close(guard) {
				prometheus.SubParticipant()
			} else {
				logger.Infow(
					"stats worker active",
					"room", room.Name,
					"roomID", room.Sid,
					"participant", participant.Identity,
					"participantID", participant.Sid,
					"worker", worker,
				)
			}
		}

		if shouldSendEvent {
			webhookEvent := webhook.EventParticipantLeft
			analyticsEvent := hublive.AnalyticsEventType_PARTICIPANT_LEFT
			if !isConnected {
				webhookEvent = webhook.EventParticipantConnectionAborted
				analyticsEvent = hublive.AnalyticsEventType_PARTICIPANT_CONNECTION_ABORTED
			}
			t.NotifyEvent(ctx, &hublive.WebhookEvent{
				Event:       webhookEvent,
				Room:        room,
				Participant: participant,
			})

			t.SendEvent(ctx, newParticipantEvent(analyticsEvent, room, participant))
		}
	})
}

func (t *telemetryService) TrackPublishRequested(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	identity hublive.ParticipantIdentity,
	track *hublive.TrackInfo,
) {
	t.enqueue(func() {
		prometheus.RecordTrackPublishAttempt(track.Type.String())
		room := toMinimalRoomProto(roomID, roomName)
		ev := newTrackEvent(hublive.AnalyticsEventType_TRACK_PUBLISH_REQUESTED, room, participantID, track)
		if ev.Participant != nil {
			ev.Participant.Identity = string(identity)
		}
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) TrackPublished(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	identity hublive.ParticipantIdentity,
	track *hublive.TrackInfo,
	shouldSendEvent bool,
) {
	t.enqueue(func() {
		prometheus.AddPublishedTrack(track.Type.String())
		prometheus.RecordTrackPublishSuccess(track.Type.String())
		if !shouldSendEvent {
			return
		}

		room := toMinimalRoomProto(roomID, roomName)
		participant := &hublive.ParticipantInfo{
			Sid:      string(participantID),
			Identity: string(identity),
		}
		t.NotifyEvent(ctx, &hublive.WebhookEvent{
			Event:       webhook.EventTrackPublished,
			Room:        room,
			Participant: participant,
			Track:       track,
		})

		ev := newTrackEvent(hublive.AnalyticsEventType_TRACK_PUBLISHED, room, participantID, track)
		ev.Participant = participant
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) TrackPublishedUpdate(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	track *hublive.TrackInfo,
) {
	t.enqueue(func() {
		room := toMinimalRoomProto(roomID, roomName)
		t.SendEvent(ctx, newTrackEvent(hublive.AnalyticsEventType_TRACK_PUBLISHED_UPDATE, room, participantID, track))
	})
}

func (t *telemetryService) TrackMaxSubscribedVideoQuality(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	track *hublive.TrackInfo,
	mime mime.MimeType,
	maxQuality hublive.VideoQuality,
) {
	t.enqueue(func() {
		room := toMinimalRoomProto(roomID, roomName)
		ev := newTrackEvent(hublive.AnalyticsEventType_TRACK_MAX_SUBSCRIBED_VIDEO_QUALITY, room, participantID, track)
		ev.MaxSubscribedVideoQuality = maxQuality
		ev.Mime = mime.String()
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) TrackSubscribeRequested(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	track *hublive.TrackInfo,
) {
	t.enqueue(func() {
		prometheus.RecordTrackSubscribeAttempt()

		room := toMinimalRoomProto(roomID, roomName)
		ev := newTrackEvent(hublive.AnalyticsEventType_TRACK_SUBSCRIBE_REQUESTED, room, participantID, track)
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) TrackSubscribed(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	track *hublive.TrackInfo,
	publisher *hublive.ParticipantInfo,
	shouldSendEvent bool,
) {
	t.enqueue(func() {
		prometheus.RecordTrackSubscribeSuccess(track.Type.String())

		if !shouldSendEvent {
			return
		}

		room := toMinimalRoomProto(roomID, roomName)
		ev := newTrackEvent(hublive.AnalyticsEventType_TRACK_SUBSCRIBED, room, participantID, track)
		ev.Publisher = publisher
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) TrackSubscribeFailed(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	trackID hublive.TrackID,
	err error,
	isUserError bool,
) {
	t.enqueue(func() {
		prometheus.RecordTrackSubscribeFailure(err, isUserError)

		room := toMinimalRoomProto(roomID, roomName)
		ev := newTrackEvent(hublive.AnalyticsEventType_TRACK_SUBSCRIBE_FAILED, room, participantID, &hublive.TrackInfo{
			Sid: string(trackID),
		})
		ev.Error = err.Error()
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) TrackUnsubscribed(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	track *hublive.TrackInfo,
	shouldSendEvent bool,
) {
	t.enqueue(func() {
		prometheus.RecordTrackUnsubscribed(track.Type.String())

		if shouldSendEvent {
			room := toMinimalRoomProto(roomID, roomName)
			t.SendEvent(ctx, newTrackEvent(hublive.AnalyticsEventType_TRACK_UNSUBSCRIBED, room, participantID, track))
		}
	})
}

func (t *telemetryService) TrackUnpublished(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	identity hublive.ParticipantIdentity,
	track *hublive.TrackInfo,
	shouldSendEvent bool,
) {
	t.enqueue(func() {
		prometheus.SubPublishedTrack(track.Type.String())
		if !shouldSendEvent {
			return
		}

		room := toMinimalRoomProto(roomID, roomName)
		participant := &hublive.ParticipantInfo{
			Sid:      string(participantID),
			Identity: string(identity),
		}
		t.NotifyEvent(ctx, &hublive.WebhookEvent{
			Event:       webhook.EventTrackUnpublished,
			Room:        room,
			Participant: participant,
			Track:       track,
		})

		t.SendEvent(ctx, newTrackEvent(hublive.AnalyticsEventType_TRACK_UNPUBLISHED, room, participantID, track))
	})
}

func (t *telemetryService) TrackMuted(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	track *hublive.TrackInfo,
) {
	t.enqueue(func() {
		room := toMinimalRoomProto(roomID, roomName)
		t.SendEvent(ctx, newTrackEvent(hublive.AnalyticsEventType_TRACK_MUTED, room, participantID, track))
	})
}

func (t *telemetryService) TrackUnmuted(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	track *hublive.TrackInfo,
) {
	t.enqueue(func() {
		room := toMinimalRoomProto(roomID, roomName)
		t.SendEvent(ctx, newTrackEvent(hublive.AnalyticsEventType_TRACK_UNMUTED, room, participantID, track))
	})
}

func (t *telemetryService) TrackPublishRTPStats(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	trackID hublive.TrackID,
	mimeType mime.MimeType,
	layer int,
	stats *hublive.RTPStats,
) {
	t.enqueue(func() {
		room := toMinimalRoomProto(roomID, roomName)
		ev := newRoomEvent(hublive.AnalyticsEventType_TRACK_PUBLISH_STATS, room)
		ev.ParticipantId = string(participantID)
		ev.TrackId = string(trackID)
		ev.Mime = mimeType.String()
		ev.VideoLayer = int32(layer)
		ev.RtpStats = stats
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) TrackSubscribeRTPStats(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	trackID hublive.TrackID,
	mimeType mime.MimeType,
	stats *hublive.RTPStats,
) {
	t.enqueue(func() {
		room := toMinimalRoomProto(roomID, roomName)
		ev := newRoomEvent(hublive.AnalyticsEventType_TRACK_SUBSCRIBE_STATS, room)
		ev.ParticipantId = string(participantID)
		ev.TrackId = string(trackID)
		ev.Mime = mimeType.String()
		ev.RtpStats = stats
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) NotifyEgressEvent(ctx context.Context, event string, info *hublive.EgressInfo) {
	opts := egress.GetEgressNotifyOptions(info)

	t.NotifyEvent(ctx, &hublive.WebhookEvent{
		Event:      event,
		EgressInfo: info,
	}, opts...)
}

func (t *telemetryService) EgressStarted(ctx context.Context, info *hublive.EgressInfo) {

	t.enqueue(func() {
		t.NotifyEgressEvent(ctx, webhook.EventEgressStarted, info)

		t.SendEvent(ctx, newEgressEvent(hublive.AnalyticsEventType_EGRESS_STARTED, info))
	})
}

func (t *telemetryService) EgressUpdated(ctx context.Context, info *hublive.EgressInfo) {
	t.enqueue(func() {
		t.NotifyEgressEvent(ctx, webhook.EventEgressUpdated, info)

		t.SendEvent(ctx, newEgressEvent(hublive.AnalyticsEventType_EGRESS_UPDATED, info))
	})
}

func (t *telemetryService) EgressEnded(ctx context.Context, info *hublive.EgressInfo) {
	t.enqueue(func() {
		t.NotifyEgressEvent(ctx, webhook.EventEgressEnded, info)

		t.SendEvent(ctx, newEgressEvent(hublive.AnalyticsEventType_EGRESS_ENDED, info))
	})
}

func (t *telemetryService) IngressCreated(ctx context.Context, info *hublive.IngressInfo) {
	t.enqueue(func() {
		t.SendEvent(ctx, newIngressEvent(hublive.AnalyticsEventType_INGRESS_CREATED, info))
	})
}

func (t *telemetryService) IngressDeleted(ctx context.Context, info *hublive.IngressInfo) {
	t.enqueue(func() {
		t.SendEvent(ctx, newIngressEvent(hublive.AnalyticsEventType_INGRESS_DELETED, info))
	})
}

func (t *telemetryService) IngressStarted(ctx context.Context, info *hublive.IngressInfo) {
	t.enqueue(func() {
		t.NotifyEvent(ctx, &hublive.WebhookEvent{
			Event:       webhook.EventIngressStarted,
			IngressInfo: info,
		})

		t.SendEvent(ctx, newIngressEvent(hublive.AnalyticsEventType_INGRESS_STARTED, info))
	})
}

func (t *telemetryService) IngressUpdated(ctx context.Context, info *hublive.IngressInfo) {
	t.enqueue(func() {
		t.SendEvent(ctx, newIngressEvent(hublive.AnalyticsEventType_INGRESS_UPDATED, info))
	})
}

func (t *telemetryService) IngressEnded(ctx context.Context, info *hublive.IngressInfo) {
	t.enqueue(func() {
		t.NotifyEvent(ctx, &hublive.WebhookEvent{
			Event:       webhook.EventIngressEnded,
			IngressInfo: info,
		})

		t.SendEvent(ctx, newIngressEvent(hublive.AnalyticsEventType_INGRESS_ENDED, info))
	})
}

func (t *telemetryService) Report(ctx context.Context, reportInfo *hublive.ReportInfo) {
	t.enqueue(func() {
		ev := &hublive.AnalyticsEvent{
			Type:      hublive.AnalyticsEventType_REPORT,
			Timestamp: timestamppb.Now(),
			Report:    reportInfo,
		}
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) APICall(ctx context.Context, apiCallInfo *hublive.APICallInfo) {
	t.enqueue(func() {
		ev := &hublive.AnalyticsEvent{
			Type:      hublive.AnalyticsEventType_API_CALL,
			Timestamp: timestamppb.Now(),
			ApiCall:   apiCallInfo,
		}
		t.SendEvent(ctx, ev)
	})
}

func (t *telemetryService) Webhook(ctx context.Context, webhookInfo *hublive.WebhookInfo) {
	t.enqueue(func() {
		ev := &hublive.AnalyticsEvent{
			Type:      hublive.AnalyticsEventType_WEBHOOK,
			Timestamp: timestamppb.Now(),
			Webhook:   webhookInfo,
		}
		t.SendEvent(ctx, ev)
	})
}

func newRoomEvent(event hublive.AnalyticsEventType, room *hublive.Room) *hublive.AnalyticsEvent {
	ev := &hublive.AnalyticsEvent{
		Type:      event,
		Timestamp: timestamppb.Now(),
	}
	if room != nil {
		ev.Room = room
		ev.RoomId = room.Sid
	}
	return ev
}

func newParticipantEvent(event hublive.AnalyticsEventType, room *hublive.Room, participant *hublive.ParticipantInfo) *hublive.AnalyticsEvent {
	ev := newRoomEvent(event, room)
	if participant != nil {
		ev.ParticipantId = participant.Sid
		ev.Participant = participant
	}
	return ev
}

func newTrackEvent(event hublive.AnalyticsEventType, room *hublive.Room, participantID hublive.ParticipantID, track *hublive.TrackInfo) *hublive.AnalyticsEvent {
	ev := newParticipantEvent(event, room, &hublive.ParticipantInfo{
		Sid: string(participantID),
	})
	if track != nil {
		ev.TrackId = track.Sid
		ev.Track = track
	}
	return ev
}

func newEgressEvent(event hublive.AnalyticsEventType, egress *hublive.EgressInfo) *hublive.AnalyticsEvent {
	return &hublive.AnalyticsEvent{
		Type:      event,
		Timestamp: timestamppb.Now(),
		EgressId:  egress.EgressId,
		RoomId:    egress.RoomId,
		Egress:    egress,
	}
}

func newIngressEvent(event hublive.AnalyticsEventType, ingress *hublive.IngressInfo) *hublive.AnalyticsEvent {
	return &hublive.AnalyticsEvent{
		Type:      event,
		Timestamp: timestamppb.Now(),
		IngressId: ingress.IngressId,
		Ingress:   ingress,
	}
}

func toMinimalRoomProto(roomID hublive.RoomID, roomName hublive.RoomName) *hublive.Room {
	return &hublive.Room{
		Sid:  string(roomID),
		Name: string(roomName),
	}
}
