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
	"errors"
	"time"

	"github.com/dennwc/iters"
	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/types/known/emptypb"

	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"__GITHUB_HUBLIVE__protocol/rpc"
	"__GITHUB_HUBLIVE__protocol/sip"
	"__GITHUB_HUBLIVE__protocol/utils"
	"__GITHUB_HUBLIVE__protocol/utils/guid"
	"__GITHUB_HUBLIVE__psrpc"

	"github.com/maxhubsv/hublive-server/pkg/config"
	"github.com/maxhubsv/hublive-server/pkg/telemetry"
)

type SIPService struct {
	conf        *config.SIPConfig
	nodeID      hublive.NodeID
	bus         psrpc.MessageBus
	psrpcClient rpc.SIPClient
	store       SIPStore
	roomService hublive.RoomService
}

func NewSIPService(
	conf *config.SIPConfig,
	nodeID hublive.NodeID,
	bus psrpc.MessageBus,
	psrpcClient rpc.SIPClient,
	store SIPStore,
	rs hublive.RoomService,
	ts telemetry.TelemetryService,
) *SIPService {
	return &SIPService{
		conf:        conf,
		nodeID:      nodeID,
		bus:         bus,
		psrpcClient: psrpcClient,
		store:       store,
		roomService: rs,
	}
}

