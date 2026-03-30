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
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"go.uber.org/atomic"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"

	"github.com/maxhubsv/hublive-server/pkg/config"
	"github.com/maxhubsv/hublive-server/pkg/utils"
	"__GITHUB_HUBLIVE__protocol/auth"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/rpc"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

// MessageSink is an abstraction for writing protobuf messages and having them read by a MessageSource,
// potentially on a different node via a transport
//
//counterfeiter:generate . MessageSink
type MessageSink interface {
	WriteMessage(msg proto.Message) error
	IsClosed() bool
	Close()
	ConnectionID() hublive.ConnectionID
}

// ----------

type NullMessageSink struct {
	connID   hublive.ConnectionID
	isClosed atomic.Bool
}

func NewNullMessageSink(connID hublive.ConnectionID) *NullMessageSink {
	return &NullMessageSink{
		connID: connID,
	}
}

func (n *NullMessageSink) WriteMessage(_msg proto.Message) error {
	return nil
}

func (n *NullMessageSink) IsClosed() bool {
	return n.isClosed.Load()
}

func (n *NullMessageSink) Close() {
	n.isClosed.Store(true)
}

func (n *NullMessageSink) ConnectionID() hublive.ConnectionID {
	return n.connID
}

// ------------------------------------------------

//counterfeiter:generate . MessageSource
type MessageSource interface {
	// ReadChan exposes a one way channel to make it easier to use with select
	ReadChan() <-chan proto.Message
	IsClosed() bool
	Close()
	ConnectionID() hublive.ConnectionID
}

// ----------

type NullMessageSource struct {
	connID   hublive.ConnectionID
	msgChan  chan proto.Message
	isClosed atomic.Bool
}

func NewNullMessageSource(connID hublive.ConnectionID) *NullMessageSource {
	return &NullMessageSource{
		connID:  connID,
		msgChan: make(chan proto.Message),
	}
}

func (n *NullMessageSource) ReadChan() <-chan proto.Message {
	return n.msgChan
}

func (n *NullMessageSource) IsClosed() bool {
	return n.isClosed.Load()
}

func (n *NullMessageSource) Close() {
	if !n.isClosed.Swap(true) {
		close(n.msgChan)
	}
}

func (n *NullMessageSource) ConnectionID() hublive.ConnectionID {
	return n.connID
}

// ------------------------------------------------

// Router allows multiple nodes to coordinate the participant session
//
//counterfeiter:generate . Router
type Router interface {
	MessageRouter

	RegisterNode() error
	UnregisterNode() error
	RemoveDeadNodes() error

	ListNodes() ([]*hublive.Node, error)

	GetNodeForRoom(ctx context.Context, roomName hublive.RoomName) (*hublive.Node, error)
	SetNodeForRoom(ctx context.Context, roomName hublive.RoomName, nodeId hublive.NodeID) error
	ClearRoomState(ctx context.Context, roomName hublive.RoomName) error

	GetRegion() string

	Start() error
	Drain()
	Stop()
}

type StartParticipantSignalResults struct {
	ConnectionID        hublive.ConnectionID
	RequestSink         MessageSink
	ResponseSource      MessageSource
	NodeID              hublive.NodeID
	NodeSelectionReason string
}

type MessageRouter interface {
	// CreateRoom starts an rtc room
	CreateRoom(ctx context.Context, req *hublive.CreateRoomRequest) (res *hublive.Room, err error)

	// StartParticipantSignal participant signal connection is ready to start
	StartParticipantSignal(
		ctx context.Context,
		roomName hublive.RoomName,
		pi ParticipantInit,
	) (res StartParticipantSignalResults, err error)
}

func CreateRouter(
	rc redis.UniversalClient,
	node LocalNode,
	signalClient SignalClient,
	roomManagerClient RoomManagerClient,
	kps rpc.KeepalivePubSub,
	nodeStatsConfig config.NodeStatsConfig,
) Router {
	lr := NewLocalRouter(node, signalClient, roomManagerClient, nodeStatsConfig)

	if rc != nil {
		return NewRedisRouter(lr, rc, kps)
	}

	// local routing and store
	logger.Infow("using single-node routing")
	return lr
}

// ------------------------------------------------

type ParticipantInit struct {
	Identity                hublive.ParticipantIdentity
	Name                    hublive.ParticipantName
	Reconnect               bool
	ReconnectReason         hublive.ReconnectReason
	AutoSubscribe           bool
	AutoSubscribeDataTrack  *bool
	Client                  *hublive.ClientInfo
	Grants                  *auth.ClaimGrants
	Region                  string
	AdaptiveStream          bool
	ID                      hublive.ParticipantID
	SubscriberAllowPause    *bool
	DisableICELite          bool
	CreateRoom              *hublive.CreateRoomRequest
	AddTrackRequests        []*hublive.AddTrackRequest
	PublisherOffer          *hublive.SessionDescription
	SyncState               *hublive.SyncState
	UseSinglePeerConnection bool
}

