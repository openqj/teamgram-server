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

func (s *Service) AccountUpdateConnectedBot(ctx context.Context, request *mtproto.TLAccountUpdateConnectedBot) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountUpdateConnectedBot - request: %s", request)
	r, err := c.AccountUpdateConnectedBot(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountUpdateConnectedBot - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetConnectedBots(ctx context.Context, request *mtproto.TLAccountGetConnectedBots) (*mtproto.Account_ConnectedBots, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetConnectedBots - request: %s", request)
	r, err := c.AccountGetConnectedBots(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetConnectedBots - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetBotBusinessConnection(ctx context.Context, request *mtproto.TLAccountGetBotBusinessConnection) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetBotBusinessConnection - request: %s", request)
	r, err := c.AccountGetBotBusinessConnection(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetBotBusinessConnection - reply: %s", r)
	return r, nil
}

func (s *Service) AccountToggleConnectedBotPaused(ctx context.Context, request *mtproto.TLAccountToggleConnectedBotPaused) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountToggleConnectedBotPaused - request: %s", request)
	r, err := c.AccountToggleConnectedBotPaused(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountToggleConnectedBotPaused - reply: %s", r)
	return r, nil
}

func (s *Service) AccountDisablePeerConnectedBot(ctx context.Context, request *mtproto.TLAccountDisablePeerConnectedBot) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountDisablePeerConnectedBot - request: %s", request)
	r, err := c.AccountDisablePeerConnectedBot(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountDisablePeerConnectedBot - reply: %s", r)
	return r, nil
}
