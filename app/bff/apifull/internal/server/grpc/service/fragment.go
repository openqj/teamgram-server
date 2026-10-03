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

func (s *Service) AccountReorderUsernames(ctx context.Context, request *mtproto.TLAccountReorderUsernames) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountReorderUsernames - request: %s", request)
	r, err := c.AccountReorderUsernames(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountReorderUsernames - reply: %s", r)
	return r, nil
}

func (s *Service) AccountToggleUsername(ctx context.Context, request *mtproto.TLAccountToggleUsername) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountToggleUsername - request: %s", request)
	r, err := c.AccountToggleUsername(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountToggleUsername - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsReorderUsernames(ctx context.Context, request *mtproto.TLChannelsReorderUsernames) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsReorderUsernames - request: %s", request)
	r, err := c.ChannelsReorderUsernames(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsReorderUsernames - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsToggleUsername(ctx context.Context, request *mtproto.TLChannelsToggleUsername) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsToggleUsername - request: %s", request)
	r, err := c.ChannelsToggleUsername(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsToggleUsername - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsDeactivateAllUsernames(ctx context.Context, request *mtproto.TLChannelsDeactivateAllUsernames) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsDeactivateAllUsernames - request: %s", request)
	r, err := c.ChannelsDeactivateAllUsernames(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsDeactivateAllUsernames - reply: %s", r)
	return r, nil
}

func (s *Service) BotsReorderUsernames(ctx context.Context, request *mtproto.TLBotsReorderUsernames) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsReorderUsernames - request: %s", request)
	r, err := c.BotsReorderUsernames(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsReorderUsernames - reply: %s", r)
	return r, nil
}

func (s *Service) BotsToggleUsername(ctx context.Context, request *mtproto.TLBotsToggleUsername) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsToggleUsername - request: %s", request)
	r, err := c.BotsToggleUsername(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsToggleUsername - reply: %s", r)
	return r, nil
}
