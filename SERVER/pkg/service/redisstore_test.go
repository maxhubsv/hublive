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

package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"__GITHUB_HUBLIVE__protocol/ingress"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/guid"

	"github.com/maxhubsv/hublive-server/pkg/service"
)

func redisStoreDocker(t testing.TB) *service.RedisStore {
	return service.NewRedisStore(redisClientDocker(t))
}

func redisStore(t testing.TB) *service.RedisStore {
	return service.NewRedisStore(redisClient(t))
}

func TestRoomInternal(t *testing.T) {
	ctx := context.Background()
	rs := redisStore(t)

	room := &hublive.Room{
		Sid:  "123",
		Name: "test_room",
	}
	internal := &hublive.RoomInternal{
		TrackEgress: &hublive.AutoTrackEgress{Filepath: "egress"},
	}

	require.NoError(t, rs.StoreRoom(ctx, room, internal))
	actualRoom, actualInternal, err := rs.LoadRoom(ctx, hublive.RoomName(room.Name), true)
	require.NoError(t, err)
	require.Equal(t, room.Sid, actualRoom.Sid)
	require.Equal(t, internal.TrackEgress.Filepath, actualInternal.TrackEgress.Filepath)

	// remove internal
	require.NoError(t, rs.StoreRoom(ctx, room, nil))
	_, actualInternal, err = rs.LoadRoom(ctx, hublive.RoomName(room.Name), true)
	require.NoError(t, err)
	require.Nil(t, actualInternal)

	// clean up
	require.NoError(t, rs.DeleteRoom(ctx, "test_room"))
}

func TestParticipantPersistence(t *testing.T) {
	ctx := context.Background()
	rs := redisStore(t)

	roomName := hublive.RoomName("room1")
	_ = rs.DeleteRoom(ctx, roomName)

	p := &hublive.ParticipantInfo{
		Sid:      "PA_test",
		Identity: "test",
		State:    hublive.ParticipantInfo_ACTIVE,
		Tracks: []*hublive.TrackInfo{
			{
				Sid:  "track1",
				Type: hublive.TrackType_AUDIO,
				Name: "audio",
			},
		},
	}

	// create the participant
	require.NoError(t, rs.StoreParticipant(ctx, roomName, p))

	// result should match
	pGet, err := rs.LoadParticipant(ctx, roomName, hublive.ParticipantIdentity(p.Identity))
	require.NoError(t, err)
	require.Equal(t, p.Identity, pGet.Identity)
	require.Equal(t, len(p.Tracks), len(pGet.Tracks))
	require.Equal(t, p.Tracks[0].Sid, pGet.Tracks[0].Sid)

	// list should return one participant
	participants, err := rs.ListParticipants(ctx, roomName)
	require.NoError(t, err)
	require.Len(t, participants, 1)

	// deleting participant should return to normal
	require.NoError(t, rs.DeleteParticipant(ctx, roomName, hublive.ParticipantIdentity(p.Identity)))

	participants, err = rs.ListParticipants(ctx, roomName)
	require.NoError(t, err)
	require.Len(t, participants, 0)

	// shouldn't be able to get it
	_, err = rs.LoadParticipant(ctx, roomName, hublive.ParticipantIdentity(p.Identity))
	require.Equal(t, err, service.ErrParticipantNotFound)
}

