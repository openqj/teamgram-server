// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/core"
)

func (s *Service) CommunitiesCreate(ctx context.Context, request *mtproto.TLCommunitiesCreate) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesCreate - request: %s", request)
	r, err := c.CommunitiesCreate(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesCreate - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesTogglePeerLink(ctx context.Context, request *mtproto.TLCommunitiesTogglePeerLink) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesTogglePeerLink - request: %s", request)
	r, err := c.CommunitiesTogglePeerLink(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesTogglePeerLink - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesGetJoinedCommunities(ctx context.Context, request *mtproto.TLCommunitiesGetJoinedCommunities) (*mtproto.Messages_Chats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesGetJoinedCommunities - request: %s", request)
	r, err := c.CommunitiesGetJoinedCommunities(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesGetJoinedCommunities - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesToggleCommunityCollapsedInDialogs(ctx context.Context, request *mtproto.TLCommunitiesToggleCommunityCollapsedInDialogs) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesToggleCommunityCollapsedInDialogs - request: %s", request)
	r, err := c.CommunitiesToggleCommunityCollapsedInDialogs(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesToggleCommunityCollapsedInDialogs - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesGetPeerLinkRequests(ctx context.Context, request *mtproto.TLCommunitiesGetPeerLinkRequests) (*mtproto.Communities_PeerLinkRequests, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesGetPeerLinkRequests - request: %s", request)
	r, err := c.CommunitiesGetPeerLinkRequests(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesGetPeerLinkRequests - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesTogglePeerLinkRequestApproval(ctx context.Context, request *mtproto.TLCommunitiesTogglePeerLinkRequestApproval) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesTogglePeerLinkRequestApproval - request: %s", request)
	r, err := c.CommunitiesTogglePeerLinkRequestApproval(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesTogglePeerLinkRequestApproval - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesToggleAllPeerLinkRequestApproval(ctx context.Context, request *mtproto.TLCommunitiesToggleAllPeerLinkRequestApproval) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesToggleAllPeerLinkRequestApproval - request: %s", request)
	r, err := c.CommunitiesToggleAllPeerLinkRequestApproval(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesToggleAllPeerLinkRequestApproval - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesToggleParticipantBanned(ctx context.Context, request *mtproto.TLCommunitiesToggleParticipantBanned) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesToggleParticipantBanned - request: %s", request)
	r, err := c.CommunitiesToggleParticipantBanned(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesToggleParticipantBanned - reply: %s", r)
	return r, nil
}

func (s *Service) CommunitiesGetParticipantJoinedChats(ctx context.Context, request *mtproto.TLCommunitiesGetParticipantJoinedChats) (*mtproto.Communities_ParticipantJoinedChats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("CommunitiesGetParticipantJoinedChats - request: %s", request)
	r, err := c.CommunitiesGetParticipantJoinedChats(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("CommunitiesGetParticipantJoinedChats - reply: %s", r)
	return r, nil
}
