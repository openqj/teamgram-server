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

func (s *Service) AccountCreateBusinessChatLink(ctx context.Context, request *mtproto.TLAccountCreateBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountCreateBusinessChatLink - request: %s", request)
	r, err := c.AccountCreateBusinessChatLink(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountCreateBusinessChatLink - reply: %s", r)
	return r, nil
}

func (s *Service) AccountEditBusinessChatLink(ctx context.Context, request *mtproto.TLAccountEditBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountEditBusinessChatLink - request: %s", request)
	r, err := c.AccountEditBusinessChatLink(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountEditBusinessChatLink - reply: %s", r)
	return r, nil
}

func (s *Service) AccountDeleteBusinessChatLink(ctx context.Context, request *mtproto.TLAccountDeleteBusinessChatLink) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountDeleteBusinessChatLink - request: %s", request)
	r, err := c.AccountDeleteBusinessChatLink(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountDeleteBusinessChatLink - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetBusinessChatLinks(ctx context.Context, request *mtproto.TLAccountGetBusinessChatLinks) (*mtproto.Account_BusinessChatLinks, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetBusinessChatLinks - request: %s", request)
	r, err := c.AccountGetBusinessChatLinks(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetBusinessChatLinks - reply: %s", r)
	return r, nil
}

func (s *Service) AccountResolveBusinessChatLink(ctx context.Context, request *mtproto.TLAccountResolveBusinessChatLink) (*mtproto.Account_ResolvedBusinessChatLinks, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountResolveBusinessChatLink - request: %s", request)
	r, err := c.AccountResolveBusinessChatLink(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountResolveBusinessChatLink - reply: %s", r)
	return r, nil
}
