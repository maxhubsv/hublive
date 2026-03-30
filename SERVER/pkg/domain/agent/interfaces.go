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

// Package agent defines domain interfaces for agent dispatch and management.
// This package breaks the rtc -> service -> rtc import cycle by providing
// the AgentStore interface in a neutral location.
package agent

import (
	"context"

	"__GITHUB_HUBLIVE__protocol/hublive"
)

// AgentStore persists agent dispatch and job state.
// This interface was previously duplicated in both service/interfaces.go
// and rtc/room.go to avoid circular imports. Now defined once here.
type AgentStore interface {
	StoreAgentDispatch(ctx context.Context, dispatch *hublive.AgentDispatch) error
	DeleteAgentDispatch(ctx context.Context, dispatch *hublive.AgentDispatch) error
	ListAgentDispatches(ctx context.Context, roomName hublive.RoomName) ([]*hublive.AgentDispatch, error)

	StoreAgentJob(ctx context.Context, job *hublive.Job) error
	DeleteAgentJob(ctx context.Context, job *hublive.Job) error
}

// AgentDispatcher decides how to dispatch jobs to workers.
// Default implementation preserves current behavior.
// Implement custom dispatchers for: priority queues, skill-based routing,
// multi-tenant isolation.
type AgentDispatcher interface {
	Dispatch(ctx context.Context, job *AgentJob) error
	Cancel(ctx context.Context, dispatchID string) error
	ListActive(ctx context.Context, roomName hublive.RoomName) ([]*hublive.AgentDispatch, error)
}

// AgentJob represents a job to be dispatched to an agent worker.
type AgentJob struct {
	DispatchID string
	JobType    hublive.JobType
	Room       *hublive.Room
	Participant *hublive.ParticipantInfo
	Metadata   string
	Namespace  string
}

// WorkerRegistry tracks available agent workers.
// Extend to support custom worker types, health checking, or external worker pools.
type WorkerRegistry interface {
	Register(worker *WorkerInfo) error
	Deregister(workerID string) error
	FindWorkers(jobType hublive.JobType, namespace string) ([]*WorkerInfo, error)
}

// WorkerInfo describes a registered agent worker.
type WorkerInfo struct {
	ID        string
	JobType   hublive.JobType
	Namespace string
	Load      float32
}
