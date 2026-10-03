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

func (s *Service) ChannelsSetBoostsToUnblockRestrictions(ctx context.Context, request *mtproto.TLChannelsSetBoostsToUnblockRestrictions) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsSetBoostsToUnblockRestrictions - request: %s", request)
	r, err := c.ChannelsSetBoostsToUnblockRestrictions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsSetBoostsToUnblockRestrictions - reply: %s", r)
	return r, nil
}

func (s *Service) PremiumGetBoostsList(ctx context.Context, request *mtproto.TLPremiumGetBoostsList) (*mtproto.Premium_BoostsList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PremiumGetBoostsList - request: %s", request)
	r, err := c.PremiumGetBoostsList(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PremiumGetBoostsList - reply: %s", r)
	return r, nil
}

func (s *Service) PremiumGetMyBoosts(ctx context.Context, request *mtproto.TLPremiumGetMyBoosts) (*mtproto.Premium_MyBoosts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PremiumGetMyBoosts - request: %s", request)
	r, err := c.PremiumGetMyBoosts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PremiumGetMyBoosts - reply: %s", r)
	return r, nil
}

func (s *Service) PremiumApplyBoost(ctx context.Context, request *mtproto.TLPremiumApplyBoost) (*mtproto.Premium_MyBoosts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PremiumApplyBoost - request: %s", request)
	r, err := c.PremiumApplyBoost(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PremiumApplyBoost - reply: %s", r)
	return r, nil
}

func (s *Service) PremiumGetBoostsStatus(ctx context.Context, request *mtproto.TLPremiumGetBoostsStatus) (*mtproto.Premium_BoostsStatus, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PremiumGetBoostsStatus - request: %s", request)
	r, err := c.PremiumGetBoostsStatus(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PremiumGetBoostsStatus - reply: %s", r)
	return r, nil
}

func (s *Service) PremiumGetUserBoosts(ctx context.Context, request *mtproto.TLPremiumGetUserBoosts) (*mtproto.Premium_BoostsList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PremiumGetUserBoosts - request: %s", request)
	r, err := c.PremiumGetUserBoosts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PremiumGetUserBoosts - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetBoostsStatus(ctx context.Context, request *mtproto.TLStoriesGetBoostsStatus) (*mtproto.Stories_BoostsStatus, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetBoostsStatus - request: %s", request)
	r, err := c.StoriesGetBoostsStatus(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetBoostsStatus - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetBoostersList(ctx context.Context, request *mtproto.TLStoriesGetBoostersList) (*mtproto.Stories_BoostersList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetBoostersList - request: %s", request)
	r, err := c.StoriesGetBoostersList(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetBoostersList - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesCanApplyBoost(ctx context.Context, request *mtproto.TLStoriesCanApplyBoost) (*mtproto.Stories_CanApplyBoostResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesCanApplyBoost - request: %s", request)
	r, err := c.StoriesCanApplyBoost(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesCanApplyBoost - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesApplyBoost(ctx context.Context, request *mtproto.TLStoriesApplyBoost) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesApplyBoost - request: %s", request)
	r, err := c.StoriesApplyBoost(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesApplyBoost - reply: %s", r)
	return r, nil
}
