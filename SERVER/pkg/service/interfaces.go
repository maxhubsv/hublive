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

package service

import (
	"context"

	"__GITHUB_HUBLIVE__protocol/hublive"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

// Store interfaces have been extracted to pkg/store/interfaces.go.
// Type aliases in compat_store.go maintain backward compatibility.
// See: ObjectStore, ServiceStore, OSSServiceStore, EgressStore,
// IngressStore, SIPStore, AgentStore in pkg/store/

//counterfeiter:generate . ObjectStore

//counterfeiter:generate . ServiceStore

//counterfeiter:generate . EgressStore

//counterfeiter:generate . IngressStore

//counterfeiter:generate . SIPStore

//counterfeiter:generate . AgentStore

//counterfeiter:generate . RoomAllocator
type RoomAllocator interface {
	AutoCreateEnabled(ctx context.Context) bool
	SelectRoomNode(ctx context.Context, roomName hublive.RoomName, nodeID hublive.NodeID) error
	CreateRoom(ctx context.Context, req *hublive.CreateRoomRequest, isExplicit bool) (*hublive.Room, *hublive.RoomInternal, bool, error)
	ValidateCreateRoom(ctx context.Context, roomName hublive.RoomName) error
}
