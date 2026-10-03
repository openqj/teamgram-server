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

func (s *Service) AccountUploadTheme(ctx context.Context, request *mtproto.TLAccountUploadTheme) (*mtproto.Document, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountUploadTheme - request: %s", request)
	r, err := c.AccountUploadTheme(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountUploadTheme - reply: %s", r)
	return r, nil
}

func (s *Service) AccountCreateTheme(ctx context.Context, request *mtproto.TLAccountCreateTheme) (*mtproto.Theme, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountCreateTheme - request: %s", request)
	r, err := c.AccountCreateTheme(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountCreateTheme - reply: %s", r)
	return r, nil
}

func (s *Service) AccountUpdateTheme(ctx context.Context, request *mtproto.TLAccountUpdateTheme) (*mtproto.Theme, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountUpdateTheme - request: %s", request)
	r, err := c.AccountUpdateTheme(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountUpdateTheme - reply: %s", r)
	return r, nil
}

func (s *Service) AccountSaveTheme(ctx context.Context, request *mtproto.TLAccountSaveTheme) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountSaveTheme - request: %s", request)
	r, err := c.AccountSaveTheme(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountSaveTheme - reply: %s", r)
	return r, nil
}

func (s *Service) AccountInstallTheme(ctx context.Context, request *mtproto.TLAccountInstallTheme) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountInstallTheme - request: %s", request)
	r, err := c.AccountInstallTheme(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountInstallTheme - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetTheme(ctx context.Context, request *mtproto.TLAccountGetTheme) (*mtproto.Theme, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetTheme - request: %s", request)
	r, err := c.AccountGetTheme(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetTheme - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetThemes(ctx context.Context, request *mtproto.TLAccountGetThemes) (*mtproto.Account_Themes, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetThemes - request: %s", request)
	r, err := c.AccountGetThemes(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetThemes - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetChatThemes(ctx context.Context, request *mtproto.TLAccountGetChatThemes) (*mtproto.Account_Themes, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetChatThemes - request: %s", request)
	r, err := c.AccountGetChatThemes(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetChatThemes - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetUniqueGiftChatThemes(ctx context.Context, request *mtproto.TLAccountGetUniqueGiftChatThemes) (*mtproto.Account_ChatThemes, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetUniqueGiftChatThemes - request: %s", request)
	r, err := c.AccountGetUniqueGiftChatThemes(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetUniqueGiftChatThemes - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetChatTheme(ctx context.Context, request *mtproto.TLMessagesSetChatTheme) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetChatTheme - request: %s", request)
	r, err := c.MessagesSetChatTheme(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetChatTheme - reply: %s", r)
	return r, nil
}
