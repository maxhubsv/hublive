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

// Transition file for signal extraction (Phase 2B+3).
// SessionHandler interface and SignalServer are now in pkg/transport/.
// This file keeps NewDefaultSignalServer (depends on RoomManager)
// and type aliases for backward compatibility.

package service

import (
	"context"

	"github.com/maxhubsv/hublive-server/pkg/config"
	"github.com/maxhubsv/hublive-server/pkg/routing"
	"github.com/maxhubsv/hublive-server/pkg/telemetry/prometheus"
	"github.com/maxhubsv/hublive-server/pkg/transport"
	"github.com/maxhubsv/hublive-server/pkg/utils"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__psrpc"
)

// Type aliases for backward compatibility
type SessionHandler = transport.SessionHandler
type SignalServer = transport.SignalServer

// Deprecated: Use transport.NewSignalServer
var NewSignalServer = transport.NewSignalServer

func NewDefaultSignalServer(
	currentNode routing.LocalNode,
	bus psrpc.MessageBus,
	config config.SignalRelayConfig,
	router routing.Router,
	roomManager *RoomManager,
) (r *SignalServer, err error) {
	return transport.NewSignalServer(currentNode.NodeID(), currentNode.Region(), bus, config, &defaultSessionHandler{currentNode, router, roomManager})
}

type defaultSessionHandler struct {
	currentNode routing.LocalNode
	router      routing.Router
	roomManager *RoomManager
}

func (s *defaultSessionHandler) Logger(ctx context.Context) logger.Logger {
	return utils.GetLogger(ctx)
}

func (s *defaultSessionHandler) HandleSession(
	ctx context.Context,
	pi routing.ParticipantInit,
	connectionID hublive.ConnectionID,
	requestSource routing.MessageSource,
	responseSink routing.MessageSink,
) error {
	prometheus.IncrementParticipantRtcInit(1)

	rtcNode, err := s.router.GetNodeForRoom(ctx, hublive.RoomName(pi.CreateRoom.Name))
	if err != nil {
		return err
	}

	if hublive.NodeID(rtcNode.Id) != s.currentNode.NodeID() {
		err = routing.ErrIncorrectRTCNode
		logger.Errorw("called participant on incorrect node", err,
			"rtcNode", rtcNode,
		)
		return err
	}

	return s.roomManager.StartSession(ctx, pi, requestSource, responseSink, false)
}
