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
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/telemetry"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/rpc"
	"__GITHUB_HUBLIVE__protocol/webhook"
)

type EgressLauncher interface {
	StartEgress(context.Context, *rpc.StartEgressRequest) (*hublive.EgressInfo, error)
	StopEgress(context.Context, *hublive.StopEgressRequest) (*hublive.EgressInfo, error)
}

func StartParticipantEgress(
	ctx context.Context,
	launcher EgressLauncher,
	ts telemetry.TelemetryService,
	opts *hublive.AutoParticipantEgress,
	identity hublive.ParticipantIdentity,
	roomName hublive.RoomName,
	roomID hublive.RoomID,
) error {
	if req, err := startParticipantEgress(ctx, launcher, opts, identity, roomName, roomID); err != nil {
		// send egress failed webhook

		info := &hublive.EgressInfo{
			RoomId:   string(roomID),
			RoomName: string(roomName),
			Status:   hublive.EgressStatus_EGRESS_FAILED,
			Error:    err.Error(),
			Request:  &hublive.EgressInfo_Participant{Participant: req},
		}

		ts.NotifyEgressEvent(ctx, webhook.EventEgressEnded, info)

		return err
	}
	return nil
}

func startParticipantEgress(
	ctx context.Context,
	launcher EgressLauncher,
	opts *hublive.AutoParticipantEgress,
	identity hublive.ParticipantIdentity,
	roomName hublive.RoomName,
	roomID hublive.RoomID,
) (*hublive.ParticipantEgressRequest, error) {
	req := &hublive.ParticipantEgressRequest{
		RoomName:       string(roomName),
		Identity:       string(identity),
		FileOutputs:    opts.FileOutputs,
		SegmentOutputs: opts.SegmentOutputs,
	}

	switch o := opts.Options.(type) {
	case *hublive.AutoParticipantEgress_Preset:
		req.Options = &hublive.ParticipantEgressRequest_Preset{Preset: o.Preset}
	case *hublive.AutoParticipantEgress_Advanced:
		req.Options = &hublive.ParticipantEgressRequest_Advanced{Advanced: o.Advanced}
	}

	if launcher == nil {
		return req, errors.New("egress launcher not found")
	}

	_, err := launcher.StartEgress(ctx, &rpc.StartEgressRequest{
		Request: &rpc.StartEgressRequest_Participant{
			Participant: req,
		},
		RoomId: string(roomID),
	})
	return req, err
}

func StartTrackEgress(
	ctx context.Context,
	launcher EgressLauncher,
	ts telemetry.TelemetryService,
	opts *hublive.AutoTrackEgress,
	track types.MediaTrack,
	roomName hublive.RoomName,
	roomID hublive.RoomID,
) error {
	if req, err := startTrackEgress(ctx, launcher, opts, track, roomName, roomID); err != nil {
		// send egress failed webhook

		info := &hublive.EgressInfo{
			RoomId:   string(roomID),
			RoomName: string(roomName),
			Status:   hublive.EgressStatus_EGRESS_FAILED,
			Error:    err.Error(),
			Request:  &hublive.EgressInfo_Track{Track: req},
		}
		ts.NotifyEgressEvent(ctx, webhook.EventEgressEnded, info)

		return err
	}
	return nil
}

func startTrackEgress(
	ctx context.Context,
	launcher EgressLauncher,
	opts *hublive.AutoTrackEgress,
	track types.MediaTrack,
	roomName hublive.RoomName,
	roomID hublive.RoomID,
) (*hublive.TrackEgressRequest, error) {
	output := &hublive.DirectFileOutput{
		Filepath: getFilePath(opts.Filepath),
	}

	switch out := opts.Output.(type) {
	case *hublive.AutoTrackEgress_Azure:
		output.Output = &hublive.DirectFileOutput_Azure{Azure: out.Azure}
	case *hublive.AutoTrackEgress_Gcp:
		output.Output = &hublive.DirectFileOutput_Gcp{Gcp: out.Gcp}
	case *hublive.AutoTrackEgress_S3:
		output.Output = &hublive.DirectFileOutput_S3{S3: out.S3}
	}

	req := &hublive.TrackEgressRequest{
		RoomName: string(roomName),
		TrackId:  string(track.ID()),
		Output: &hublive.TrackEgressRequest_File{
			File: output,
		},
	}

	if launcher == nil {
		return req, errors.New("egress launcher not found")
	}

	_, err := launcher.StartEgress(ctx, &rpc.StartEgressRequest{
		Request: &rpc.StartEgressRequest_Track{
			Track: req,
		},
		RoomId: string(roomID),
	})
	return req, err
}

func getFilePath(filepath string) string {
	if filepath == "" || strings.HasSuffix(filepath, "/") || strings.Contains(filepath, "{track_id}") {
		return filepath
	}

	idx := strings.Index(filepath, ".")
	if idx == -1 {
		return fmt.Sprintf("%s-{track_id}", filepath)
	} else {
		return fmt.Sprintf("%s-%s%s", filepath[:idx], "{track_id}", filepath[idx:])
	}
}
