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

package signalling

import (
	"__GITHUB_HUBLIVE__protocol/hublive"

	"github.com/maxhubsv/hublive-server/pkg/routing"
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"

	"google.golang.org/protobuf/proto"
)

type ParticipantSignalHandler interface {
	HandleMessage(msg proto.Message) error
}

type ParticipantSignaller interface {
	SwapResponseSink(sink routing.MessageSink, reason types.SignallingCloseReason)
	GetResponseSink() routing.MessageSink
	CloseSignalConnection(reason types.SignallingCloseReason)

	WriteMessage(msg proto.Message) error
}

type ParticipantSignalling interface {
	SignalJoinResponse(join *hublive.JoinResponse) proto.Message
	SignalParticipantUpdate(participants []*hublive.ParticipantInfo) proto.Message
	SignalSpeakerUpdate(speakers []*hublive.SpeakerInfo) proto.Message
	SignalRoomUpdate(room *hublive.Room) proto.Message
	SignalConnectionQualityUpdate(connectionQuality *hublive.ConnectionQualityUpdate) proto.Message
	SignalRefreshToken(token string) proto.Message
	SignalRequestResponse(requestResponse *hublive.RequestResponse) proto.Message
	SignalRoomMovedResponse(roomMoved *hublive.RoomMovedResponse) proto.Message
	SignalReconnectResponse(reconnect *hublive.ReconnectResponse) proto.Message
	SignalICECandidate(trickle *hublive.TrickleRequest) proto.Message
	SignalTrackMuted(mute *hublive.MuteTrackRequest) proto.Message
	SignalTrackPublished(trackPublished *hublive.TrackPublishedResponse) proto.Message
	SignalTrackUnpublished(trackUnpublished *hublive.TrackUnpublishedResponse) proto.Message
	SignalTrackSubscribed(trackSubscribed *hublive.TrackSubscribed) proto.Message
	SignalLeaveRequest(leave *hublive.LeaveRequest) proto.Message
	SignalSdpAnswer(answer *hublive.SessionDescription) proto.Message
	SignalSdpOffer(offer *hublive.SessionDescription) proto.Message
	SignalStreamStateUpdate(streamStateUpdate *hublive.StreamStateUpdate) proto.Message
	SignalSubscribedQualityUpdate(subscribedQualityUpdate *hublive.SubscribedQualityUpdate) proto.Message
	SignalSubscriptionResponse(subscriptionResponse *hublive.SubscriptionResponse) proto.Message
	SignalSubscriptionPermissionUpdate(subscriptionPermissionUpdate *hublive.SubscriptionPermissionUpdate) proto.Message
	SignalMediaSectionsRequirement(mediaSectionsRequirement *hublive.MediaSectionsRequirement) proto.Message
	SignalSubscribedAudioCodecUpdate(subscribedAudioCodecUpdate *hublive.SubscribedAudioCodecUpdate) proto.Message
	SignalPublishDataTrackResponse(publishDataTrackResponse *hublive.PublishDataTrackResponse) proto.Message
	SignalUnpublishDataTrackResponse(unpublishDataTrackResponse *hublive.UnpublishDataTrackResponse) proto.Message
	SignalDataTrackSubscriberHandles(dataTrackSubscriberHandles *hublive.DataTrackSubscriberHandles) proto.Message
}
