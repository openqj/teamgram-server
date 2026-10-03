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

func (s *Service) PaymentsGetStarsTopupOptions(ctx context.Context, request *mtproto.TLPaymentsGetStarsTopupOptions) (*mtproto.Vector_StarsTopupOption, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsTopupOptions - request: %s", request)
	r, err := c.PaymentsGetStarsTopupOptions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsTopupOptions - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarsStatus(ctx context.Context, request *mtproto.TLPaymentsGetStarsStatus) (*mtproto.Payments_StarsStatus, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsStatus - request: %s", request)
	r, err := c.PaymentsGetStarsStatus(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsStatus - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarsTransactions(ctx context.Context, request *mtproto.TLPaymentsGetStarsTransactions) (*mtproto.Payments_StarsStatus, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsTransactions - request: %s", request)
	r, err := c.PaymentsGetStarsTransactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsTransactions - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsSendStarsForm(ctx context.Context, request *mtproto.TLPaymentsSendStarsForm) (*mtproto.Payments_PaymentResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsSendStarsForm - request: %s", request)
	r, err := c.PaymentsSendStarsForm(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsSendStarsForm - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsRefundStarsCharge(ctx context.Context, request *mtproto.TLPaymentsRefundStarsCharge) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsRefundStarsCharge - request: %s", request)
	r, err := c.PaymentsRefundStarsCharge(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsRefundStarsCharge - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarsRevenueStats(ctx context.Context, request *mtproto.TLPaymentsGetStarsRevenueStats) (*mtproto.Payments_StarsRevenueStats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsRevenueStats - request: %s", request)
	r, err := c.PaymentsGetStarsRevenueStats(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsRevenueStats - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarsRevenueWithdrawalUrl(ctx context.Context, request *mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl) (*mtproto.Payments_StarsRevenueWithdrawalUrl, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsRevenueWithdrawalUrl - request: %s", request)
	r, err := c.PaymentsGetStarsRevenueWithdrawalUrl(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsRevenueWithdrawalUrl - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarsRevenueAdsAccountUrl(ctx context.Context, request *mtproto.TLPaymentsGetStarsRevenueAdsAccountUrl) (*mtproto.Payments_StarsRevenueAdsAccountUrl, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsRevenueAdsAccountUrl - request: %s", request)
	r, err := c.PaymentsGetStarsRevenueAdsAccountUrl(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsRevenueAdsAccountUrl - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarsTransactionsByID(ctx context.Context, request *mtproto.TLPaymentsGetStarsTransactionsByID) (*mtproto.Payments_StarsStatus, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsTransactionsByID - request: %s", request)
	r, err := c.PaymentsGetStarsTransactionsByID(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsTransactionsByID - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarsGiftOptions(ctx context.Context, request *mtproto.TLPaymentsGetStarsGiftOptions) (*mtproto.Vector_StarsGiftOption, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarsGiftOptions - request: %s", request)
	r, err := c.PaymentsGetStarsGiftOptions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarsGiftOptions - reply: %s", r)
	return r, nil
}
