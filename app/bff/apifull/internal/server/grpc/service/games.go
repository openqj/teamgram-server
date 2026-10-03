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

func (s *Service) MessagesSetGameScore(ctx context.Context, request *mtproto.TLMessagesSetGameScore) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetGameScore - request: %s", request)
	r, err := c.MessagesSetGameScore(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetGameScore - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetInlineGameScore(ctx context.Context, request *mtproto.TLMessagesSetInlineGameScore) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetInlineGameScore - request: %s", request)
	r, err := c.MessagesSetInlineGameScore(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetInlineGameScore - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetGameHighScores(ctx context.Context, request *mtproto.TLMessagesGetGameHighScores) (*mtproto.Messages_HighScores, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetGameHighScores - request: %s", request)
	r, err := c.MessagesGetGameHighScores(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetGameHighScores - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetInlineGameHighScores(ctx context.Context, request *mtproto.TLMessagesGetInlineGameHighScores) (*mtproto.Messages_HighScores, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetInlineGameHighScores - request: %s", request)
	r, err := c.MessagesGetInlineGameHighScores(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetInlineGameHighScores - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetEmojiGameInfo(ctx context.Context, request *mtproto.TLMessagesGetEmojiGameInfo) (*mtproto.Messages_EmojiGameInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetEmojiGameInfo - request: %s", request)
	r, err := c.MessagesGetEmojiGameInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetEmojiGameInfo - reply: %s", r)
	return r, nil
}
