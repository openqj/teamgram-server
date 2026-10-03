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

func (s *Service) BotsUpdateStarRefProgram(ctx context.Context, request *mtproto.TLBotsUpdateStarRefProgram) (*mtproto.StarRefProgram, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("BotsUpdateStarRefProgram - request: %s", request)
	r, err := c.BotsUpdateStarRefProgram(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("BotsUpdateStarRefProgram - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetConnectedStarRefBots(ctx context.Context, request *mtproto.TLPaymentsGetConnectedStarRefBots) (*mtproto.Payments_ConnectedStarRefBots, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetConnectedStarRefBots - request: %s", request)
	r, err := c.PaymentsGetConnectedStarRefBots(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetConnectedStarRefBots - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetConnectedStarRefBot(ctx context.Context, request *mtproto.TLPaymentsGetConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetConnectedStarRefBot - request: %s", request)
	r, err := c.PaymentsGetConnectedStarRefBot(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetConnectedStarRefBot - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetSuggestedStarRefBots(ctx context.Context, request *mtproto.TLPaymentsGetSuggestedStarRefBots) (*mtproto.Payments_SuggestedStarRefBots, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetSuggestedStarRefBots - request: %s", request)
	r, err := c.PaymentsGetSuggestedStarRefBots(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetSuggestedStarRefBots - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsConnectStarRefBot(ctx context.Context, request *mtproto.TLPaymentsConnectStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsConnectStarRefBot - request: %s", request)
	r, err := c.PaymentsConnectStarRefBot(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsConnectStarRefBot - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsEditConnectedStarRefBot(ctx context.Context, request *mtproto.TLPaymentsEditConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsEditConnectedStarRefBot - request: %s", request)
	r, err := c.PaymentsEditConnectedStarRefBot(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsEditConnectedStarRefBot - reply: %s", r)
	return r, nil
}
