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
	"fmt"

	"github.com/maxhubsv/hublive-server/pkg/routing"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/rpc"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/guid"
)

type AgentDispatchService struct {
	agentDispatchClient rpc.TypedAgentDispatchInternalClient
	topicFormatter      rpc.TopicFormatter
	roomAllocator       RoomAllocator
	router              routing.MessageRouter
}

func NewAgentDispatchService(
	agentDispatchClient rpc.TypedAgentDispatchInternalClient,
	topicFormatter rpc.TopicFormatter,
	roomAllocator RoomAllocator,
	router routing.MessageRouter,
) *AgentDispatchService {
	return &AgentDispatchService{
		agentDispatchClient: agentDispatchClient,
		topicFormatter:      topicFormatter,
		roomAllocator:       roomAllocator,
		router:              router,
	}
}

func (ag *AgentDispatchService) CreateDispatch(ctx context.Context, req *hublive.CreateAgentDispatchRequest) (*hublive.AgentDispatch, error) {
	AppendLogFields(ctx, "room", req.Room, "request", logger.Proto(redactCreateAgentDispatchRequest(req)))
	err := EnsureAdminPermission(ctx, hublive.RoomName(req.Room))
	if err != nil {
		return nil, twirpAuthError(err)
	}

	if ag.roomAllocator.AutoCreateEnabled(ctx) {
		err := ag.roomAllocator.SelectRoomNode(ctx, hublive.RoomName(req.Room), "")
		if err != nil {
			return nil, err
		}

		_, err = ag.router.CreateRoom(ctx, &hublive.CreateRoomRequest{Name: req.Room})
		if err != nil {
			return nil, err
		}
	}

	dispatch := &hublive.AgentDispatch{
		Id:            guid.New(guid.AgentDispatchPrefix),
		AgentName:     req.AgentName,
		Room:          req.Room,
		Metadata:      req.Metadata,
		RestartPolicy: req.RestartPolicy,
	}
	return ag.agentDispatchClient.CreateDispatch(ctx, ag.topicFormatter.RoomTopic(ctx, hublive.RoomName(req.Room)), dispatch)
}

func (ag *AgentDispatchService) DeleteDispatch(ctx context.Context, req *hublive.DeleteAgentDispatchRequest) (*hublive.AgentDispatch, error) {
	AppendLogFields(ctx, "room", req.Room, "request", logger.Proto(req))
	err := EnsureAdminPermission(ctx, hublive.RoomName(req.Room))
	if err != nil {
		return nil, twirpAuthError(err)
	}

	return ag.agentDispatchClient.DeleteDispatch(ctx, ag.topicFormatter.RoomTopic(ctx, hublive.RoomName(req.Room)), req)
}

func (ag *AgentDispatchService) ListDispatch(ctx context.Context, req *hublive.ListAgentDispatchRequest) (*hublive.ListAgentDispatchResponse, error) {
	AppendLogFields(ctx, "room", req.Room, "request", logger.Proto(req))
	err := EnsureAdminPermission(ctx, hublive.RoomName(req.Room))
	if err != nil {
		return nil, twirpAuthError(err)
	}

	return ag.agentDispatchClient.ListDispatch(ctx, ag.topicFormatter.RoomTopic(ctx, hublive.RoomName(req.Room)), req)
}

func redactCreateAgentDispatchRequest(req *hublive.CreateAgentDispatchRequest) *hublive.CreateAgentDispatchRequest {
	if req.Metadata == "" {
		return req
	}

	clone := utils.CloneProto(req)

	// replace with size of metadata to provide visibility on request size
	if clone.Metadata != "" {
		clone.Metadata = fmt.Sprintf("__size: %d", len(clone.Metadata))
	}

	return clone
}
