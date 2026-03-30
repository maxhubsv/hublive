/*
 * Copyright 2023 HubLive, Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rtc

import (
	"sync"

	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"github.com/maxhubsv/hublive-server/pkg/utils"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
)

// RoomTrackManager holds tracks that are published to the room
type RoomTrackManager struct {
	logger logger.Logger

	lock            sync.RWMutex
	changedNotifier *utils.ChangeNotifierManager
	removedNotifier *utils.ChangeNotifierManager
	tracks          map[hublive.TrackID][]*TrackInfo
	dataTracks      map[hublive.TrackID][]*DataTrackInfo
}

type TrackInfo struct {
	Track             types.MediaTrack
	PublisherIdentity hublive.ParticipantIdentity
	PublisherID       hublive.ParticipantID
}

type DataTrackInfo struct {
	DataTrack         types.DataTrack
	PublisherIdentity hublive.ParticipantIdentity
	PublisherID       hublive.ParticipantID
}

func NewRoomTrackManager(logger logger.Logger) *RoomTrackManager {
	return &RoomTrackManager{
		logger:          logger,
		tracks:          make(map[hublive.TrackID][]*TrackInfo),
		dataTracks:      make(map[hublive.TrackID][]*DataTrackInfo),
		changedNotifier: utils.NewChangeNotifierManager(),
		removedNotifier: utils.NewChangeNotifierManager(),
	}
}

func (r *RoomTrackManager) AddTrack(track types.MediaTrack, publisherIdentity hublive.ParticipantIdentity, publisherID hublive.ParticipantID) {
	trackID := track.ID()
	r.lock.Lock()
	infos, ok := r.tracks[trackID]
	if ok {
		for _, info := range infos {
			if info.Track == track {
				r.lock.Unlock()
				r.logger.Infow("not adding duplicate track", "trackID", trackID)
				return
			}
		}
	}
	r.tracks[trackID] = append(r.tracks[trackID], &TrackInfo{
		Track:             track,
		PublisherIdentity: publisherIdentity,
		PublisherID:       publisherID,
	})
	r.lock.Unlock()

	r.NotifyTrackChanged(trackID)
}

func (r *RoomTrackManager) RemoveTrack(track types.MediaTrack) {
	trackID := track.ID()
	r.lock.Lock()
	// ensure we are removing the same track as added
	infos, ok := r.tracks[trackID]
	if !ok {
		r.lock.Unlock()
		return
	}

	numRemoved := 0
	idx := 0
	for _, info := range infos {
		if info.Track == track {
			numRemoved++
		} else {
			r.tracks[trackID][idx] = info
			idx++
		}
	}
	for j := idx; j < len(infos); j++ {
		r.tracks[trackID][j] = nil
	}
	r.tracks[trackID] = r.tracks[trackID][:idx]
	if len(r.tracks[trackID]) == 0 {
		delete(r.tracks, trackID)
	}
	r.lock.Unlock()
	if numRemoved == 0 {
		return
	}
	if numRemoved > 1 {
		r.logger.Warnw("removed multiple tracks", nil, "trackID", trackID, "numRemoved", numRemoved)
	}

	n := r.removedNotifier.GetNotifier(string(trackID))
	if n != nil {
		n.NotifyChanged()
	}

	r.changedNotifier.RemoveNotifier(string(trackID), true)
	r.removedNotifier.RemoveNotifier(string(trackID), true)
}

func (r *RoomTrackManager) GetTrackInfo(trackID hublive.TrackID) *TrackInfo {
	r.lock.RLock()
	defer r.lock.RUnlock()

	infos := r.tracks[trackID]
	if len(infos) == 0 {
		return nil
	}

	// earliest added track is used till it is removed
	info := infos[0]

	// when track is about to close, do not resolve
	if info.Track != nil && !info.Track.IsOpen() {
		return nil
	}
	return info
}

func (r *RoomTrackManager) NotifyTrackChanged(trackID hublive.TrackID) {
	n := r.changedNotifier.GetNotifier(string(trackID))
	if n != nil {
		n.NotifyChanged()
	}
}

// HasObservers lets caller know if the current media track has any observers
// this is used to signal interest in the track. when another MediaTrack with the same
// trackID is being used, track is not considered to be observed.
func (r *RoomTrackManager) HasObservers(track types.MediaTrack) bool {
	n := r.changedNotifier.GetNotifier(string(track.ID()))
	if n == nil || !n.HasObservers() {
		return false
	}

	info := r.GetTrackInfo(track.ID())
	if info == nil || info.Track != track {
		return false
	}
	return true
}

func (r *RoomTrackManager) GetOrCreateTrackChangeNotifier(trackID hublive.TrackID) *utils.ChangeNotifier {
	return r.changedNotifier.GetOrCreateNotifier(string(trackID))
}

func (r *RoomTrackManager) GetOrCreateTrackRemoveNotifier(trackID hublive.TrackID) *utils.ChangeNotifier {
	return r.removedNotifier.GetOrCreateNotifier(string(trackID))
}

func (r *RoomTrackManager) AddDataTrack(dataTrack types.DataTrack, publisherIdentity hublive.ParticipantIdentity, publisherID hublive.ParticipantID) {
	trackID := dataTrack.ID()
	r.lock.Lock()
	infos, ok := r.dataTracks[trackID]
	if ok {
		for _, info := range infos {
			if info.DataTrack == dataTrack {
				r.lock.Unlock()
				r.logger.Infow("not adding duplicate data track", "trackID", trackID)
				return
			}
		}
	}
	r.dataTracks[trackID] = append(r.dataTracks[trackID], &DataTrackInfo{
		DataTrack:         dataTrack,
		PublisherIdentity: publisherIdentity,
		PublisherID:       publisherID,
	})
	r.lock.Unlock()

	r.NotifyTrackChanged(trackID)
}

func (r *RoomTrackManager) RemoveDataTrack(dataTrack types.DataTrack) {
	trackID := dataTrack.ID()
	r.lock.Lock()
	// ensure we are removing the same track as added
	infos, ok := r.dataTracks[trackID]
	if !ok {
		r.lock.Unlock()
		return
	}

	numRemoved := 0
	idx := 0
	for _, info := range infos {
		if info.DataTrack == dataTrack {
			numRemoved++
		} else {
			r.dataTracks[trackID][idx] = info
			idx++
		}
	}
	for j := idx; j < len(infos); j++ {
		r.dataTracks[trackID][j] = nil
	}
	r.dataTracks[trackID] = r.dataTracks[trackID][:idx]
	if len(r.dataTracks[trackID]) == 0 {
		delete(r.dataTracks, trackID)
	}
	r.lock.Unlock()
	if numRemoved == 0 {
		return
	}
	if numRemoved > 1 {
		r.logger.Warnw("removed multiple data tracks", nil, "trackID", trackID, "numRemoved", numRemoved)
	}

	n := r.removedNotifier.GetNotifier(string(trackID))
	if n != nil {
		n.NotifyChanged()
	}

	r.changedNotifier.RemoveNotifier(string(trackID), true)
	r.removedNotifier.RemoveNotifier(string(trackID), true)
}

func (r *RoomTrackManager) GetDataTrackInfo(trackID hublive.TrackID) *DataTrackInfo {
	r.lock.RLock()
	defer r.lock.RUnlock()

	infos := r.dataTracks[trackID]
	if len(infos) == 0 {
		return nil
	}

	// earliest added data track is used till it is removed
	return infos[0]
}

func (r *RoomTrackManager) Report() (int, int) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return len(r.tracks), len(r.dataTracks)
}
