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

// Package participant defines domain interfaces for participant management.
// Concrete implementation is in pkg/rtc/ (ParticipantImpl struct).
// These interfaces enable extension and testing without
// importing the full rtc/ dependency tree.
package participant

import (
	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
)

// Participant is the domain interface for a room participant.
// Canonical definition in rtc/types.Participant.
type Participant = types.Participant

// LocalParticipant is the domain interface for a locally-managed participant.
// Canonical definition in rtc/types.LocalParticipant.
type LocalParticipant = types.LocalParticipant
