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

	"google.golang.org/protobuf/proto"
)

var _ ParticipantSignalling = (*signallingUnimplemented)(nil)

type signallingUnimplemented struct{}

func (u *signallingUnimplemented) SignalJoinResponse(join *hublive.JoinResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalParticipantUpdate(participants []*hublive.ParticipantInfo) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalSpeakerUpdate(speakers []*hublive.SpeakerInfo) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalRoomUpdate(room *hublive.Room) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalConnectionQualityUpdate(connectionQuality *hublive.ConnectionQualityUpdate) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalRefreshToken(token string) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalRequestResponse(requestResponse *hublive.RequestResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalRoomMovedResponse(roomMoved *hublive.RoomMovedResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalReconnectResponse(reconnect *hublive.ReconnectResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalICECandidate(trickle *hublive.TrickleRequest) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalTrackMuted(mute *hublive.MuteTrackRequest) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalTrackPublished(trackPublished *hublive.TrackPublishedResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalTrackUnpublished(trackUnpublished *hublive.TrackUnpublishedResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalTrackSubscribed(trackSubscribed *hublive.TrackSubscribed) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalLeaveRequest(leave *hublive.LeaveRequest) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalSdpAnswer(answer *hublive.SessionDescription) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalSdpOffer(offer *hublive.SessionDescription) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalStreamStateUpdate(streamStateUpdate *hublive.StreamStateUpdate) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalSubscribedQualityUpdate(subscribedQualityUpdate *hublive.SubscribedQualityUpdate) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalSubscriptionResponse(subscriptionResponse *hublive.SubscriptionResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalSubscriptionPermissionUpdate(subscriptionPermissionUpdate *hublive.SubscriptionPermissionUpdate) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalMediaSectionsRequirement(mediaSectionsRequirement *hublive.MediaSectionsRequirement) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalSubscribedAudioCodecUpdate(subscribedAudioCodecUpdate *hublive.SubscribedAudioCodecUpdate) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalPublishDataTrackResponse(publishDataTrackResponse *hublive.PublishDataTrackResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalUnpublishDataTrackResponse(unpublishDataTrackResponse *hublive.UnpublishDataTrackResponse) proto.Message {
	return nil
}

func (u *signallingUnimplemented) SignalDataTrackSubscriberHandles(dataTrackSubscriberHandles *hublive.DataTrackSubscriberHandles) proto.Message {
	return nil
}
