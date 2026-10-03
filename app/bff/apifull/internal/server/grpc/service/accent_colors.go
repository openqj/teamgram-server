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

func (s *Service) AccountUpdateColor(ctx context.Context, request *mtproto.TLAccountUpdateColor) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountUpdateColor - request: %s", request)
	r, err := c.AccountUpdateColor(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountUpdateColor - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetDefaultBackgroundEmojis(ctx context.Context, request *mtproto.TLAccountGetDefaultBackgroundEmojis) (*mtproto.EmojiList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetDefaultBackgroundEmojis - request: %s", request)
	r, err := c.AccountGetDefaultBackgroundEmojis(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetDefaultBackgroundEmojis - reply: %s", r)
	return r, nil
}

func (s *Service) HelpGetPeerColors(ctx context.Context, request *mtproto.TLHelpGetPeerColors) (*mtproto.Help_PeerColors, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("HelpGetPeerColors - request: %s", request)
	r, err := c.HelpGetPeerColors(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("HelpGetPeerColors - reply: %s", r)
	return r, nil
}

func (s *Service) HelpGetPeerProfileColors(ctx context.Context, request *mtproto.TLHelpGetPeerProfileColors) (*mtproto.Help_PeerColors, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("HelpGetPeerProfileColors - request: %s", request)
	r, err := c.HelpGetPeerProfileColors(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("HelpGetPeerProfileColors - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsUpdateColor(ctx context.Context, request *mtproto.TLChannelsUpdateColor) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsUpdateColor - request: %s", request)
	r, err := c.ChannelsUpdateColor(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsUpdateColor - reply: %s", r)
	return r, nil
}
