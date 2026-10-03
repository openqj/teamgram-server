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

func (s *Service) StatsGetBroadcastStats(ctx context.Context, request *mtproto.TLStatsGetBroadcastStats) (*mtproto.Stats_BroadcastStats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetBroadcastStats - request: %s", request)
	r, err := c.StatsGetBroadcastStats(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetBroadcastStats - reply: %s", r)
	return r, nil
}

func (s *Service) StatsLoadAsyncGraph(ctx context.Context, request *mtproto.TLStatsLoadAsyncGraph) (*mtproto.StatsGraph, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsLoadAsyncGraph - request: %s", request)
	r, err := c.StatsLoadAsyncGraph(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsLoadAsyncGraph - reply: %s", r)
	return r, nil
}

func (s *Service) StatsGetMegagroupStats(ctx context.Context, request *mtproto.TLStatsGetMegagroupStats) (*mtproto.Stats_MegagroupStats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetMegagroupStats - request: %s", request)
	r, err := c.StatsGetMegagroupStats(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetMegagroupStats - reply: %s", r)
	return r, nil
}

func (s *Service) StatsGetMessagePublicForwards5F150144(ctx context.Context, request *mtproto.TLStatsGetMessagePublicForwards5F150144) (*mtproto.Stats_PublicForwards, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetMessagePublicForwards5F150144 - request: %s", request)
	r, err := c.StatsGetMessagePublicForwards5F150144(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetMessagePublicForwards5F150144 - reply: %s", r)
	return r, nil
}

func (s *Service) StatsGetMessageStats(ctx context.Context, request *mtproto.TLStatsGetMessageStats) (*mtproto.Stats_MessageStats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetMessageStats - request: %s", request)
	r, err := c.StatsGetMessageStats(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetMessageStats - reply: %s", r)
	return r, nil
}

func (s *Service) StatsGetStoryStats(ctx context.Context, request *mtproto.TLStatsGetStoryStats) (*mtproto.Stats_StoryStats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetStoryStats - request: %s", request)
	r, err := c.StatsGetStoryStats(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetStoryStats - reply: %s", r)
	return r, nil
}

func (s *Service) StatsGetStoryPublicForwards(ctx context.Context, request *mtproto.TLStatsGetStoryPublicForwards) (*mtproto.Stats_PublicForwards, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetStoryPublicForwards - request: %s", request)
	r, err := c.StatsGetStoryPublicForwards(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetStoryPublicForwards - reply: %s", r)
	return r, nil
}

func (s *Service) StatsGetPollStats(ctx context.Context, request *mtproto.TLStatsGetPollStats) (*mtproto.Stats_PollStats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetPollStats - request: %s", request)
	r, err := c.StatsGetPollStats(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetPollStats - reply: %s", r)
	return r, nil
}

func (s *Service) StatsGetMessagePublicForwards5630281B(ctx context.Context, request *mtproto.TLStatsGetMessagePublicForwards5630281B) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StatsGetMessagePublicForwards5630281B - request: %s", request)
	r, err := c.StatsGetMessagePublicForwards5630281B(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StatsGetMessagePublicForwards5630281B - reply: %s", r)
	return r, nil
}
