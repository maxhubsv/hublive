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

package store

import (
	"__GITHUB_HUBLIVE__psrpc"
)

// Store-related sentinel errors.
// These were originally in pkg/service/errors.go and are aliased there
// for backward compatibility.
var (
	ErrEgressNotFound          = psrpc.NewErrorf(psrpc.NotFound, "egress does not exist")
	ErrIngressNotFound         = psrpc.NewErrorf(psrpc.NotFound, "ingress does not exist")
	ErrParticipantNotFound     = psrpc.NewErrorf(psrpc.NotFound, "participant does not exist")
	ErrRoomNotFound            = psrpc.NewErrorf(psrpc.NotFound, "requested room does not exist")
	ErrRoomLockFailed          = psrpc.NewErrorf(psrpc.Internal, "could not lock room")
	ErrRoomUnlockFailed        = psrpc.NewErrorf(psrpc.Internal, "could not unlock room, lock token does not match")
	ErrSIPTrunkNotFound        = psrpc.NewErrorf(psrpc.NotFound, "requested sip trunk does not exist")
	ErrSIPDispatchRuleNotFound = psrpc.NewErrorf(psrpc.NotFound, "requested sip dispatch rule does not exist")
)
