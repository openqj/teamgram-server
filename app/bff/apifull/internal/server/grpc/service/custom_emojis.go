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

func (s *Service) AccountGetDefaultProfilePhotoEmojis(ctx context.Context, request *mtproto.TLAccountGetDefaultProfilePhotoEmojis) (*mtproto.EmojiList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetDefaultProfilePhotoEmojis - request: %s", request)
	r, err := c.AccountGetDefaultProfilePhotoEmojis(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetDefaultProfilePhotoEmojis - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetDefaultGroupPhotoEmojis(ctx context.Context, request *mtproto.TLAccountGetDefaultGroupPhotoEmojis) (*mtproto.EmojiList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetDefaultGroupPhotoEmojis - request: %s", request)
	r, err := c.AccountGetDefaultGroupPhotoEmojis(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetDefaultGroupPhotoEmojis - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetCustomEmojiDocuments(ctx context.Context, request *mtproto.TLMessagesGetCustomEmojiDocuments) (*mtproto.Vector_Document, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetCustomEmojiDocuments - request: %s", request)
	r, err := c.MessagesGetCustomEmojiDocuments(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetCustomEmojiDocuments - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetEmojiStickers(ctx context.Context, request *mtproto.TLMessagesGetEmojiStickers) (*mtproto.Messages_AllStickers, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetEmojiStickers - request: %s", request)
	r, err := c.MessagesGetEmojiStickers(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetEmojiStickers - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetFeaturedEmojiStickers(ctx context.Context, request *mtproto.TLMessagesGetFeaturedEmojiStickers) (*mtproto.Messages_FeaturedStickers, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetFeaturedEmojiStickers - request: %s", request)
	r, err := c.MessagesGetFeaturedEmojiStickers(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetFeaturedEmojiStickers - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSearchCustomEmoji(ctx context.Context, request *mtproto.TLMessagesSearchCustomEmoji) (*mtproto.EmojiList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSearchCustomEmoji - request: %s", request)
	r, err := c.MessagesSearchCustomEmoji(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSearchCustomEmoji - reply: %s", r)
	return r, nil
}
