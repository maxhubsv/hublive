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

package routing

import (
	"context"
	"time"

	"go.uber.org/atomic"

	"github.com/maxhubsv/hublive-server/pkg/config"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
)

var _ Router = (*LocalRouter)(nil)

// a router of messages on the same node, basic implementation for local testing
type LocalRouter struct {
	currentNode       LocalNode
	signalClient      SignalClient
	roomManagerClient RoomManagerClient
	nodeStatsConfig   config.NodeStatsConfig

	// channels for each participant
	requestChannels  map[string]*MessageChannel
	responseChannels map[string]*MessageChannel
	isStarted        atomic.Bool
}

func NewLocalRouter(
	currentNode LocalNode,
	signalClient SignalClient,
	roomManagerClient RoomManagerClient,
	nodeStatsConfig config.NodeStatsConfig,
) *LocalRouter {
	return &LocalRouter{
		currentNode:       currentNode,
		signalClient:      signalClient,
		roomManagerClient: roomManagerClient,
		nodeStatsConfig:   nodeStatsConfig,
		requestChannels:   make(map[string]*MessageChannel),
		responseChannels:  make(map[string]*MessageChannel),
	}
}

func (r *LocalRouter) GetNodeForRoom(_ context.Context, _ hublive.RoomName) (*hublive.Node, error) {
	return r.currentNode.Clone(), nil
}

func (r *LocalRouter) SetNodeForRoom(_ context.Context, _ hublive.RoomName, _ hublive.NodeID) error {
	return nil
}

func (r *LocalRouter) ClearRoomState(_ context.Context, _ hublive.RoomName) error {
	return nil
}

func (r *LocalRouter) RegisterNode() error {
	return nil
}

func (r *LocalRouter) UnregisterNode() error {
	return nil
}

func (r *LocalRouter) RemoveDeadNodes() error {
	return nil
}

func (r *LocalRouter) GetNode(nodeID hublive.NodeID) (*hublive.Node, error) {
	if nodeID == r.currentNode.NodeID() {
		return r.currentNode.Clone(), nil
	}
	return nil, ErrNotFound
}

func (r *LocalRouter) ListNodes() ([]*hublive.Node, error) {
	return []*hublive.Node{
		r.currentNode.Clone(),
	}, nil
}

func (r *LocalRouter) CreateRoom(ctx context.Context, req *hublive.CreateRoomRequest) (res *hublive.Room, err error) {
	return r.CreateRoomWithNodeID(ctx, req, r.currentNode.NodeID())
}

func (r *LocalRouter) CreateRoomWithNodeID(ctx context.Context, req *hublive.CreateRoomRequest, nodeID hublive.NodeID) (res *hublive.Room, err error) {
	return r.roomManagerClient.CreateRoom(ctx, nodeID, req)
}

func (r *LocalRouter) StartParticipantSignal(ctx context.Context, roomName hublive.RoomName, pi ParticipantInit) (res StartParticipantSignalResults, err error) {
	return r.StartParticipantSignalWithNodeID(ctx, roomName, pi, r.currentNode.NodeID())
}

func (r *LocalRouter) StartParticipantSignalWithNodeID(ctx context.Context, roomName hublive.RoomName, pi ParticipantInit, nodeID hublive.NodeID) (res StartParticipantSignalResults, err error) {
	connectionID, reqSink, resSource, err := r.signalClient.StartParticipantSignal(ctx, roomName, pi, nodeID)
	if err != nil {
		logger.Errorw(
			"could not handle new participant", err,
			"room", roomName,
			"participant", pi.Identity,
			"connID", connectionID,
		)
	} else {
		return StartParticipantSignalResults{
			ConnectionID:   connectionID,
			RequestSink:    reqSink,
			ResponseSource: resSource,
			NodeID:         nodeID,
		}, nil
	}
	return
}

func (r *LocalRouter) Start() error {
	if r.isStarted.Swap(true) {
		return nil
	}
	go r.statsWorker()
	// go r.memStatsWorker()
	return nil
}

func (r *LocalRouter) Drain() {
	r.currentNode.SetState(hublive.NodeState_SHUTTING_DOWN)
}

func (r *LocalRouter) Stop() {}

func (r *LocalRouter) GetRegion() string {
	return r.currentNode.Region()
}

func (r *LocalRouter) statsWorker() {
	for {
		if !r.isStarted.Load() {
			return
		}
		<-time.After(r.nodeStatsConfig.StatsUpdateInterval)
		r.currentNode.UpdateNodeStats()
	}
}

/*
	func (r *LocalRouter) memStatsWorker() {
		ticker := time.NewTicker(time.Second * 30)
		defer ticker.Stop()

		for {
			<-ticker.C

			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			logger.Infow("memstats",
				"mallocs", m.Mallocs, "frees", m.Frees, "m-f", m.Mallocs-m.Frees,
				"hinuse", m.HeapInuse, "halloc", m.HeapAlloc, "frag", m.HeapInuse-m.HeapAlloc,
			)
		}
	}
*/
