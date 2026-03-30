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
	"sync"
	"time"

	"github.com/maxhubsv/hublive-server/pkg/utils"
	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/webhook"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . TelemetryService
type TelemetryService interface {
	// TrackStats is called periodically for each track in both directions (published/subscribed)
	TrackStats(roomID hublive.RoomID, roomName hublive.RoomName, key StatsKey, stat *hublive.AnalyticsStat)

	// events
	RoomStarted(ctx context.Context, room *hublive.Room)
	RoomEnded(ctx context.Context, room *hublive.Room)

	// ParticipantJoined - a participant establishes signal connection to a room
	ParticipantJoined(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, clientInfo *hublive.ClientInfo, clientMeta *hublive.AnalyticsClientMeta, shouldSendEvent bool, guard *ReferenceGuard)
	// ParticipantActive - a participant establishes media connection
	ParticipantActive(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, clientMeta *hublive.AnalyticsClientMeta, isMigration bool, guard *ReferenceGuard)
	// ParticipantResumed - there has been an ICE restart or connection resume attempt, and we've received their signal connection
	ParticipantResumed(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, nodeID hublive.NodeID, reason hublive.ReconnectReason)
	// ParticipantLeft - the participant leaves the room, only sent if ParticipantActive has been called before
	ParticipantLeft(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, shouldSendEvent bool, guard *ReferenceGuard)
	// TrackPublishRequested - a publication attempt has been received
	TrackPublishRequested(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, identity hublive.ParticipantIdentity, track *hublive.TrackInfo)
	// TrackPublished - a publication attempt has been successful
	TrackPublished(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, identity hublive.ParticipantIdentity, track *hublive.TrackInfo, shouldSendEvent bool)
	// TrackUnpublished - a participant unpublished a track
	TrackUnpublished(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, identity hublive.ParticipantIdentity, track *hublive.TrackInfo, shouldSendEvent bool)
	// TrackSubscribeRequested - a participant requested to subscribe to a track
	TrackSubscribeRequested(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo)
	// TrackSubscribed - a participant subscribed to a track successfully
	TrackSubscribed(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo, publisher *hublive.ParticipantInfo, shouldSendEvent bool)
	// TrackUnsubscribed - a participant unsubscribed from a track successfully
	TrackUnsubscribed(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo, shouldSendEvent bool)
	// TrackSubscribeFailed - failure to subscribe to a track
	TrackSubscribeFailed(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, trackID hublive.TrackID, err error, isUserError bool)
	// TrackMuted - the publisher has muted the Track
	TrackMuted(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo)
	// TrackUnmuted - the publisher has muted the Track
	TrackUnmuted(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo)
	// TrackPublishedUpdate - track metadata has been updated
	TrackPublishedUpdate(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo)
	// TrackMaxSubscribedVideoQuality - publisher is notified of the max quality subscribers desire
	TrackMaxSubscribedVideoQuality(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo, mime mime.MimeType, maxQuality hublive.VideoQuality)
	TrackPublishRTPStats(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, layer int, stats *hublive.RTPStats)
	TrackSubscribeRTPStats(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, stats *hublive.RTPStats)

	EgressStarted(ctx context.Context, info *hublive.EgressInfo)
	EgressUpdated(ctx context.Context, info *hublive.EgressInfo)
	EgressEnded(ctx context.Context, info *hublive.EgressInfo)

	IngressCreated(ctx context.Context, info *hublive.IngressInfo)
	IngressDeleted(ctx context.Context, info *hublive.IngressInfo)
	IngressStarted(ctx context.Context, info *hublive.IngressInfo)
	IngressUpdated(ctx context.Context, info *hublive.IngressInfo)
	IngressEnded(ctx context.Context, info *hublive.IngressInfo)

	LocalRoomState(ctx context.Context, info *hublive.AnalyticsNodeRooms)

	Report(ctx context.Context, reportInfo *hublive.ReportInfo)

	APICall(ctx context.Context, apiCallInfo *hublive.APICallInfo)

	Webhook(ctx context.Context, webhookInfo *hublive.WebhookInfo)

	// helpers
	AnalyticsService
	NotifyEgressEvent(ctx context.Context, event string, info *hublive.EgressInfo)
	FlushStats()
}

// -----------------------------

var _ TelemetryService = (*NullTelemetryService)(nil)

