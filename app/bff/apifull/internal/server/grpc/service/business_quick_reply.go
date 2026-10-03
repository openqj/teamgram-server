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

func (s *Service) MessagesGetQuickReplies(ctx context.Context, request *mtproto.TLMessagesGetQuickReplies) (*mtproto.Messages_QuickReplies, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetQuickReplies - request: %s", request)
	r, err := c.MessagesGetQuickReplies(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetQuickReplies - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReorderQuickReplies(ctx context.Context, request *mtproto.TLMessagesReorderQuickReplies) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReorderQuickReplies - request: %s", request)
	r, err := c.MessagesReorderQuickReplies(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReorderQuickReplies - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesCheckQuickReplyShortcut(ctx context.Context, request *mtproto.TLMessagesCheckQuickReplyShortcut) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesCheckQuickReplyShortcut - request: %s", request)
	r, err := c.MessagesCheckQuickReplyShortcut(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesCheckQuickReplyShortcut - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesEditQuickReplyShortcut(ctx context.Context, request *mtproto.TLMessagesEditQuickReplyShortcut) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesEditQuickReplyShortcut - request: %s", request)
	r, err := c.MessagesEditQuickReplyShortcut(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesEditQuickReplyShortcut - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeleteQuickReplyShortcut(ctx context.Context, request *mtproto.TLMessagesDeleteQuickReplyShortcut) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeleteQuickReplyShortcut - request: %s", request)
	r, err := c.MessagesDeleteQuickReplyShortcut(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeleteQuickReplyShortcut - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetQuickReplyMessages(ctx context.Context, request *mtproto.TLMessagesGetQuickReplyMessages) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetQuickReplyMessages - request: %s", request)
	r, err := c.MessagesGetQuickReplyMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetQuickReplyMessages - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendQuickReplyMessages(ctx context.Context, request *mtproto.TLMessagesSendQuickReplyMessages) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendQuickReplyMessages - request: %s", request)
	r, err := c.MessagesSendQuickReplyMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendQuickReplyMessages - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeleteQuickReplyMessages(ctx context.Context, request *mtproto.TLMessagesDeleteQuickReplyMessages) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeleteQuickReplyMessages - request: %s", request)
	r, err := c.MessagesDeleteQuickReplyMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeleteQuickReplyMessages - reply: %s", r)
	return r, nil
}