func (pi *ParticipantInit) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if pi == nil {
		return nil
	}

	logBoolPtr := func(prop string, val *bool) {
		if val == nil {
			e.AddString(prop, "not-set")
		} else {
			e.AddBool(prop, *val)
		}
	}

	e.AddString("Identity", string(pi.Identity))
	logBoolPtr("Reconnect", &pi.Reconnect)
	e.AddString("ReconnectReason", pi.ReconnectReason.String())
	logBoolPtr("AutoSubscribe", &pi.AutoSubscribe)
	logBoolPtr("AutoSubscribeDataTrack", pi.AutoSubscribeDataTrack)
	e.AddObject("Client", logger.Proto(utils.ClientInfoWithoutAddress(pi.Client)))
	e.AddObject("Grants", pi.Grants)
	e.AddString("Region", pi.Region)
	logBoolPtr("AdaptiveStream", &pi.AdaptiveStream)
	e.AddString("ID", string(pi.ID))
	logBoolPtr("SubscriberAllowPause", pi.SubscriberAllowPause)
	logBoolPtr("DisableICELite", &pi.DisableICELite)
	e.AddObject("CreateRoom", logger.Proto(pi.CreateRoom))
	e.AddArray("AddTrackRequests", logger.ProtoSlice(pi.AddTrackRequests))
	e.AddObject("PublisherOffer", logger.Proto(pi.PublisherOffer))
	e.AddObject("SyncState", logger.Proto(pi.SyncState))
	logBoolPtr("UseSinglePeerConnection", &pi.UseSinglePeerConnection)
	return nil
}

func (pi *ParticipantInit) ToStartSession(roomName hublive.RoomName, connectionID hublive.ConnectionID) (*hublive.StartSession, error) {
	claims, err := json.Marshal(pi.Grants)
	if err != nil {
		return nil, err
	}

	ss := &hublive.StartSession{
		RoomName:                string(roomName),
		Identity:                string(pi.Identity),
		Name:                    string(pi.Name),
		ConnectionId:            string(connectionID),
		Reconnect:               pi.Reconnect,
		ReconnectReason:         pi.ReconnectReason,
		AutoSubscribe:           pi.AutoSubscribe,
		Client:                  pi.Client,
		GrantsJson:              string(claims),
		AdaptiveStream:          pi.AdaptiveStream,
		ParticipantId:           string(pi.ID),
		DisableIceLite:          pi.DisableICELite,
		CreateRoom:              pi.CreateRoom,
		AddTrackRequests:        pi.AddTrackRequests,
		PublisherOffer:          pi.PublisherOffer,
		SyncState:               pi.SyncState,
		UseSinglePeerConnection: pi.UseSinglePeerConnection,
	}
	if pi.AutoSubscribeDataTrack != nil {
		autoSubscribeDataTrack := *pi.AutoSubscribeDataTrack
		ss.AutoSubscribeDataTrack = &autoSubscribeDataTrack
	}
	if pi.SubscriberAllowPause != nil {
		subscriberAllowPause := *pi.SubscriberAllowPause
		ss.SubscriberAllowPause = &subscriberAllowPause
	}

	return ss, nil
}

func ParticipantInitFromStartSession(ss *hublive.StartSession, region string) (*ParticipantInit, error) {
	claims := &auth.ClaimGrants{}
	if err := json.Unmarshal([]byte(ss.GrantsJson), claims); err != nil {
		return nil, err
	}

	pi := &ParticipantInit{
		Identity:                hublive.ParticipantIdentity(ss.Identity),
		Name:                    hublive.ParticipantName(ss.Name),
		Reconnect:               ss.Reconnect,
		ReconnectReason:         ss.ReconnectReason,
		Client:                  ss.Client,
		AutoSubscribe:           ss.AutoSubscribe,
		Grants:                  claims,
		Region:                  region,
		AdaptiveStream:          ss.AdaptiveStream,
		ID:                      hublive.ParticipantID(ss.ParticipantId),
		DisableICELite:          ss.DisableIceLite,
		CreateRoom:              ss.CreateRoom,
		AddTrackRequests:        ss.AddTrackRequests,
		PublisherOffer:          ss.PublisherOffer,
		SyncState:               ss.SyncState,
		UseSinglePeerConnection: ss.UseSinglePeerConnection,
	}
	if ss.AutoSubscribeDataTrack != nil {
		autoSubscribeDataTrack := *ss.AutoSubscribeDataTrack
		pi.AutoSubscribeDataTrack = &autoSubscribeDataTrack
	}
	if ss.SubscriberAllowPause != nil {
		subscriberAllowPause := *ss.SubscriberAllowPause
		pi.SubscriberAllowPause = &subscriberAllowPause
	}

	// TODO: clean up after 1.7 eol
	if pi.CreateRoom == nil {
		pi.CreateRoom = &hublive.CreateRoomRequest{
			Name: ss.RoomName,
		}
	}

	return pi, nil
}