type NullTelemetryService struct {
	NullAnalyticService
}

func (n NullTelemetryService) TrackStats(roomID hublive.RoomID, roomName hublive.RoomName, key StatsKey, stat *hublive.AnalyticsStat) {
}
func (n NullTelemetryService) RoomStarted(ctx context.Context, room *hublive.Room) {}
func (n NullTelemetryService) RoomEnded(ctx context.Context, room *hublive.Room)   {}
func (n NullTelemetryService) ParticipantJoined(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, clientInfo *hublive.ClientInfo, clientMeta *hublive.AnalyticsClientMeta, shouldSendEvent bool, guard *ReferenceGuard) {
}
func (n NullTelemetryService) ParticipantActive(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, clientMeta *hublive.AnalyticsClientMeta, isMigration bool, guard *ReferenceGuard) {
}
func (n NullTelemetryService) ParticipantResumed(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, nodeID hublive.NodeID, reason hublive.ReconnectReason) {
}
func (n NullTelemetryService) ParticipantLeft(ctx context.Context, room *hublive.Room, participant *hublive.ParticipantInfo, shouldSendEvent bool, guard *ReferenceGuard) {
}
func (n NullTelemetryService) TrackPublishRequested(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, identity hublive.ParticipantIdentity, track *hublive.TrackInfo) {
}
func (n NullTelemetryService) TrackPublished(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, identity hublive.ParticipantIdentity, track *hublive.TrackInfo, shouldSendEvent bool) {
}
func (n NullTelemetryService) TrackUnpublished(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, identity hublive.ParticipantIdentity, track *hublive.TrackInfo, shouldSendEvent bool) {
}
func (n NullTelemetryService) TrackSubscribeRequested(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo) {
}
func (n NullTelemetryService) TrackSubscribed(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo, publisher *hublive.ParticipantInfo, shouldSendEvent bool) {
}
func (n NullTelemetryService) TrackUnsubscribed(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo, shouldSendEvent bool) {
}
func (n NullTelemetryService) TrackSubscribeFailed(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, trackID hublive.TrackID, err error, isUserError bool) {
}
func (n NullTelemetryService) TrackMuted(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo) {
}
func (n NullTelemetryService) TrackUnmuted(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo) {
}
func (n NullTelemetryService) TrackPublishedUpdate(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo) {
}
func (n NullTelemetryService) TrackMaxSubscribedVideoQuality(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, track *hublive.TrackInfo, mime mime.MimeType, maxQuality hublive.VideoQuality) {
}
func (n NullTelemetryService) TrackPublishRTPStats(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, layer int, stats *hublive.RTPStats) {
}
func (n NullTelemetryService) TrackSubscribeRTPStats(ctx context.Context, roomID hublive.RoomID, roomName hublive.RoomName, participantID hublive.ParticipantID, trackID hublive.TrackID, mimeType mime.MimeType, stats *hublive.RTPStats) {
}
func (n NullTelemetryService) EgressStarted(ctx context.Context, info *hublive.EgressInfo)          {}
func (n NullTelemetryService) EgressUpdated(ctx context.Context, info *hublive.EgressInfo)          {}
func (n NullTelemetryService) EgressEnded(ctx context.Context, info *hublive.EgressInfo)            {}
func (n NullTelemetryService) IngressCreated(ctx context.Context, info *hublive.IngressInfo)        {}
func (n NullTelemetryService) IngressDeleted(ctx context.Context, info *hublive.IngressInfo)        {}
func (n NullTelemetryService) IngressStarted(ctx context.Context, info *hublive.IngressInfo)        {}
func (n NullTelemetryService) IngressUpdated(ctx context.Context, info *hublive.IngressInfo)        {}
func (n NullTelemetryService) IngressEnded(ctx context.Context, info *hublive.IngressInfo)          {}
func (n NullTelemetryService) LocalRoomState(ctx context.Context, info *hublive.AnalyticsNodeRooms) {}
func (n NullTelemetryService) Report(ctx context.Context, reportInfo *hublive.ReportInfo)           {}
func (n NullTelemetryService) APICall(ctx context.Context, apiCallInfo *hublive.APICallInfo)        {}
func (n NullTelemetryService) Webhook(ctx context.Context, webhookInfo *hublive.WebhookInfo)        {}
func (n NullTelemetryService) NotifyEgressEvent(ctx context.Context, event string, info *hublive.EgressInfo) {
}
func (n NullTelemetryService) FlushStats() {}

