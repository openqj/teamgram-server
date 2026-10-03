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

func (s *Service) AccountGetWebAuthorizations(ctx context.Context, request *mtproto.TLAccountGetWebAuthorizations) (*mtproto.Account_WebAuthorizations, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetWebAuthorizations - request: %s", request)
	r, err := c.AccountGetWebAuthorizations(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetWebAuthorizations - reply: %s", r)
	return r, nil
}

func (s *Service) AccountResetWebAuthorization(ctx context.Context, request *mtproto.TLAccountResetWebAuthorization) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountResetWebAuthorization - request: %s", request)
	r, err := c.AccountResetWebAuthorization(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountResetWebAuthorization - reply: %s", r)
	return r, nil
}

func (s *Service) AccountResetWebAuthorizations(ctx context.Context, request *mtproto.TLAccountResetWebAuthorizations) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountResetWebAuthorizations - request: %s", request)
	r, err := c.AccountResetWebAuthorizations(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountResetWebAuthorizations - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestUrlAuth(ctx context.Context, request *mtproto.TLMessagesRequestUrlAuth) (*mtproto.UrlAuthResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestUrlAuth - request: %s", request)
	r, err := c.MessagesRequestUrlAuth(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestUrlAuth - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesAcceptUrlAuth(ctx context.Context, request *mtproto.TLMessagesAcceptUrlAuth) (*mtproto.UrlAuthResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesAcceptUrlAuth - request: %s", request)
	r, err := c.MessagesAcceptUrlAuth(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesAcceptUrlAuth - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeclineUrlAuth(ctx context.Context, request *mtproto.TLMessagesDeclineUrlAuth) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeclineUrlAuth - request: %s", request)
	r, err := c.MessagesDeclineUrlAuth(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeclineUrlAuth - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesCheckUrlAuthMatchCode(ctx context.Context, request *mtproto.TLMessagesCheckUrlAuthMatchCode) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesCheckUrlAuthMatchCode - request: %s", request)
	r, err := c.MessagesCheckUrlAuthMatchCode(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesCheckUrlAuthMatchCode - reply: %s", r)
	return r, nil
}
