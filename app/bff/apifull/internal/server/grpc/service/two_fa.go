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

func (s *Service) AccountGetPassword(ctx context.Context, request *mtproto.TLAccountGetPassword) (*mtproto.Account_Password, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetPassword - request: %s", request)
	r, err := c.AccountGetPassword(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetPassword - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetPasswordSettings(ctx context.Context, request *mtproto.TLAccountGetPasswordSettings) (*mtproto.Account_PasswordSettings, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetPasswordSettings - request: %s", request)
	r, err := c.AccountGetPasswordSettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetPasswordSettings - reply: %s", r)
	return r, nil
}

func (s *Service) AccountUpdatePasswordSettings(ctx context.Context, request *mtproto.TLAccountUpdatePasswordSettings) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountUpdatePasswordSettings - request: %s", request)
	r, err := c.AccountUpdatePasswordSettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountUpdatePasswordSettings - reply: %s", r)
	return r, nil
}

func (s *Service) AccountConfirmPasswordEmail(ctx context.Context, request *mtproto.TLAccountConfirmPasswordEmail) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountConfirmPasswordEmail - request: %s", request)
	r, err := c.AccountConfirmPasswordEmail(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountConfirmPasswordEmail - reply: %s", r)
	return r, nil
}

func (s *Service) AccountResendPasswordEmail(ctx context.Context, request *mtproto.TLAccountResendPasswordEmail) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountResendPasswordEmail - request: %s", request)
	r, err := c.AccountResendPasswordEmail(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountResendPasswordEmail - reply: %s", r)
	return r, nil
}

func (s *Service) AccountCancelPasswordEmail(ctx context.Context, request *mtproto.TLAccountCancelPasswordEmail) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountCancelPasswordEmail - request: %s", request)
	r, err := c.AccountCancelPasswordEmail(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountCancelPasswordEmail - reply: %s", r)
	return r, nil
}

func (s *Service) AccountDeclinePasswordReset(ctx context.Context, request *mtproto.TLAccountDeclinePasswordReset) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountDeclinePasswordReset - request: %s", request)
	r, err := c.AccountDeclinePasswordReset(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountDeclinePasswordReset - reply: %s", r)
	return r, nil
}
