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

func (s *Service) MessagesGetWebPagePreview570D6F6F(ctx context.Context, request *mtproto.TLMessagesGetWebPagePreview570D6F6F) (*mtproto.Messages_WebPagePreview, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetWebPagePreview570D6F6F - request: %s", request)
	r, err := c.MessagesGetWebPagePreview570D6F6F(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetWebPagePreview570D6F6F - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetWebPage8D9692A3(ctx context.Context, request *mtproto.TLMessagesGetWebPage8D9692A3) (*mtproto.Messages_WebPage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetWebPage8D9692A3 - request: %s", request)
	r, err := c.MessagesGetWebPage8D9692A3(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetWebPage8D9692A3 - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetWebPagePreview8B68B0CC(ctx context.Context, request *mtproto.TLMessagesGetWebPagePreview8B68B0CC) (*mtproto.MessageMedia, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetWebPagePreview8B68B0CC - request: %s", request)
	r, err := c.MessagesGetWebPagePreview8B68B0CC(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetWebPagePreview8B68B0CC - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetWebPage32CA8F91(ctx context.Context, request *mtproto.TLMessagesGetWebPage32CA8F91) (*mtproto.WebPage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetWebPage32CA8F91 - request: %s", request)
	r, err := c.MessagesGetWebPage32CA8F91(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetWebPage32CA8F91 - reply: %s", r)
	return r, nil
}