// -----------------------------

const (
	workerCleanupWait = 3 * time.Minute
	jobsQueueMinSize  = 2048

	telemetryStatsUpdateInterval = time.Second * 30
)

type statsWorkerKey struct {
	roomID        hublive.RoomID
	participantID hublive.ParticipantID
}

type telemetryService struct {
	AnalyticsService

	notifier  webhook.QueuedNotifier
	jobsQueue *utils.OpsQueue

	workersMu  sync.RWMutex
	workers    map[statsWorkerKey]*StatsWorker
	workerList *StatsWorker

	flushMu sync.Mutex
}

func NewTelemetryService(notifier webhook.QueuedNotifier, analytics AnalyticsService) TelemetryService {
	t := &telemetryService{
		AnalyticsService: analytics,
		notifier:         notifier,
		jobsQueue: utils.NewOpsQueue(utils.OpsQueueParams{
			Name:        "telemetry",
			MinSize:     jobsQueueMinSize,
			FlushOnStop: true,
			Logger:      logger.GetLogger(),
		}),
		workers: make(map[statsWorkerKey]*StatsWorker),
	}
	if t.notifier != nil {
		t.notifier.RegisterProcessedHook(func(ctx context.Context, whi *hublive.WebhookInfo) {
			t.Webhook(ctx, whi)
		})
	}

	t.jobsQueue.Start()
	go t.run()

	return t
}

func (t *telemetryService) FlushStats() {
	t.flushMu.Lock()
	defer t.flushMu.Unlock()

	t.workersMu.RLock()
	worker := t.workerList
	t.workersMu.RUnlock()

	now := time.Now()
	var prev, reap *StatsWorker
	for worker != nil {
		next := worker.next
		if closed := worker.Flush(now, workerCleanupWait); closed {
			if prev == nil {
				// this worker was at the head of the list
				t.workersMu.Lock()
				p := &t.workerList
				for *p != worker {
					// new workers have been added. scan until we find the one
					// immediately before this
					prev = *p
					p = &prev.next
				}
				*p = worker.next
				t.workersMu.Unlock()
			} else {
				prev.next = worker.next
			}

			worker.next = reap
			reap = worker
		} else {
			prev = worker
		}
		worker = next
	}

	if reap != nil {
		t.workersMu.Lock()
		for reap != nil {
			key := statsWorkerKey{reap.roomID, reap.participantID}
			if reap == t.workers[key] {
				delete(t.workers, key)
			}
			reap = reap.next
		}
		t.workersMu.Unlock()
	}
}

func (t *telemetryService) run() {
	for range time.Tick(telemetryStatsUpdateInterval) {
		t.FlushStats()
	}
}

func (t *telemetryService) enqueue(op func()) {
	t.jobsQueue.Enqueue(op)
}

func (t *telemetryService) getWorker(roomID hublive.RoomID, participantID hublive.ParticipantID) (worker *StatsWorker, ok bool) {
	t.workersMu.RLock()
	defer t.workersMu.RUnlock()

	worker, ok = t.workers[statsWorkerKey{roomID, participantID}]
	return
}

func (t *telemetryService) getOrCreateWorker(
	ctx context.Context,
	roomID hublive.RoomID,
	roomName hublive.RoomName,
	participantID hublive.ParticipantID,
	participantIdentity hublive.ParticipantIdentity,
	guard *ReferenceGuard,
) (*StatsWorker, bool) {
	t.workersMu.Lock()
	defer t.workersMu.Unlock()

	key := statsWorkerKey{roomID, participantID}
	worker, ok := t.workers[key]
	if ok && !worker.Closed(guard) {
		return worker, true
	}

	existingIsConnected := false
	if ok {
		existingIsConnected = worker.IsConnected()
	}

	worker = newStatsWorker(
		ctx,
		t,
		roomID,
		roomName,
		participantID,
		participantIdentity,
		guard,
	)
	if existingIsConnected {
		worker.SetConnected()
	}

	t.workers[key] = worker

	worker.next = t.workerList
	t.workerList = worker

	return worker, false
}

func (t *telemetryService) LocalRoomState(ctx context.Context, info *hublive.AnalyticsNodeRooms) {
	t.enqueue(func() {
		t.SendNodeRoomStates(ctx, info)
	})
}
