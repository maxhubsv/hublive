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
	"context"
	"time"

	"__GITHUB_HUBLIVE__protocol/hublive"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

// encapsulates CRUD operations for room settings
//
//counterfeiter:generate . ObjectStore
type ObjectStore interface {
	ServiceStore
	OSSServiceStore

	// enable locking on a specific room to prevent race
	// returns a (lock uuid, error)
	LockRoom(ctx context.Context, roomName hublive.RoomName, duration time.Duration) (string, error)
	UnlockRoom(ctx context.Context, roomName hublive.RoomName, uid string) error

	StoreRoom(ctx context.Context, room *hublive.Room, internal *hublive.RoomInternal) error

	StoreParticipant(ctx context.Context, roomName hublive.RoomName, participant *hublive.ParticipantInfo) error
	DeleteParticipant(ctx context.Context, roomName hublive.RoomName, identity hublive.ParticipantIdentity) error
}

//counterfeiter:generate . ServiceStore
type ServiceStore interface {
	LoadRoom(ctx context.Context, roomName hublive.RoomName, includeInternal bool) (*hublive.Room, *hublive.RoomInternal, error)
	RoomExists(ctx context.Context, roomName hublive.RoomName) (bool, error)

	// ListRooms returns currently active rooms. if names is not nil, it'll filter and return
	// only rooms that match
	ListRooms(ctx context.Context, roomNames []hublive.RoomName) ([]*hublive.Room, error)
}

type OSSServiceStore interface {
	DeleteRoom(ctx context.Context, roomName hublive.RoomName) error
	HasParticipant(context.Context, hublive.RoomName, hublive.ParticipantIdentity) (bool, error)
	LoadParticipant(ctx context.Context, roomName hublive.RoomName, identity hublive.ParticipantIdentity) (*hublive.ParticipantInfo, error)
	ListParticipants(ctx context.Context, roomName hublive.RoomName) ([]*hublive.ParticipantInfo, error)
}

//counterfeiter:generate . EgressStore
type EgressStore interface {
	StoreEgress(ctx context.Context, info *hublive.EgressInfo) error
	LoadEgress(ctx context.Context, egressID string) (*hublive.EgressInfo, error)
	ListEgress(ctx context.Context, roomName hublive.RoomName, active bool) ([]*hublive.EgressInfo, error)
	UpdateEgress(ctx context.Context, info *hublive.EgressInfo) error
}

//counterfeiter:generate . IngressStore
type IngressStore interface {
	StoreIngress(ctx context.Context, info *hublive.IngressInfo) error
	LoadIngress(ctx context.Context, ingressID string) (*hublive.IngressInfo, error)
	LoadIngressFromStreamKey(ctx context.Context, streamKey string) (*hublive.IngressInfo, error)
	ListIngress(ctx context.Context, roomName hublive.RoomName) ([]*hublive.IngressInfo, error)
	UpdateIngress(ctx context.Context, info *hublive.IngressInfo) error
	UpdateIngressState(ctx context.Context, ingressId string, state *hublive.IngressState) error
	DeleteIngress(ctx context.Context, info *hublive.IngressInfo) error
}

//counterfeiter:generate . SIPStore
type SIPStore interface {
	StoreSIPTrunk(ctx context.Context, info *hublive.SIPTrunkInfo) error
	StoreSIPInboundTrunk(ctx context.Context, info *hublive.SIPInboundTrunkInfo) error
	StoreSIPOutboundTrunk(ctx context.Context, info *hublive.SIPOutboundTrunkInfo) error
	LoadSIPTrunk(ctx context.Context, sipTrunkID string) (*hublive.SIPTrunkInfo, error)
	LoadSIPInboundTrunk(ctx context.Context, sipTrunkID string) (*hublive.SIPInboundTrunkInfo, error)
	LoadSIPOutboundTrunk(ctx context.Context, sipTrunkID string) (*hublive.SIPOutboundTrunkInfo, error)
	ListSIPTrunk(ctx context.Context, opts *hublive.ListSIPTrunkRequest) (*hublive.ListSIPTrunkResponse, error)
	ListSIPInboundTrunk(ctx context.Context, opts *hublive.ListSIPInboundTrunkRequest) (*hublive.ListSIPInboundTrunkResponse, error)
	ListSIPOutboundTrunk(ctx context.Context, opts *hublive.ListSIPOutboundTrunkRequest) (*hublive.ListSIPOutboundTrunkResponse, error)
	DeleteSIPTrunk(ctx context.Context, sipTrunkID string) error

	StoreSIPDispatchRule(ctx context.Context, info *hublive.SIPDispatchRuleInfo) error
	LoadSIPDispatchRule(ctx context.Context, sipDispatchRuleID string) (*hublive.SIPDispatchRuleInfo, error)
	ListSIPDispatchRule(ctx context.Context, opts *hublive.ListSIPDispatchRuleRequest) (*hublive.ListSIPDispatchRuleResponse, error)
	DeleteSIPDispatchRule(ctx context.Context, sipDispatchRuleID string) error
}

//counterfeiter:generate . AgentStore
type AgentStore interface {
	StoreAgentDispatch(ctx context.Context, dispatch *hublive.AgentDispatch) error
	DeleteAgentDispatch(ctx context.Context, dispatch *hublive.AgentDispatch) error
	ListAgentDispatches(ctx context.Context, roomName hublive.RoomName) ([]*hublive.AgentDispatch, error)

	StoreAgentJob(ctx context.Context, job *hublive.Job) error
	DeleteAgentJob(ctx context.Context, job *hublive.Job) error
}
