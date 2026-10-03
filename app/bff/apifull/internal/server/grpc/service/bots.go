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

func (s *Service) BotsSetBotCommands(ctx context.Context, request *mtproto.TLBotsSetBotCommands) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsSetBotCommands - request: %s", request)
	r, err := c.BotsSetBotCommands(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsSetBotCommands - reply: %s", r)
	return r, nil
}

func (s *Service) BotsResetBotCommands(ctx context.Context, request *mtproto.TLBotsResetBotCommands) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsResetBotCommands - request: %s", request)
	r, err := c.BotsResetBotCommands(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsResetBotCommands - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetBotCommands(ctx context.Context, request *mtproto.TLBotsGetBotCommands) (*mtproto.Vector_BotCommand, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetBotCommands - request: %s", request)
	r, err := c.BotsGetBotCommands(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetBotCommands - reply: %s", r)
	return r, nil
}

func (s *Service) BotsSetBotInfo(ctx context.Context, request *mtproto.TLBotsSetBotInfo) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsSetBotInfo - request: %s", request)
	r, err := c.BotsSetBotInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsSetBotInfo - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetBotInfoDCD914FD(ctx context.Context, request *mtproto.TLBotsGetBotInfoDCD914FD) (*mtproto.Bots_BotInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetBotInfoDCD914FD - request: %s", request)
	r, err := c.BotsGetBotInfoDCD914FD(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetBotInfoDCD914FD - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetAdminedBots(ctx context.Context, request *mtproto.TLBotsGetAdminedBots) (*mtproto.Vector_User, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetAdminedBots - request: %s", request)
	r, err := c.BotsGetAdminedBots(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetAdminedBots - reply: %s", r)
	return r, nil
}

func (s *Service) BotsCheckUsername(ctx context.Context, request *mtproto.TLBotsCheckUsername) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsCheckUsername - request: %s", request)
	r, err := c.BotsCheckUsername(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsCheckUsername - reply: %s", r)
	return r, nil
}

func (s *Service) BotsCreateBot(ctx context.Context, request *mtproto.TLBotsCreateBot) (*mtproto.User, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsCreateBot - request: %s", request)
	r, err := c.BotsCreateBot(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsCreateBot - reply: %s", r)
	return r, nil
}

func (s *Service) BotsExportBotToken(ctx context.Context, request *mtproto.TLBotsExportBotToken) (*mtproto.Bots_ExportedBotToken, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsExportBotToken - request: %s", request)
	r, err := c.BotsExportBotToken(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsExportBotToken - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetAccessSettings(ctx context.Context, request *mtproto.TLBotsGetAccessSettings) (*mtproto.Bots_AccessSettings, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetAccessSettings - request: %s", request)
	r, err := c.BotsGetAccessSettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetAccessSettings - reply: %s", r)
	return r, nil
}

func (s *Service) BotsEditAccessSettings(ctx context.Context, request *mtproto.TLBotsEditAccessSettings) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsEditAccessSettings - request: %s", request)
	r, err := c.BotsEditAccessSettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsEditAccessSettings - reply: %s", r)
	return r, nil
}

func (s *Service) BotsSetJoinChatResults(ctx context.Context, request *mtproto.TLBotsSetJoinChatResults) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsSetJoinChatResults - request: %s", request)
	r, err := c.BotsSetJoinChatResults(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsSetJoinChatResults - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetBotInfo75EC12E6(ctx context.Context, request *mtproto.TLBotsGetBotInfo75EC12E6) (*mtproto.Vector_String, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetBotInfo75EC12E6 - request: %s", request)
	r, err := c.BotsGetBotInfo75EC12E6(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetBotInfo75EC12E6 - reply: %s", r)
	return r, nil
}
