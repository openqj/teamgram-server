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

func (s *Service) MessagesRequestMainWebView(ctx context.Context, request *mtproto.TLMessagesRequestMainWebView) (*mtproto.WebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestMainWebView - request: %s", request)
	r, err := c.MessagesRequestMainWebView(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestMainWebView - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetPopularAppBots(ctx context.Context, request *mtproto.TLBotsGetPopularAppBots) (*mtproto.Bots_PopularAppBots, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetPopularAppBots - request: %s", request)
	r, err := c.BotsGetPopularAppBots(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetPopularAppBots - reply: %s", r)
	return r, nil
}

func (s *Service) BotsAddPreviewMedia(ctx context.Context, request *mtproto.TLBotsAddPreviewMedia) (*mtproto.BotPreviewMedia, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsAddPreviewMedia - request: %s", request)
	r, err := c.BotsAddPreviewMedia(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsAddPreviewMedia - reply: %s", r)
	return r, nil
}

func (s *Service) BotsEditPreviewMedia(ctx context.Context, request *mtproto.TLBotsEditPreviewMedia) (*mtproto.BotPreviewMedia, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsEditPreviewMedia - request: %s", request)
	r, err := c.BotsEditPreviewMedia(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsEditPreviewMedia - reply: %s", r)
	return r, nil
}

func (s *Service) BotsDeletePreviewMedia(ctx context.Context, request *mtproto.TLBotsDeletePreviewMedia) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsDeletePreviewMedia - request: %s", request)
	r, err := c.BotsDeletePreviewMedia(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsDeletePreviewMedia - reply: %s", r)
	return r, nil
}

func (s *Service) BotsReorderPreviewMedias(ctx context.Context, request *mtproto.TLBotsReorderPreviewMedias) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsReorderPreviewMedias - request: %s", request)
	r, err := c.BotsReorderPreviewMedias(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsReorderPreviewMedias - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetPreviewInfo(ctx context.Context, request *mtproto.TLBotsGetPreviewInfo) (*mtproto.Bots_PreviewInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetPreviewInfo - request: %s", request)
	r, err := c.BotsGetPreviewInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetPreviewInfo - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetPreviewMedias(ctx context.Context, request *mtproto.TLBotsGetPreviewMedias) (*mtproto.Vector_BotPreviewMedia, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetPreviewMedias - request: %s", request)
	r, err := c.BotsGetPreviewMedias(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetPreviewMedias - reply: %s", r)
	return r, nil
}