func (s *SIPService) CreateSIPTrunk(ctx context.Context, req *hublive.CreateSIPTrunkRequest) (*hublive.SIPTrunkInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if len(req.InboundNumbersRegex) != 0 {
		return nil, twirp.NewError(twirp.InvalidArgument, "Trunks with InboundNumbersRegex are deprecated. Use InboundNumbers instead.")
	}

	// Keep ID empty, so that validation can print "<new>" instead of a non-existent ID in the error.
	info := &hublive.SIPTrunkInfo{
		InboundAddresses: req.InboundAddresses,
		OutboundAddress:  req.OutboundAddress,
		OutboundNumber:   req.OutboundNumber,
		InboundNumbers:   req.InboundNumbers,
		InboundUsername:  req.InboundUsername,
		InboundPassword:  req.InboundPassword,
		OutboundUsername: req.OutboundUsername,
		OutboundPassword: req.OutboundPassword,
		Name:             req.Name,
		Metadata:         req.Metadata,
	}
	if err := info.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	// Validate all trunks including the new one first.
	it, err := ListSIPInboundTrunk(ctx, s.store, &hublive.ListSIPInboundTrunkRequest{}, info.AsInbound())
	if err != nil {
		return nil, err
	}
	defer it.Close()
	if err = sip.ValidateTrunksIter(it); err != nil {
		return nil, err
	}

	// Now we can generate ID and store.
	info.SipTrunkId = guid.New(utils.SIPTrunkPrefix)
	if err := s.store.StoreSIPTrunk(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *SIPService) CreateSIPInboundTrunk(ctx context.Context, req *hublive.CreateSIPInboundTrunkRequest) (*hublive.SIPInboundTrunkInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	info := req.Trunk
	if info.SipTrunkId != "" {
		return nil, twirp.NewError(twirp.InvalidArgument, "trunk ID must be empty")
	}
	AppendLogFields(ctx, "trunk", logger.Proto(info))

	// Keep ID empty still, so that validation can print "<new>" instead of a non-existent ID in the error.

	// Validate all trunks including the new one first.
	it, err := ListSIPInboundTrunk(ctx, s.store, &hublive.ListSIPInboundTrunkRequest{
		Numbers: req.GetTrunk().GetNumbers(),
	}, info)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	if err = sip.ValidateTrunksIter(it); err != nil {
		return nil, err
	}

	// Now we can generate ID and store.
	info.SipTrunkId = guid.New(utils.SIPTrunkPrefix)
	if err := s.store.StoreSIPInboundTrunk(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *SIPService) CreateSIPOutboundTrunk(ctx context.Context, req *hublive.CreateSIPOutboundTrunkRequest) (*hublive.SIPOutboundTrunkInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	info := req.Trunk
	if info.SipTrunkId != "" {
		return nil, twirp.NewError(twirp.InvalidArgument, "trunk ID must be empty")
	}
	AppendLogFields(ctx, "trunk", logger.Proto(info))

	// No additional validation needed for outbound.
	info.SipTrunkId = guid.New(utils.SIPTrunkPrefix)
	if err := s.store.StoreSIPOutboundTrunk(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *SIPService) UpdateSIPInboundTrunk(ctx context.Context, req *hublive.UpdateSIPInboundTrunkRequest) (*hublive.SIPInboundTrunkInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	AppendLogFields(ctx,
		"request", logger.Proto(req),
		"trunkID", req.SipTrunkId,
	)

	// Validate all trunks including the new one first.
	info, err := s.store.LoadSIPInboundTrunk(ctx, req.SipTrunkId)
	if err != nil {
		if errors.Is(err, ErrSIPTrunkNotFound) {
			return nil, twirp.NewError(twirp.NotFound, err.Error())
		}
		return nil, err
	}
	switch a := req.Action.(type) {
	default:
		return nil, errors.New("missing or unsupported action")
	case hublive.UpdateSIPInboundTrunkRequestAction:
		info, err = a.Apply(info)
		if err != nil {
			return nil, err
		}
	}

	it, err := ListSIPInboundTrunk(ctx, s.store, &hublive.ListSIPInboundTrunkRequest{
		Numbers: info.Numbers,
	})
	if err != nil {
		return nil, err
	}
	defer it.Close()
	if err = sip.ValidateTrunksIter(it, sip.WithTrunkReplace(func(t *hublive.SIPInboundTrunkInfo) *hublive.SIPInboundTrunkInfo {
		if req.SipTrunkId == t.SipTrunkId {
			return info // updated one
		}
		return t
	})); err != nil {
		return nil, err
	}
	if err := s.store.StoreSIPInboundTrunk(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *SIPService) UpdateSIPOutboundTrunk(ctx context.Context, req *hublive.UpdateSIPOutboundTrunkRequest) (*hublive.SIPOutboundTrunkInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	AppendLogFields(ctx,
		"request", logger.Proto(req),
		"trunkID", req.SipTrunkId,
	)

	info, err := s.store.LoadSIPOutboundTrunk(ctx, req.SipTrunkId)
	if err != nil {
		if errors.Is(err, ErrSIPTrunkNotFound) {
			return nil, twirp.NewError(twirp.NotFound, err.Error())
		}
		return nil, err
	}
	switch a := req.Action.(type) {
	default:
		return nil, errors.New("missing or unsupported action")
	case hublive.UpdateSIPOutboundTrunkRequestAction:
		info, err = a.Apply(info)
		if err != nil {
			return nil, err
		}
	}
	// No additional validation needed for outbound.
	if err := s.store.StoreSIPOutboundTrunk(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *SIPService) GetSIPInboundTrunk(ctx context.Context, req *hublive.GetSIPInboundTrunkRequest) (*hublive.GetSIPInboundTrunkResponse, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if req.SipTrunkId == "" {
		return nil, twirp.NewError(twirp.InvalidArgument, "trunk ID is required")
	}
	AppendLogFields(ctx, "trunkID", req.SipTrunkId)

	trunk, err := s.store.LoadSIPInboundTrunk(ctx, req.SipTrunkId)
	if err != nil {
		if errors.Is(err, ErrSIPTrunkNotFound) {
			return nil, twirp.NewError(twirp.NotFound, err.Error())
		}
		return nil, err
	}

	return &hublive.GetSIPInboundTrunkResponse{Trunk: trunk}, nil
}

func (s *SIPService) GetSIPOutboundTrunk(ctx context.Context, req *hublive.GetSIPOutboundTrunkRequest) (*hublive.GetSIPOutboundTrunkResponse, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if req.SipTrunkId == "" {
		return nil, twirp.NewError(twirp.InvalidArgument, "trunk ID is required")
	}
	AppendLogFields(ctx, "trunkID", req.SipTrunkId)

	trunk, err := s.store.LoadSIPOutboundTrunk(ctx, req.SipTrunkId)
	if err != nil {
		if errors.Is(err, ErrSIPTrunkNotFound) {
			return nil, twirp.NewError(twirp.NotFound, err.Error())
		}
		return nil, err
	}

	return &hublive.GetSIPOutboundTrunkResponse{Trunk: trunk}, nil
}

// deprecated: ListSIPTrunk will be removed in the future
func (s *SIPService) ListSIPTrunk(ctx context.Context, req *hublive.ListSIPTrunkRequest) (*hublive.ListSIPTrunkResponse, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	it := hublive.ListPageIter(s.store.ListSIPTrunk, req)
	defer it.Close()

	items, err := iters.AllPages(ctx, it)
	if err != nil {
		return nil, err
	}
	return &hublive.ListSIPTrunkResponse{Items: items}, nil
}

func ListSIPInboundTrunk(ctx context.Context, s SIPStore, req *hublive.ListSIPInboundTrunkRequest, add ...*hublive.SIPInboundTrunkInfo) (iters.Iter[*hublive.SIPInboundTrunkInfo], error) {
	if s == nil {
		return nil, ErrSIPNotConnected
	}
	pages := hublive.ListPageIter(s.ListSIPInboundTrunk, req)
	it := iters.PagesAsIter(ctx, pages)
	if len(add) != 0 {
		it = iters.MultiIter(true, it, iters.Slice(add))
	}
	return it, nil
}

func ListSIPOutboundTrunk(ctx context.Context, s SIPStore, req *hublive.ListSIPOutboundTrunkRequest, add ...*hublive.SIPOutboundTrunkInfo) (iters.Iter[*hublive.SIPOutboundTrunkInfo], error) {
	if s == nil {
		return nil, ErrSIPNotConnected
	}
	pages := hublive.ListPageIter(s.ListSIPOutboundTrunk, req)
	it := iters.PagesAsIter(ctx, pages)
	if len(add) != 0 {
		it = iters.MultiIter(true, it, iters.Slice(add))
	}
	return it, nil
}

func (s *SIPService) ListSIPInboundTrunk(ctx context.Context, req *hublive.ListSIPInboundTrunkRequest) (*hublive.ListSIPInboundTrunkResponse, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	it, err := ListSIPInboundTrunk(ctx, s.store, req)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	items, err := iters.All(it)
	if err != nil {
		return nil, err
	}
	return &hublive.ListSIPInboundTrunkResponse{Items: items}, nil
}

func (s *SIPService) ListSIPOutboundTrunk(ctx context.Context, req *hublive.ListSIPOutboundTrunkRequest) (*hublive.ListSIPOutboundTrunkResponse, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	it, err := ListSIPOutboundTrunk(ctx, s.store, req)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	items, err := iters.All(it)
	if err != nil {
		return nil, err
	}
	return &hublive.ListSIPOutboundTrunkResponse{Items: items}, nil
}

func (s *SIPService) DeleteSIPTrunk(ctx context.Context, req *hublive.DeleteSIPTrunkRequest) (*hublive.SIPTrunkInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if req.SipTrunkId == "" {
		return nil, twirp.NewError(twirp.InvalidArgument, "trunk ID is required")
	}

	AppendLogFields(ctx, "trunkID", req.SipTrunkId)
	if err := s.store.DeleteSIPTrunk(ctx, req.SipTrunkId); err != nil {
		return nil, err
	}

	return &hublive.SIPTrunkInfo{SipTrunkId: req.SipTrunkId}, nil
}

func (s *SIPService) CreateSIPDispatchRule(ctx context.Context, req *hublive.CreateSIPDispatchRuleRequest) (*hublive.SIPDispatchRuleInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	AppendLogFields(ctx,
		"request", logger.Proto(req),
		"trunkID", req.TrunkIds,
	)
	// Keep ID empty, so that validation can print "<new>" instead of a non-existent ID in the error.
	info := req.DispatchRuleInfo()
	info.SipDispatchRuleId = ""

	// Validate all rules including the new one first.
	it, err := ListSIPDispatchRule(ctx, s.store, &hublive.ListSIPDispatchRuleRequest{
		TrunkIds: req.TrunkIds,
	}, info)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	if _, err = sip.ValidateDispatchRulesIter(it); err != nil {
		return nil, err
	}

	// Now we can generate ID and store.
	info.SipDispatchRuleId = guid.New(utils.SIPDispatchRulePrefix)
	if err := s.store.StoreSIPDispatchRule(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *SIPService) UpdateSIPDispatchRule(ctx context.Context, req *hublive.UpdateSIPDispatchRuleRequest) (*hublive.SIPDispatchRuleInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	AppendLogFields(ctx,
		"request", logger.Proto(req),
		"ruleID", req.SipDispatchRuleId,
	)

	// Validate all trunks including the new one first.
	info, err := s.store.LoadSIPDispatchRule(ctx, req.SipDispatchRuleId)
	if err != nil {
		if errors.Is(err, ErrSIPDispatchRuleNotFound) {
			return nil, twirp.NewError(twirp.NotFound, err.Error())
		}
		return nil, err
	}
	switch a := req.Action.(type) {
	default:
		return nil, errors.New("missing or unsupported action")
	case hublive.UpdateSIPDispatchRuleRequestAction:
		info, err = a.Apply(info)
		if err != nil {
			return nil, err
		}
	}

	it, err := ListSIPDispatchRule(ctx, s.store, &hublive.ListSIPDispatchRuleRequest{
		TrunkIds: info.TrunkIds,
	})
	if err != nil {
		return nil, err
	}
	defer it.Close()
	if _, err = sip.ValidateDispatchRulesIter(it, sip.WithDispatchRuleReplace(func(t *hublive.SIPDispatchRuleInfo) *hublive.SIPDispatchRuleInfo {
		if req.SipDispatchRuleId == t.SipDispatchRuleId {
			return info // updated one
		}
		return t
	})); err != nil {
		return nil, err
	}

	if err := s.store.StoreSIPDispatchRule(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

func ListSIPDispatchRule(ctx context.Context, s SIPStore, req *hublive.ListSIPDispatchRuleRequest, add ...*hublive.SIPDispatchRuleInfo) (iters.Iter[*hublive.SIPDispatchRuleInfo], error) {
	if s == nil {
		return nil, ErrSIPNotConnected
	}
	pages := hublive.ListPageIter(s.ListSIPDispatchRule, req)
	it := iters.PagesAsIter(ctx, pages)
	if len(add) != 0 {
		it = iters.MultiIter(true, it, iters.Slice(add))
	}
	return it, nil
}

func (s *SIPService) ListSIPDispatchRule(ctx context.Context, req *hublive.ListSIPDispatchRuleRequest) (*hublive.ListSIPDispatchRuleResponse, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	it, err := ListSIPDispatchRule(ctx, s.store, req)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	items, err := iters.All(it)
	if err != nil {
		return nil, err
	}
	return &hublive.ListSIPDispatchRuleResponse{Items: items}, nil
}

func (s *SIPService) DeleteSIPDispatchRule(ctx context.Context, req *hublive.DeleteSIPDispatchRuleRequest) (*hublive.SIPDispatchRuleInfo, error) {
	if err := EnsureSIPAdminPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if req.SipDispatchRuleId == "" {
		return nil, twirp.NewError(twirp.InvalidArgument, "dispatch rule ID is required")
	}

	info, err := s.store.LoadSIPDispatchRule(ctx, req.SipDispatchRuleId)
	if err != nil {
		if errors.Is(err, ErrSIPDispatchRuleNotFound) {
			return nil, twirp.NewError(twirp.NotFound, err.Error())
		}
		return nil, err
	}

	if err = s.store.DeleteSIPDispatchRule(ctx, info.SipDispatchRuleId); err != nil {
		return nil, err
	}

	return info, nil
}

func (s *SIPService) CreateSIPParticipant(ctx context.Context, req *hublive.CreateSIPParticipantRequest) (*hublive.SIPParticipantInfo, error) {
	unlikelyLogger := logger.GetLogger().WithUnlikelyValues(
		"room", req.RoomName,
		"sipTrunk", req.SipTrunkId,
		"toUser", req.SipCallTo,
		"participant", req.ParticipantIdentity,
	)
	AppendLogFields(ctx,
		"room", req.RoomName,
		"participant", req.ParticipantIdentity,
		"toUser", req.SipCallTo,
		"trunkID", req.SipTrunkId,
	)
	ireq, err := s.CreateSIPParticipantRequest(ctx, req, "", "", "", "")
	if err != nil {
		unlikelyLogger.Errorw("cannot create sip participant request", err)
		return nil, err
	}
	unlikelyLogger = unlikelyLogger.WithValues(
		"callID", ireq.SipCallId,
		"fromUser", ireq.Number,
		"toHost", ireq.Address,
	)
	AppendLogFields(ctx,
		"callID", ireq.SipCallId,
		"fromUser", ireq.Number,
		"toHost", ireq.Address,
	)

	// CreateSIPParticipant will wait for HubLive Participant to be created and that can take some time.
	// Thus, we must set a higher deadline for it, if it's not set already.
	timeout := 30 * time.Second
	if req.WaitUntilAnswered {
		timeout = 80 * time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	} else {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	resp, err := s.psrpcClient.CreateSIPParticipant(ctx, "", ireq, psrpc.WithRequestTimeout(timeout))
	if err != nil {
		unlikelyLogger.Errorw("cannot create sip participant", err)
		return nil, err
	}
	return &hublive.SIPParticipantInfo{
		ParticipantId:       resp.ParticipantId,
		ParticipantIdentity: resp.ParticipantIdentity,
		RoomName:            req.RoomName,
		SipCallId:           ireq.SipCallId,
	}, nil
}

func (s *SIPService) CreateSIPParticipantRequest(ctx context.Context, req *hublive.CreateSIPParticipantRequest, projectID, host, wsUrl, token string) (*rpc.InternalCreateSIPParticipantRequest, error) {
	if err := EnsureSIPCallPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if s.store == nil {
		return nil, ErrSIPNotConnected
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}
	callID := sip.NewCallID()
	log := logger.GetLogger().WithUnlikelyValues(
		"callID", callID,
		"room", req.RoomName,
		"sipTrunk", req.SipTrunkId,
		"toUser", req.SipCallTo,
	)
	if projectID != "" {
		log = log.WithValues("projectID", projectID)
	}

	var trunk *hublive.SIPOutboundTrunkInfo
	if req.SipTrunkId != "" {
		var err error
		trunk, err = s.store.LoadSIPOutboundTrunk(ctx, req.SipTrunkId)
		if err != nil {
			log.Errorw("cannot get trunk to update sip participant", err)
			if errors.Is(err, ErrSIPTrunkNotFound) {
				return nil, twirp.NewError(twirp.NotFound, err.Error())
			}
			return nil, err
		}
	}
	if trunk != nil && trunk.FromHost != "" {
		host = trunk.FromHost
	} else if t := req.Trunk; t != nil && t.FromHost != "" {
		host = t.FromHost
	}
	return rpc.NewCreateSIPParticipantRequest(projectID, callID, host, wsUrl, token, req, trunk)
}

func (s *SIPService) TransferSIPParticipant(ctx context.Context, req *hublive.TransferSIPParticipantRequest) (*emptypb.Empty, error) {
	log := logger.GetLogger().WithUnlikelyValues(
		"room", req.RoomName,
		"participant", req.ParticipantIdentity,
		"transferTo", req.TransferTo,
		"playDialtone", req.PlayDialtone,
	)
	AppendLogFields(ctx,
		"room", req.RoomName,
		"participant", req.ParticipantIdentity,
		"transferTo", req.TransferTo,
		"playDialtone", req.PlayDialtone,
	)

	ireq, err := s.transferSIPParticipantRequest(ctx, req)
	if err != nil {
		log.Errorw("cannot create transfer sip participant request", err)
		return nil, err
	}

	// by default we set the timeout to be 30 seconds.
	// this timeout covers:
	//  - a network failure between this process and the HubLive SIP bridge
	//  - the SIP transfer target not returning 200 OK fast enough.
	// WARN: any timeout/cancellation of a SIP transfer risks leaving
	// either the SIP bridge, or the SIP REFER exchange, in a "unknown" state.
	timeout := 30 * time.Second
	if req.RingingTimeout != nil {
		timeout = req.RingingTimeout.AsDuration()
	}

	// it's also possible the ctx has a Deadline.
	// in that case we want to use that deadline,
	// or our timeout, whichover is soonest.
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	} else {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	_, err = s.psrpcClient.TransferSIPParticipant(ctx, ireq.SipCallId, ireq, psrpc.WithRequestTimeout(timeout))
	if err != nil {
		log.Errorw("cannot transfer sip participant", err)
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *SIPService) transferSIPParticipantRequest(ctx context.Context, req *hublive.TransferSIPParticipantRequest) (*rpc.InternalTransferSIPParticipantRequest, error) {
	if req.RoomName == "" {
		return nil, psrpc.NewErrorf(psrpc.InvalidArgument, "Missing room name")
	}

	if req.ParticipantIdentity == "" {
		return nil, psrpc.NewErrorf(psrpc.InvalidArgument, "Missing participant identity")
	}

	if err := EnsureSIPCallPermission(ctx); err != nil {
		return nil, twirpAuthError(err)
	}
	if err := EnsureAdminPermission(ctx, hublive.RoomName(req.RoomName)); err != nil {
		return nil, twirpAuthError(err)
	}
	if err := req.Validate(); err != nil {
		return nil, twirp.WrapError(twirp.NewError(twirp.InvalidArgument, err.Error()), err)
	}

	resp, err := s.roomService.GetParticipant(ctx, &hublive.RoomParticipantIdentity{
		Room:     req.RoomName,
		Identity: req.ParticipantIdentity,
	})

	if err != nil {
		return nil, err
	}

	callID, ok := resp.Attributes[hublive.AttrSIPCallID]
	if !ok {
		return nil, psrpc.NewErrorf(psrpc.InvalidArgument, "no SIP session associated with participant")
	}

	return &rpc.InternalTransferSIPParticipantRequest{
		SipCallId:      callID,
		TransferTo:     req.TransferTo,
		PlayDialtone:   req.PlayDialtone,
		Headers:        req.Headers,
		RingingTimeout: req.RingingTimeout,
	}, nil
}