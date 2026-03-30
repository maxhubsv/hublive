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
	"runtime"
	"sync"
	"time"

	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/guid"

	"github.com/maxhubsv/hublive-server/pkg/config"
)

type LocalNode interface {
	Clone() *hublive.Node
	SetNodeID(nodeID hublive.NodeID)
	NodeID() hublive.NodeID
	NodeType() hublive.NodeType
	NodeIP() string
	Region() string
	SetState(state hublive.NodeState)
	SetStats(stats *hublive.NodeStats)
	UpdateNodeStats() bool
	SecondsSinceNodeStatsUpdate() float64
}

type LocalNodeImpl struct {
	lock sync.RWMutex
	node *hublive.Node

	nodeStats *NodeStats
}

func NewLocalNode(conf *config.Config) (*LocalNodeImpl, error) {
	nodeID := guid.New(utils.NodePrefix)
	if conf != nil && conf.RTC.NodeIP.IsEmpty() {
		return nil, ErrIPNotSet
	}
	nowUnix := time.Now().Unix()
	l := &LocalNodeImpl{
		node: &hublive.Node{
			Id:      nodeID,
			NumCpus: uint32(runtime.NumCPU()),
			State:   hublive.NodeState_SERVING,
			Stats: &hublive.NodeStats{
				StartedAt: nowUnix,
				UpdatedAt: nowUnix,
			},
		},
	}
	var nsc *config.NodeStatsConfig
	if conf != nil {
		l.node.Ip = conf.RTC.NodeIP.PrimaryIP()
		l.node.Region = conf.Region

		nsc = &conf.NodeStats
	}
	l.nodeStats = NewNodeStats(nsc, nowUnix)

	return l, nil
}

func NewLocalNodeFromNodeProto(node *hublive.Node) (*LocalNodeImpl, error) {
	return &LocalNodeImpl{node: utils.CloneProto(node)}, nil
}

func (l *LocalNodeImpl) Clone() *hublive.Node {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return utils.CloneProto(l.node)
}

// for testing only
func (l *LocalNodeImpl) SetNodeID(nodeID hublive.NodeID) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.node.Id = string(nodeID)
}

func (l *LocalNodeImpl) NodeID() hublive.NodeID {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return hublive.NodeID(l.node.Id)
}

func (l *LocalNodeImpl) NodeType() hublive.NodeType {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.node.Type
}

func (l *LocalNodeImpl) NodeIP() string {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.node.Ip
}

func (l *LocalNodeImpl) Region() string {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.node.Region
}

func (l *LocalNodeImpl) SetState(state hublive.NodeState) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.node.State = state
}

// for testing only
func (l *LocalNodeImpl) SetStats(stats *hublive.NodeStats) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.node.Stats = utils.CloneProto(stats)
}

func (l *LocalNodeImpl) UpdateNodeStats() bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	stats, err := l.nodeStats.UpdateAndGetNodeStats()
	if err != nil {
		return false
	}

	l.node.Stats = stats
	return true
}

func (l *LocalNodeImpl) SecondsSinceNodeStatsUpdate() float64 {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return time.Since(time.Unix(l.node.Stats.UpdatedAt, 0)).Seconds()
}
