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

func (s *Service) AccountUpdateEmojiStatus(ctx context.Context, request *mtproto.TLAccountUpdateEmojiStatus) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountUpdateEmojiStatus - request: %s", request)
	r, err := c.AccountUpdateEmojiStatus(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountUpdateEmojiStatus - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetDefaultEmojiStatuses(ctx context.Context, request *mtproto.TLAccountGetDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetDefaultEmojiStatuses - request: %s", request)
	r, err := c.AccountGetDefaultEmojiStatuses(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetDefaultEmojiStatuses - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetRecentEmojiStatuses(ctx context.Context, request *mtproto.TLAccountGetRecentEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetRecentEmojiStatuses - request: %s", request)
	r, err := c.AccountGetRecentEmojiStatuses(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetRecentEmojiStatuses - reply: %s", r)
	return r, nil
}

func (s *Service) AccountClearRecentEmojiStatuses(ctx context.Context, request *mtproto.TLAccountClearRecentEmojiStatuses) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountClearRecentEmojiStatuses - request: %s", request)
	r, err := c.AccountClearRecentEmojiStatuses(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountClearRecentEmojiStatuses - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetChannelDefaultEmojiStatuses(ctx context.Context, request *mtproto.TLAccountGetChannelDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetChannelDefaultEmojiStatuses - request: %s", request)
	r, err := c.AccountGetChannelDefaultEmojiStatuses(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetChannelDefaultEmojiStatuses - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetChannelRestrictedStatusEmojis(ctx context.Context, request *mtproto.TLAccountGetChannelRestrictedStatusEmojis) (*mtproto.EmojiList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetChannelRestrictedStatusEmojis - request: %s", request)
	r, err := c.AccountGetChannelRestrictedStatusEmojis(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetChannelRestrictedStatusEmojis - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetCollectibleEmojiStatuses(ctx context.Context, request *mtproto.TLAccountGetCollectibleEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetCollectibleEmojiStatuses - request: %s", request)
	r, err := c.AccountGetCollectibleEmojiStatuses(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetCollectibleEmojiStatuses - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsUpdateEmojiStatus(ctx context.Context, request *mtproto.TLChannelsUpdateEmojiStatus) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsUpdateEmojiStatus - request: %s", request)
	r, err := c.ChannelsUpdateEmojiStatus(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsUpdateEmojiStatus - reply: %s", r)
	return r, nil
}

func (s *Service) BotsUpdateUserEmojiStatus(ctx context.Context, request *mtproto.TLBotsUpdateUserEmojiStatus) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsUpdateUserEmojiStatus - request: %s", request)
	r, err := c.BotsUpdateUserEmojiStatus(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsUpdateUserEmojiStatus - reply: %s", r)
	return r, nil
}

func (s *Service) BotsToggleUserEmojiStatusPermission(ctx context.Context, request *mtproto.TLBotsToggleUserEmojiStatusPermission) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsToggleUserEmojiStatusPermission - request: %s", request)
	r, err := c.BotsToggleUserEmojiStatusPermission(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsToggleUserEmojiStatusPermission - reply: %s", r)
	return r, nil
}