func TestRoomLock(t *testing.T) {
	ctx := context.Background()
	rs := redisStore(t)
	lockInterval := 5 * time.Millisecond
	roomName := hublive.RoomName("myroom")

	t.Run("normal locking", func(t *testing.T) {
		token, err := rs.LockRoom(ctx, roomName, lockInterval)
		require.NoError(t, err)
		require.NotEmpty(t, token)
		require.NoError(t, rs.UnlockRoom(ctx, roomName, token))
	})

	t.Run("waits before acquiring lock", func(t *testing.T) {
		token, err := rs.LockRoom(ctx, roomName, lockInterval)
		require.NoError(t, err)
		require.NotEmpty(t, token)
		unlocked := atomic.NewUint32(0)
		wg := sync.WaitGroup{}

		wg.Add(1)
		go func() {
			// attempt to lock again
			defer wg.Done()
			token2, err := rs.LockRoom(ctx, roomName, lockInterval)
			require.NoError(t, err)
			defer rs.UnlockRoom(ctx, roomName, token2)
			require.Equal(t, uint32(1), unlocked.Load())
		}()

		// release after 2 ms
		time.Sleep(2 * time.Millisecond)
		unlocked.Store(1)
		_ = rs.UnlockRoom(ctx, roomName, token)

		wg.Wait()
	})

	t.Run("lock expires", func(t *testing.T) {
		token, err := rs.LockRoom(ctx, roomName, lockInterval)
		require.NoError(t, err)
		defer rs.UnlockRoom(ctx, roomName, token)

		time.Sleep(lockInterval + time.Millisecond)
		token2, err := rs.LockRoom(ctx, roomName, lockInterval)
		require.NoError(t, err)
		_ = rs.UnlockRoom(ctx, roomName, token2)
	})
}

func TestEgressStore(t *testing.T) {
	ctx := context.Background()
	rs := redisStore(t)

	roomName := "egress-test"

	// store egress info
	info := &hublive.EgressInfo{
		EgressId: guid.New(utils.EgressPrefix),
		RoomId:   guid.New(utils.RoomPrefix),
		RoomName: roomName,
		Status:   hublive.EgressStatus_EGRESS_STARTING,
		Request: &hublive.EgressInfo_RoomComposite{
			RoomComposite: &hublive.RoomCompositeEgressRequest{
				RoomName: roomName,
				Layout:   "speaker-dark",
			},
		},
	}
	require.NoError(t, rs.StoreEgress(ctx, info))

	// load
	res, err := rs.LoadEgress(ctx, info.EgressId)
	require.NoError(t, err)
	require.Equal(t, res.EgressId, info.EgressId)

	// store another
	info2 := &hublive.EgressInfo{
		EgressId: guid.New(utils.EgressPrefix),
		RoomId:   guid.New(utils.RoomPrefix),
		RoomName: "another-egress-test",
		Status:   hublive.EgressStatus_EGRESS_STARTING,
		Request: &hublive.EgressInfo_RoomComposite{
			RoomComposite: &hublive.RoomCompositeEgressRequest{
				RoomName: "another-egress-test",
				Layout:   "speaker-dark",
			},
		},
	}
	require.NoError(t, rs.StoreEgress(ctx, info2))

	// update
	info2.Status = hublive.EgressStatus_EGRESS_COMPLETE
	info2.EndedAt = time.Now().Add(-24 * time.Hour).UnixNano()
	require.NoError(t, rs.UpdateEgress(ctx, info))

	// list
	list, err := rs.ListEgress(ctx, "", false)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// list by room
	list, err = rs.ListEgress(ctx, hublive.RoomName(roomName), false)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// update
	info.Status = hublive.EgressStatus_EGRESS_COMPLETE
	info.EndedAt = time.Now().Add(-24 * time.Hour).UnixNano()
	require.NoError(t, rs.UpdateEgress(ctx, info))

	// clean
	require.NoError(t, rs.CleanEndedEgress())

	// list
	list, err = rs.ListEgress(ctx, hublive.RoomName(roomName), false)
	require.NoError(t, err)
	require.Len(t, list, 0)
}

