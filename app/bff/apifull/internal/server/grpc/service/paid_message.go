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

func (s *Service) AccountGetPaidMessagesRevenue(ctx context.Context, request *mtproto.TLAccountGetPaidMessagesRevenue) (*mtproto.Account_PaidMessagesRevenue, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetPaidMessagesRevenue - request: %s", request)
	r, err := c.AccountGetPaidMessagesRevenue(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetPaidMessagesRevenue - reply: %s", r)
	return r, nil
}

func (s *Service) AccountToggleNoPaidMessagesException(ctx context.Context, request *mtproto.TLAccountToggleNoPaidMessagesException) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountToggleNoPaidMessagesException - request: %s", request)
	r, err := c.AccountToggleNoPaidMessagesException(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountToggleNoPaidMessagesException - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsUpdatePaidMessagesPrice(ctx context.Context, request *mtproto.TLChannelsUpdatePaidMessagesPrice) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsUpdatePaidMessagesPrice - request: %s", request)
	r, err := c.ChannelsUpdatePaidMessagesPrice(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsUpdatePaidMessagesPrice - reply: %s", r)
	return r, nil
}

func (s *Service) AccountAddNoPaidMessagesException(ctx context.Context, request *mtproto.TLAccountAddNoPaidMessagesException) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountAddNoPaidMessagesException - request: %s", request)
	r, err := c.AccountAddNoPaidMessagesException(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountAddNoPaidMessagesException - reply: %s", r)
	return r, nil
}
