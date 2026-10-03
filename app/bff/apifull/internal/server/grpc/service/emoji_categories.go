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

func (s *Service) MessagesGetEmojiGroups(ctx context.Context, request *mtproto.TLMessagesGetEmojiGroups) (*mtproto.Messages_EmojiGroups, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetEmojiGroups - request: %s", request)
	r, err := c.MessagesGetEmojiGroups(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetEmojiGroups - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetEmojiStatusGroups(ctx context.Context, request *mtproto.TLMessagesGetEmojiStatusGroups) (*mtproto.Messages_EmojiGroups, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetEmojiStatusGroups - request: %s", request)
	r, err := c.MessagesGetEmojiStatusGroups(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetEmojiStatusGroups - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetEmojiProfilePhotoGroups(ctx context.Context, request *mtproto.TLMessagesGetEmojiProfilePhotoGroups) (*mtproto.Messages_EmojiGroups, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetEmojiProfilePhotoGroups - request: %s", request)
	r, err := c.MessagesGetEmojiProfilePhotoGroups(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetEmojiProfilePhotoGroups - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetEmojiStickerGroups(ctx context.Context, request *mtproto.TLMessagesGetEmojiStickerGroups) (*mtproto.Messages_EmojiGroups, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetEmojiStickerGroups - request: %s", request)
	r, err := c.MessagesGetEmojiStickerGroups(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetEmojiStickerGroups - reply: %s", r)
	return r, nil
}
