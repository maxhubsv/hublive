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

package local

import (
	"context"
	"sync"
	"time"

	"github.com/thoas/go-funk"

	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/utils"

	"github.com/maxhubsv/hublive-server/pkg/store"
)

var _ store.OSSServiceStore = (*LocalStore)(nil)

// encapsulates CRUD operations for room settings
type LocalStore struct {
	// map of roomName => room
	rooms        map[hublive.RoomName]*hublive.Room
	roomInternal map[hublive.RoomName]*hublive.RoomInternal
	// map of roomName => { identity: participant }
	participants map[hublive.RoomName]map[hublive.ParticipantIdentity]*hublive.ParticipantInfo

	agentDispatches map[hublive.RoomName]map[string]*hublive.AgentDispatch
	agentJobs       map[hublive.RoomName]map[string]*hublive.Job

	lock       sync.RWMutex
	globalLock sync.Mutex
}

func NewLocalStore() *LocalStore {
	return &LocalStore{
		rooms:           make(map[hublive.RoomName]*hublive.Room),
		roomInternal:    make(map[hublive.RoomName]*hublive.RoomInternal),
		participants:    make(map[hublive.RoomName]map[hublive.ParticipantIdentity]*hublive.ParticipantInfo),
		agentDispatches: make(map[hublive.RoomName]map[string]*hublive.AgentDispatch),
		agentJobs:       make(map[hublive.RoomName]map[string]*hublive.Job),
		lock:            sync.RWMutex{},
	}
}

func (s *LocalStore) StoreRoom(_ context.Context, room *hublive.Room, internal *hublive.RoomInternal) error {
	if room.CreationTime == 0 {
		now := time.Now()
		room.CreationTime = now.Unix()
		room.CreationTimeMs = now.UnixMilli()
	}
	roomName := hublive.RoomName(room.Name)

	s.lock.Lock()
	s.rooms[roomName] = room
	s.roomInternal[roomName] = internal
	s.lock.Unlock()

	return nil
}

func (s *LocalStore) LoadRoom(_ context.Context, roomName hublive.RoomName, includeInternal bool) (*hublive.Room, *hublive.RoomInternal, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	room := s.rooms[roomName]
	if room == nil {
		return nil, nil, store.ErrRoomNotFound
	}

	var internal *hublive.RoomInternal
	if includeInternal {
		internal = s.roomInternal[roomName]
	}

	return room, internal, nil
}

func (s *LocalStore) RoomExists(ctx context.Context, roomName hublive.RoomName) (bool, error) {
	_, _, err := s.LoadRoom(ctx, roomName, false)
	if err == store.ErrRoomNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (s *LocalStore) ListRooms(_ context.Context, roomNames []hublive.RoomName) ([]*hublive.Room, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	rooms := make([]*hublive.Room, 0, len(s.rooms))
	for _, r := range s.rooms {
		if roomNames == nil || funk.Contains(roomNames, hublive.RoomName(r.Name)) {
			rooms = append(rooms, r)
		}
	}
	return rooms, nil
}

func (s *LocalStore) DeleteRoom(ctx context.Context, roomName hublive.RoomName) error {
	room, _, err := s.LoadRoom(ctx, roomName, false)
	if err == store.ErrRoomNotFound {
		return nil
	} else if err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.participants, hublive.RoomName(room.Name))
	delete(s.rooms, hublive.RoomName(room.Name))
	delete(s.roomInternal, hublive.RoomName(room.Name))
	delete(s.agentDispatches, hublive.RoomName(room.Name))
	delete(s.agentJobs, hublive.RoomName(room.Name))
	return nil
}

func (s *LocalStore) LockRoom(_ context.Context, _ hublive.RoomName, _ time.Duration) (string, error) {
	// local rooms lock & unlock globally
	s.globalLock.Lock()
	return "", nil
}

func (s *LocalStore) UnlockRoom(_ context.Context, _ hublive.RoomName, _ string) error {
	s.globalLock.Unlock()
	return nil
}

func (s *LocalStore) StoreParticipant(_ context.Context, roomName hublive.RoomName, participant *hublive.ParticipantInfo) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	roomParticipants := s.participants[roomName]
	if roomParticipants == nil {
		roomParticipants = make(map[hublive.ParticipantIdentity]*hublive.ParticipantInfo)
		s.participants[roomName] = roomParticipants
	}
	roomParticipants[hublive.ParticipantIdentity(participant.Identity)] = participant
	return nil
}

