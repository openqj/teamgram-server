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

func (s *Service) AccountGetTmpPassword(ctx context.Context, request *mtproto.TLAccountGetTmpPassword) (*mtproto.Account_TmpPassword, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetTmpPassword - request: %s", request)
	r, err := c.AccountGetTmpPassword(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetTmpPassword - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetBotShippingResults(ctx context.Context, request *mtproto.TLMessagesSetBotShippingResults) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetBotShippingResults - request: %s", request)
	r, err := c.MessagesSetBotShippingResults(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetBotShippingResults - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetBotPrecheckoutResults(ctx context.Context, request *mtproto.TLMessagesSetBotPrecheckoutResults) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetBotPrecheckoutResults - request: %s", request)
	r, err := c.MessagesSetBotPrecheckoutResults(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetBotPrecheckoutResults - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetPaymentForm(ctx context.Context, request *mtproto.TLPaymentsGetPaymentForm) (*mtproto.Payments_PaymentForm, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetPaymentForm - request: %s", request)
	r, err := c.PaymentsGetPaymentForm(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetPaymentForm - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetPaymentReceipt(ctx context.Context, request *mtproto.TLPaymentsGetPaymentReceipt) (*mtproto.Payments_PaymentReceipt, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetPaymentReceipt - request: %s", request)
	r, err := c.PaymentsGetPaymentReceipt(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetPaymentReceipt - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsValidateRequestedInfo(ctx context.Context, request *mtproto.TLPaymentsValidateRequestedInfo) (*mtproto.Payments_ValidatedRequestedInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsValidateRequestedInfo - request: %s", request)
	r, err := c.PaymentsValidateRequestedInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsValidateRequestedInfo - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsSendPaymentForm(ctx context.Context, request *mtproto.TLPaymentsSendPaymentForm) (*mtproto.Payments_PaymentResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsSendPaymentForm - request: %s", request)
	r, err := c.PaymentsSendPaymentForm(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsSendPaymentForm - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetSavedInfo(ctx context.Context, request *mtproto.TLPaymentsGetSavedInfo) (*mtproto.Payments_SavedInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetSavedInfo - request: %s", request)
	r, err := c.PaymentsGetSavedInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetSavedInfo - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsClearSavedInfo(ctx context.Context, request *mtproto.TLPaymentsClearSavedInfo) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsClearSavedInfo - request: %s", request)
	r, err := c.PaymentsClearSavedInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsClearSavedInfo - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetBankCardData(ctx context.Context, request *mtproto.TLPaymentsGetBankCardData) (*mtproto.Payments_BankCardData, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetBankCardData - request: %s", request)
	r, err := c.PaymentsGetBankCardData(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetBankCardData - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsExportInvoice(ctx context.Context, request *mtproto.TLPaymentsExportInvoice) (*mtproto.Payments_ExportedInvoice, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsExportInvoice - request: %s", request)
	r, err := c.PaymentsExportInvoice(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsExportInvoice - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsRequestRecurringPayment(ctx context.Context, request *mtproto.TLPaymentsRequestRecurringPayment) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsRequestRecurringPayment - request: %s", request)
	r, err := c.PaymentsRequestRecurringPayment(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsRequestRecurringPayment - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsRestorePlayMarketReceipt(ctx context.Context, request *mtproto.TLPaymentsRestorePlayMarketReceipt) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsRestorePlayMarketReceipt - request: %s", request)
	r, err := c.PaymentsRestorePlayMarketReceipt(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsRestorePlayMarketReceipt - reply: %s", r)
	return r, nil
}
