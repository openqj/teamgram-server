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

func (s *Service) MessagesGetScheduledHistory(ctx context.Context, request *mtproto.TLMessagesGetScheduledHistory) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetScheduledHistory - request: %s", request)
	r, err := c.MessagesGetScheduledHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetScheduledHistory - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetScheduledMessages(ctx context.Context, request *mtproto.TLMessagesGetScheduledMessages) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetScheduledMessages - request: %s", request)
	r, err := c.MessagesGetScheduledMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetScheduledMessages - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendScheduledMessages(ctx context.Context, request *mtproto.TLMessagesSendScheduledMessages) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendScheduledMessages - request: %s", request)
	r, err := c.MessagesSendScheduledMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendScheduledMessages - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeleteScheduledMessages(ctx context.Context, request *mtproto.TLMessagesDeleteScheduledMessages) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeleteScheduledMessages - request: %s", request)
	r, err := c.MessagesDeleteScheduledMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeleteScheduledMessages - reply: %s", r)
	return r, nil
}