func TestIngressStore(t *testing.T) {
	ctx := context.Background()
	rs := redisStore(t)

	info := &hublive.IngressInfo{
		IngressId: "ingressId",
		StreamKey: "streamKey",
		State: &hublive.IngressState{
			StartedAt: 2,
		},
	}

	err := rs.StoreIngress(ctx, info)
	require.NoError(t, err)

	err = rs.UpdateIngressState(ctx, info.IngressId, info.State)
	require.NoError(t, err)

	t.Cleanup(func() {
		rs.DeleteIngress(ctx, info)
	})

	pulledInfo, err := rs.LoadIngress(ctx, "ingressId")
	require.NoError(t, err)
	compareIngressInfo(t, pulledInfo, info)

	infos, err := rs.ListIngress(ctx, "room")
	require.NoError(t, err)
	require.Equal(t, 0, len(infos))

	info.RoomName = "room"
	err = rs.UpdateIngress(ctx, info)
	require.NoError(t, err)

	infos, err = rs.ListIngress(ctx, "room")
	require.NoError(t, err)

	require.NoError(t, err)
	require.Equal(t, 1, len(infos))
	compareIngressInfo(t, infos[0], info)

	info.RoomName = ""
	err = rs.UpdateIngress(ctx, info)
	require.NoError(t, err)

	infos, err = rs.ListIngress(ctx, "room")
	require.NoError(t, err)
	require.Equal(t, 0, len(infos))

	info.State.StartedAt = 1
	err = rs.UpdateIngressState(ctx, info.IngressId, info.State)
	require.Equal(t, ingress.ErrIngressOutOfDate, err)

	info.State.StartedAt = 3
	err = rs.UpdateIngressState(ctx, info.IngressId, info.State)
	require.NoError(t, err)

	infos, err = rs.ListIngress(ctx, "")
	require.NoError(t, err)
	require.Equal(t, 1, len(infos))
	require.Equal(t, "", infos[0].RoomName)
}

func TestAgentStore(t *testing.T) {
	ctx := context.Background()
	rs := redisStore(t)

	ad := &hublive.AgentDispatch{
		Id:        "dispatch_id",
		AgentName: "agent_name",
		Metadata:  "metadata",
		Room:      "room_name",
		State: &hublive.AgentDispatchState{
			CreatedAt: 1,
			DeletedAt: 2,
			Jobs: []*hublive.Job{
				&hublive.Job{
					Id:         "job_id",
					DispatchId: "dispatch_id",
					Type:       hublive.JobType_JT_PUBLISHER,
					Room: &hublive.Room{
						Name: "room_name",
					},
					Participant: &hublive.ParticipantInfo{
						Identity: "identity",
						Name:     "name",
					},
					Namespace: "ns",
					Metadata:  "metadata",
					AgentName: "agent_name",
					State: &hublive.JobState{
						Status:    hublive.JobStatus_JS_RUNNING,
						StartedAt: 3,
						EndedAt:   4,
						Error:     "error",
					},
				},
			},
		},
	}

	err := rs.StoreAgentDispatch(ctx, ad)
	require.NoError(t, err)

	rd, err := rs.ListAgentDispatches(ctx, "not_a_room")
	require.NoError(t, err)
	require.Equal(t, 0, len(rd))

	rd, err = rs.ListAgentDispatches(ctx, "room_name")
	require.NoError(t, err)
	require.Equal(t, 1, len(rd))

	expected := utils.CloneProto(ad)
	expected.State.Jobs = nil
	require.True(t, proto.Equal(expected, rd[0]))

	err = rs.StoreAgentJob(ctx, ad.State.Jobs[0])
	require.NoError(t, err)

	rd, err = rs.ListAgentDispatches(ctx, "room_name")
	require.NoError(t, err)
	require.Equal(t, 1, len(rd))

	expected = utils.CloneProto(ad)
	expected.State.Jobs[0].Room = nil
	expected.State.Jobs[0].Participant = &hublive.ParticipantInfo{
		Identity: "identity",
	}
	require.True(t, proto.Equal(expected, rd[0]))

	err = rs.DeleteAgentJob(ctx, ad.State.Jobs[0])
	require.NoError(t, err)

	rd, err = rs.ListAgentDispatches(ctx, "room_name")
	require.NoError(t, err)
	require.Equal(t, 1, len(rd))

	expected = utils.CloneProto(ad)
	expected.State.Jobs = nil
	require.True(t, proto.Equal(expected, rd[0]))

	err = rs.DeleteAgentDispatch(ctx, ad)
	require.NoError(t, err)

	rd, err = rs.ListAgentDispatches(ctx, "room_name")
	require.NoError(t, err)
	require.Equal(t, 0, len(rd))
}

func compareIngressInfo(t *testing.T, expected, v *hublive.IngressInfo) {
	require.Equal(t, expected.IngressId, v.IngressId)
	require.Equal(t, expected.StreamKey, v.StreamKey)
	require.Equal(t, expected.RoomName, v.RoomName)
}
