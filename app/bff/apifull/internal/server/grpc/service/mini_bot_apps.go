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

func (s *Service) MessagesRequestWebView(ctx context.Context, request *mtproto.TLMessagesRequestWebView) (*mtproto.WebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestWebView - request: %s", request)
	r, err := c.MessagesRequestWebView(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestWebView - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesProlongWebView(ctx context.Context, request *mtproto.TLMessagesProlongWebView) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesProlongWebView - request: %s", request)
	r, err := c.MessagesProlongWebView(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesProlongWebView - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestSimpleWebView413A3E73(ctx context.Context, request *mtproto.TLMessagesRequestSimpleWebView413A3E73) (*mtproto.WebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestSimpleWebView413A3E73 - request: %s", request)
	r, err := c.MessagesRequestSimpleWebView413A3E73(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestSimpleWebView413A3E73 - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendWebViewResultMessage(ctx context.Context, request *mtproto.TLMessagesSendWebViewResultMessage) (*mtproto.WebViewMessageSent, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendWebViewResultMessage - request: %s", request)
	r, err := c.MessagesSendWebViewResultMessage(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendWebViewResultMessage - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendWebViewData(ctx context.Context, request *mtproto.TLMessagesSendWebViewData) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendWebViewData - request: %s", request)
	r, err := c.MessagesSendWebViewData(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendWebViewData - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetBotApp(ctx context.Context, request *mtproto.TLMessagesGetBotApp) (*mtproto.Messages_BotApp, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetBotApp - request: %s", request)
	r, err := c.MessagesGetBotApp(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetBotApp - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestAppWebView53618BCE(ctx context.Context, request *mtproto.TLMessagesRequestAppWebView53618BCE) (*mtproto.WebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestAppWebView53618BCE - request: %s", request)
	r, err := c.MessagesRequestAppWebView53618BCE(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestAppWebView53618BCE - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestChatJoinWebView(ctx context.Context, request *mtproto.TLMessagesRequestChatJoinWebView) (*mtproto.WebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestChatJoinWebView - request: %s", request)
	r, err := c.MessagesRequestChatJoinWebView(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestChatJoinWebView - reply: %s", r)
	return r, nil
}

func (s *Service) BotsCanSendMessage(ctx context.Context, request *mtproto.TLBotsCanSendMessage) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsCanSendMessage - request: %s", request)
	r, err := c.BotsCanSendMessage(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsCanSendMessage - reply: %s", r)
	return r, nil
}

func (s *Service) BotsAllowSendMessage(ctx context.Context, request *mtproto.TLBotsAllowSendMessage) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsAllowSendMessage - request: %s", request)
	r, err := c.BotsAllowSendMessage(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsAllowSendMessage - reply: %s", r)
	return r, nil
}

func (s *Service) BotsInvokeWebViewCustomMethod(ctx context.Context, request *mtproto.TLBotsInvokeWebViewCustomMethod) (*mtproto.DataJSON, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsInvokeWebViewCustomMethod - request: %s", request)
	r, err := c.BotsInvokeWebViewCustomMethod(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsInvokeWebViewCustomMethod - reply: %s", r)
	return r, nil
}

func (s *Service) BotsCheckDownloadFileParams(ctx context.Context, request *mtproto.TLBotsCheckDownloadFileParams) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsCheckDownloadFileParams - request: %s", request)
	r, err := c.BotsCheckDownloadFileParams(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsCheckDownloadFileParams - reply: %s", r)
	return r, nil
}

func (s *Service) BotsRequestWebViewButton(ctx context.Context, request *mtproto.TLBotsRequestWebViewButton) (*mtproto.Bots_RequestedButton, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsRequestWebViewButton - request: %s", request)
	r, err := c.BotsRequestWebViewButton(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsRequestWebViewButton - reply: %s", r)
	return r, nil
}

func (s *Service) BotsGetRequestedWebViewButton(ctx context.Context, request *mtproto.TLBotsGetRequestedWebViewButton) (*mtproto.KeyboardButton, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsGetRequestedWebViewButton - request: %s", request)
	r, err := c.BotsGetRequestedWebViewButton(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsGetRequestedWebViewButton - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestSimpleWebView1A46500A(ctx context.Context, request *mtproto.TLMessagesRequestSimpleWebView1A46500A) (*mtproto.SimpleWebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestSimpleWebView1A46500A - request: %s", request)
	r, err := c.MessagesRequestSimpleWebView1A46500A(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestSimpleWebView1A46500A - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestAppWebView8C5A3B3C(ctx context.Context, request *mtproto.TLMessagesRequestAppWebView8C5A3B3C) (*mtproto.AppWebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestAppWebView8C5A3B3C - request: %s", request)
	r, err := c.MessagesRequestAppWebView8C5A3B3C(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestAppWebView8C5A3B3C - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestSimpleWebView299BEC8E(ctx context.Context, request *mtproto.TLMessagesRequestSimpleWebView299BEC8E) (*mtproto.SimpleWebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestSimpleWebView299BEC8E - request: %s", request)
	r, err := c.MessagesRequestSimpleWebView299BEC8E(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestSimpleWebView299BEC8E - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestSimpleWebView6ABB2F73(ctx context.Context, request *mtproto.TLMessagesRequestSimpleWebView6ABB2F73) (*mtproto.SimpleWebViewResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestSimpleWebView6ABB2F73 - request: %s", request)
	r, err := c.MessagesRequestSimpleWebView6ABB2F73(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestSimpleWebView6ABB2F73 - reply: %s", r)
	return r, nil
}
