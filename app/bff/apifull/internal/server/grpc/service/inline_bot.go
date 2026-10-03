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

func (s *Service) MessagesGetInlineBotResults(ctx context.Context, request *mtproto.TLMessagesGetInlineBotResults) (*mtproto.Messages_BotResults, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetInlineBotResults - request: %s", request)
	r, err := c.MessagesGetInlineBotResults(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetInlineBotResults - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetInlineBotResults(ctx context.Context, request *mtproto.TLMessagesSetInlineBotResults) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetInlineBotResults - request: %s", request)
	r, err := c.MessagesSetInlineBotResults(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetInlineBotResults - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendInlineBotResult(ctx context.Context, request *mtproto.TLMessagesSendInlineBotResult) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendInlineBotResult - request: %s", request)
	r, err := c.MessagesSendInlineBotResult(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendInlineBotResult - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesEditInlineBotMessage(ctx context.Context, request *mtproto.TLMessagesEditInlineBotMessage) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesEditInlineBotMessage - request: %s", request)
	r, err := c.MessagesEditInlineBotMessage(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesEditInlineBotMessage - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetBotCallbackAnswer(ctx context.Context, request *mtproto.TLMessagesGetBotCallbackAnswer) (*mtproto.Messages_BotCallbackAnswer, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetBotCallbackAnswer - request: %s", request)
	r, err := c.MessagesGetBotCallbackAnswer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetBotCallbackAnswer - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetBotCallbackAnswer(ctx context.Context, request *mtproto.TLMessagesSetBotCallbackAnswer) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetBotCallbackAnswer - request: %s", request)
	r, err := c.MessagesSetBotCallbackAnswer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetBotCallbackAnswer - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendBotRequestedPeer(ctx context.Context, request *mtproto.TLMessagesSendBotRequestedPeer) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendBotRequestedPeer - request: %s", request)
	r, err := c.MessagesSendBotRequestedPeer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendBotRequestedPeer - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetBotGuestChatResult(ctx context.Context, request *mtproto.TLMessagesSetBotGuestChatResult) (*mtproto.InputBotInlineMessageID, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetBotGuestChatResult - request: %s", request)
	r, err := c.MessagesSetBotGuestChatResult(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetBotGuestChatResult - reply: %s", r)
	return r, nil
}
