// Copyright 2024 HubLive, Inc.
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

// Package room defines domain interfaces for room management.
// Concrete implementation is in pkg/rtc/ (Room struct).
// These interfaces enable extension and testing without
// importing the full rtc/ dependency tree.
package room

import (
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
)

// Room is the domain interface for a room.
// Canonical definition in rtc/types.Room.
// Re-exported here for domain-layer consumers.
type Room = types.Room

// SubscriptionPolicy determines which tracks a participant auto-subscribes to.
// Default behavior: subscribe to all tracks (current HubLive behavior).
// Implement custom policies for role-based, permission-based, or
// bandwidth-aware subscription control.
type SubscriptionPolicy interface {
	// ShouldSubscribe returns true if the subscriber should receive this track.
	ShouldSubscribe(subscriber types.LocalParticipant, track types.MediaTrack) bool
}

// DefaultSubscriptionPolicy subscribes to all tracks (current HubLive behavior).
type DefaultSubscriptionPolicy struct{}

func (d DefaultSubscriptionPolicy) ShouldSubscribe(subscriber types.LocalParticipant, track types.MediaTrack) bool {
	return true
}