func (s *LocalStore) LoadParticipant(_ context.Context, roomName hublive.RoomName, identity hublive.ParticipantIdentity) (*hublive.ParticipantInfo, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	roomParticipants := s.participants[roomName]
	if roomParticipants == nil {
		return nil, store.ErrParticipantNotFound
	}
	participant := roomParticipants[identity]
	if participant == nil {
		return nil, store.ErrParticipantNotFound
	}
	return participant, nil
}

func (s *LocalStore) HasParticipant(ctx context.Context, roomName hublive.RoomName, identity hublive.ParticipantIdentity) (bool, error) {
	p, err := s.LoadParticipant(ctx, roomName, identity)
	return p != nil, utils.ScreenError(err, store.ErrParticipantNotFound)
}

func (s *LocalStore) ListParticipants(_ context.Context, roomName hublive.RoomName) ([]*hublive.ParticipantInfo, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	roomParticipants := s.participants[roomName]
	if roomParticipants == nil {
		// empty array
		return nil, nil
	}

	items := make([]*hublive.ParticipantInfo, 0, len(roomParticipants))
	for _, p := range roomParticipants {
		items = append(items, p)
	}

	return items, nil
}

func (s *LocalStore) DeleteParticipant(_ context.Context, roomName hublive.RoomName, identity hublive.ParticipantIdentity) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	roomParticipants := s.participants[roomName]
	if roomParticipants != nil {
		delete(roomParticipants, identity)
	}
	return nil
}

func (s *LocalStore) StoreAgentDispatch(ctx context.Context, dispatch *hublive.AgentDispatch) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	clone := utils.CloneProto(dispatch)
	if clone.State != nil {
		clone.State.Jobs = nil
	}

	roomDispatches := s.agentDispatches[hublive.RoomName(dispatch.Room)]
	if roomDispatches == nil {
		roomDispatches = make(map[string]*hublive.AgentDispatch)
		s.agentDispatches[hublive.RoomName(dispatch.Room)] = roomDispatches
	}

	roomDispatches[clone.Id] = clone
	return nil
}

func (s *LocalStore) DeleteAgentDispatch(ctx context.Context, dispatch *hublive.AgentDispatch) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	roomDispatches := s.agentDispatches[hublive.RoomName(dispatch.Room)]
	if roomDispatches != nil {
		delete(roomDispatches, dispatch.Id)
	}

	return nil
}

func (s *LocalStore) ListAgentDispatches(ctx context.Context, roomName hublive.RoomName) ([]*hublive.AgentDispatch, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	agentDispatches := s.agentDispatches[roomName]
	if agentDispatches == nil {
		return nil, nil
	}
	agentJobs := s.agentJobs[roomName]

	var js []*hublive.Job
	for _, j := range agentJobs {
		js = append(js, utils.CloneProto(j))
	}
	var ds []*hublive.AgentDispatch

	m := make(map[string]*hublive.AgentDispatch)
	for _, d := range agentDispatches {
		clone := utils.CloneProto(d)
		m[d.Id] = clone
		ds = append(ds, clone)
	}

	for _, j := range js {
		d := m[j.DispatchId]
		if d != nil {
			d.State.Jobs = append(d.State.Jobs, utils.CloneProto(j))
		}
	}

	return ds, nil
}

func (s *LocalStore) StoreAgentJob(ctx context.Context, job *hublive.Job) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	clone := utils.CloneProto(job)
	clone.Room = nil
	if clone.Participant != nil {
		clone.Participant = &hublive.ParticipantInfo{
			Identity: clone.Participant.Identity,
		}
	}

	roomJobs := s.agentJobs[hublive.RoomName(job.Room.Name)]
	if roomJobs == nil {
		roomJobs = make(map[string]*hublive.Job)
		s.agentJobs[hublive.RoomName(job.Room.Name)] = roomJobs
	}
	roomJobs[clone.Id] = clone

	return nil
}

func (s *LocalStore) DeleteAgentJob(ctx context.Context, job *hublive.Job) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	roomJobs := s.agentJobs[hublive.RoomName(job.Room.Name)]
	if roomJobs != nil {
		delete(roomJobs, job.Id)
	}

	return nil
}
