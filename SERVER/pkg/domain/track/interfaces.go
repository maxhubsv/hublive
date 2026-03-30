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

// Package track defines domain interfaces for media and data tracks.
// Concrete implementations are in pkg/rtc/ (MediaTrack, SubscribedTrack, etc.).
// These interfaces enable extension and testing without
// importing the full rtc/ dependency tree.
package track

import (
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
)

// MediaTrack is the domain interface for a published media track.
// Canonical definition in rtc/types.MediaTrack.
type MediaTrack = types.MediaTrack

// LocalMediaTrack is the domain interface for a locally-managed media track.
// Canonical definition in rtc/types.LocalMediaTrack.
type LocalMediaTrack = types.LocalMediaTrack

// SubscribedTrack is the domain interface for a subscriber's view of a track.
// Canonical definition in rtc/types.SubscribedTrack.
type SubscribedTrack = types.SubscribedTrack

// DataTrack is the domain interface for a data channel track.
// Canonical definition in rtc/types.DataTrack.
type DataTrack = types.DataTrack

// DataDownTrack is the domain interface for a subscriber's data track.
// Canonical definition in rtc/types.DataDownTrack.
type DataDownTrack = types.DataDownTrack
