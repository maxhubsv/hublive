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
	"__GITHUB_HUBLIVE__protocol/logger"

	"google.golang.org/protobuf/proto"
)

var _ ParticipantSignalling = (*signalling)(nil)

type SignallingParams struct {
	Logger logger.Logger
}

type signalling struct {
	signallingUnimplemented

	params SignallingParams
}

func NewSignalling(params SignallingParams) ParticipantSignalling {
	return &signalling{
		params: params,
	}
}

func (s *signalling) SignalJoinResponse(join *hublive.JoinResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Join{
			Join: join,
		},
	}
}

func (s *signalling) SignalParticipantUpdate(participants []*hublive.ParticipantInfo) proto.Message {
	if len(participants) == 0 {
		return nil
	}

	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Update{
			Update: &hublive.ParticipantUpdate{
				Participants: participants,
			},
		},
	}
}

func (s *signalling) SignalSpeakerUpdate(speakers []*hublive.SpeakerInfo) proto.Message {
	if len(speakers) == 0 {
		return nil
	}

	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_SpeakersChanged{
			SpeakersChanged: &hublive.SpeakersChanged{
				Speakers: speakers,
			},
		},
	}
}

func (s *signalling) SignalRoomUpdate(room *hublive.Room) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_RoomUpdate{
			RoomUpdate: &hublive.RoomUpdate{
				Room: room,
			},
		},
	}
}

func (s *signalling) SignalConnectionQualityUpdate(connectionQuality *hublive.ConnectionQualityUpdate) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_ConnectionQuality{
			ConnectionQuality: connectionQuality,
		},
	}
}

func (s *signalling) SignalRefreshToken(token string) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_RefreshToken{
			RefreshToken: token,
		},
	}
}

func (s *signalling) SignalRequestResponse(requestResponse *hublive.RequestResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_RequestResponse{
			RequestResponse: requestResponse,
		},
	}
}

func (s *signalling) SignalRoomMovedResponse(roomMoved *hublive.RoomMovedResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_RoomMoved{
			RoomMoved: roomMoved,
		},
	}
}

func (s *signalling) SignalReconnectResponse(reconnect *hublive.ReconnectResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Reconnect{
			Reconnect: reconnect,
		},
	}
}

func (s *signalling) SignalICECandidate(trickle *hublive.TrickleRequest) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Trickle{
			Trickle: trickle,
		},
	}
}

func (s *signalling) SignalTrackMuted(mute *hublive.MuteTrackRequest) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Mute{
			Mute: mute,
		},
	}
}

func (s *signalling) SignalTrackPublished(trackPublished *hublive.TrackPublishedResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_TrackPublished{
			TrackPublished: trackPublished,
		},
	}
}

func (s *signalling) SignalTrackUnpublished(trackUnpublished *hublive.TrackUnpublishedResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_TrackUnpublished{
			TrackUnpublished: trackUnpublished,
		},
	}
}

func (s *signalling) SignalTrackSubscribed(trackSubscribed *hublive.TrackSubscribed) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_TrackSubscribed{
			TrackSubscribed: trackSubscribed,
		},
	}
}

func (s *signalling) SignalLeaveRequest(leave *hublive.LeaveRequest) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Leave{
			Leave: leave,
		},
	}
}

func (s *signalling) SignalSdpAnswer(answer *hublive.SessionDescription) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Answer{
			Answer: answer,
		},
	}
}

func (s *signalling) SignalSdpOffer(offer *hublive.SessionDescription) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_Offer{
			Offer: offer,
		},
	}
}

func (s *signalling) SignalStreamStateUpdate(streamStateUpdate *hublive.StreamStateUpdate) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_StreamStateUpdate{
			StreamStateUpdate: streamStateUpdate,
		},
	}
}

func (s *signalling) SignalSubscribedQualityUpdate(subscribedQualityUpdate *hublive.SubscribedQualityUpdate) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_SubscribedQualityUpdate{
			SubscribedQualityUpdate: subscribedQualityUpdate,
		},
	}
}

func (s *signalling) SignalSubscriptionResponse(subscriptionResponse *hublive.SubscriptionResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_SubscriptionResponse{
			SubscriptionResponse: subscriptionResponse,
		},
	}
}

func (s *signalling) SignalSubscriptionPermissionUpdate(subscriptionPermissionUpdate *hublive.SubscriptionPermissionUpdate) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_SubscriptionPermissionUpdate{
			SubscriptionPermissionUpdate: subscriptionPermissionUpdate,
		},
	}
}

func (u *signalling) SignalMediaSectionsRequirement(mediaSectionsRequirement *hublive.MediaSectionsRequirement) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_MediaSectionsRequirement{
			MediaSectionsRequirement: mediaSectionsRequirement,
		},
	}
}

func (s *signalling) SignalSubscribedAudioCodecUpdate(subscribedAudioCodecUpdate *hublive.SubscribedAudioCodecUpdate) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_SubscribedAudioCodecUpdate{
			SubscribedAudioCodecUpdate: subscribedAudioCodecUpdate,
		},
	}
}

func (u *signalling) SignalPublishDataTrackResponse(publishDataTrackResponse *hublive.PublishDataTrackResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_PublishDataTrackResponse{
			PublishDataTrackResponse: publishDataTrackResponse,
		},
	}
}

func (u *signalling) SignalUnpublishDataTrackResponse(unpublishDataTrackResponse *hublive.UnpublishDataTrackResponse) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_UnpublishDataTrackResponse{
			UnpublishDataTrackResponse: unpublishDataTrackResponse,
		},
	}
}

func (u *signalling) SignalDataTrackSubscriberHandles(dataTrackSubscriberHandles *hublive.DataTrackSubscriberHandles) proto.Message {
	return &hublive.SignalResponse{
		Message: &hublive.SignalResponse_DataTrackSubscriberHandles{
			DataTrackSubscriberHandles: dataTrackSubscriberHandles,
		},
	}
}
