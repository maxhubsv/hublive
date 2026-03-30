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
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/observability/roomobs"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/guid"

	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/rtc/types/typesfakes"
)

func NewMockParticipant(
	identity hublive.ParticipantIdentity,
	protocol types.ProtocolVersion,
	hidden bool,
	publisher bool,
	participantListener types.LocalParticipantListener,
) *typesfakes.FakeLocalParticipant {
	p := &typesfakes.FakeLocalParticipant{}
	sid := guid.New(utils.ParticipantPrefix)
	p.IDReturns(hublive.ParticipantID(sid))
	p.IdentityReturns(identity)
	p.StateReturns(hublive.ParticipantInfo_JOINED)
	p.ProtocolVersionReturns(protocol)
	p.CanSubscribeReturns(true)
	p.CanPublishSourceReturns(!hidden)
	p.CanPublishDataReturns(!hidden)
	p.HiddenReturns(hidden)
	p.ToProtoReturns(&hublive.ParticipantInfo{
		Sid:         sid,
		Identity:    string(identity),
		State:       hublive.ParticipantInfo_JOINED,
		IsPublisher: publisher,
	})
	p.ToProtoWithVersionReturns(&hublive.ParticipantInfo{
		Sid:         sid,
		Identity:    string(identity),
		State:       hublive.ParticipantInfo_JOINED,
		IsPublisher: publisher,
	}, utils.TimedVersion(0))

	p.SetMetadataCalls(func(m string) {
		participantListener.OnParticipantUpdate(p)
	})
	updateTrack := func() {
		participantListener.OnTrackUpdated(p, NewMockTrack(hublive.TrackType_VIDEO, "testcam"))
	}

	p.SetTrackMutedCalls(func(mute *hublive.MuteTrackRequest, fromServer bool) *hublive.TrackInfo {
		updateTrack()
		return nil
	})
	p.AddTrackCalls(func(req *hublive.AddTrackRequest) {
		updateTrack()
	})
	p.GetLoggerReturns(logger.GetLogger())
	p.GetReporterReturns(roomobs.NewNoopParticipantSessionReporter())

	return p
}

func NewMockTrack(kind hublive.TrackType, name string) *typesfakes.FakeMediaTrack {
	t := &typesfakes.FakeMediaTrack{}
	t.IDReturns(hublive.TrackID(guid.New(utils.TrackPrefix)))
	t.KindReturns(kind)
	t.NameReturns(name)
	t.ToProtoReturns(&hublive.TrackInfo{
		Type: kind,
		Name: name,
	})
	return t
}
