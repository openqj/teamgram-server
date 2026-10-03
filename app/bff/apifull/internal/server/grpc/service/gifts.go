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

func (s *Service) PaymentsGetStarGifts(ctx context.Context, request *mtproto.TLPaymentsGetStarGifts) (*mtproto.Payments_StarGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGifts - request: %s", request)
	r, err := c.PaymentsGetStarGifts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGifts - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsSaveStarGift(ctx context.Context, request *mtproto.TLPaymentsSaveStarGift) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsSaveStarGift - request: %s", request)
	r, err := c.PaymentsSaveStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsSaveStarGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsConvertStarGift(ctx context.Context, request *mtproto.TLPaymentsConvertStarGift) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsConvertStarGift - request: %s", request)
	r, err := c.PaymentsConvertStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsConvertStarGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarGiftUpgradePreview(ctx context.Context, request *mtproto.TLPaymentsGetStarGiftUpgradePreview) (*mtproto.Payments_StarGiftUpgradePreview, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGiftUpgradePreview - request: %s", request)
	r, err := c.PaymentsGetStarGiftUpgradePreview(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGiftUpgradePreview - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsUpgradeStarGift(ctx context.Context, request *mtproto.TLPaymentsUpgradeStarGift) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsUpgradeStarGift - request: %s", request)
	r, err := c.PaymentsUpgradeStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsUpgradeStarGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsTransferStarGift(ctx context.Context, request *mtproto.TLPaymentsTransferStarGift) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsTransferStarGift - request: %s", request)
	r, err := c.PaymentsTransferStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsTransferStarGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetUniqueStarGift(ctx context.Context, request *mtproto.TLPaymentsGetUniqueStarGift) (*mtproto.Payments_UniqueStarGift, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetUniqueStarGift - request: %s", request)
	r, err := c.PaymentsGetUniqueStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetUniqueStarGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetSavedStarGifts(ctx context.Context, request *mtproto.TLPaymentsGetSavedStarGifts) (*mtproto.Payments_SavedStarGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetSavedStarGifts - request: %s", request)
	r, err := c.PaymentsGetSavedStarGifts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetSavedStarGifts - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetSavedStarGift(ctx context.Context, request *mtproto.TLPaymentsGetSavedStarGift) (*mtproto.Payments_SavedStarGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetSavedStarGift - request: %s", request)
	r, err := c.PaymentsGetSavedStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetSavedStarGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarGiftWithdrawalUrl(ctx context.Context, request *mtproto.TLPaymentsGetStarGiftWithdrawalUrl) (*mtproto.Payments_StarGiftWithdrawalUrl, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGiftWithdrawalUrl - request: %s", request)
	r, err := c.PaymentsGetStarGiftWithdrawalUrl(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGiftWithdrawalUrl - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsToggleChatStarGiftNotifications(ctx context.Context, request *mtproto.TLPaymentsToggleChatStarGiftNotifications) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsToggleChatStarGiftNotifications - request: %s", request)
	r, err := c.PaymentsToggleChatStarGiftNotifications(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsToggleChatStarGiftNotifications - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsToggleStarGiftsPinnedToTop(ctx context.Context, request *mtproto.TLPaymentsToggleStarGiftsPinnedToTop) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsToggleStarGiftsPinnedToTop - request: %s", request)
	r, err := c.PaymentsToggleStarGiftsPinnedToTop(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsToggleStarGiftsPinnedToTop - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetResaleStarGifts(ctx context.Context, request *mtproto.TLPaymentsGetResaleStarGifts) (*mtproto.Payments_ResaleStarGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetResaleStarGifts - request: %s", request)
	r, err := c.PaymentsGetResaleStarGifts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetResaleStarGifts - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsUpdateStarGiftPrice(ctx context.Context, request *mtproto.TLPaymentsUpdateStarGiftPrice) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsUpdateStarGiftPrice - request: %s", request)
	r, err := c.PaymentsUpdateStarGiftPrice(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsUpdateStarGiftPrice - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetUniqueStarGiftValueInfo(ctx context.Context, request *mtproto.TLPaymentsGetUniqueStarGiftValueInfo) (*mtproto.Payments_UniqueStarGiftValueInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetUniqueStarGiftValueInfo - request: %s", request)
	r, err := c.PaymentsGetUniqueStarGiftValueInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetUniqueStarGiftValueInfo - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsCheckCanSendGift(ctx context.Context, request *mtproto.TLPaymentsCheckCanSendGift) (*mtproto.Payments_CheckCanSendGiftResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsCheckCanSendGift - request: %s", request)
	r, err := c.PaymentsCheckCanSendGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsCheckCanSendGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarGiftAuctionState(ctx context.Context, request *mtproto.TLPaymentsGetStarGiftAuctionState) (*mtproto.Payments_StarGiftAuctionState, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGiftAuctionState - request: %s", request)
	r, err := c.PaymentsGetStarGiftAuctionState(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGiftAuctionState - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarGiftAuctionAcquiredGifts(ctx context.Context, request *mtproto.TLPaymentsGetStarGiftAuctionAcquiredGifts) (*mtproto.Payments_StarGiftAuctionAcquiredGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGiftAuctionAcquiredGifts - request: %s", request)
	r, err := c.PaymentsGetStarGiftAuctionAcquiredGifts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGiftAuctionAcquiredGifts - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarGiftActiveAuctions(ctx context.Context, request *mtproto.TLPaymentsGetStarGiftActiveAuctions) (*mtproto.Payments_StarGiftActiveAuctions, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGiftActiveAuctions - request: %s", request)
	r, err := c.PaymentsGetStarGiftActiveAuctions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGiftActiveAuctions - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsResolveStarGiftOffer(ctx context.Context, request *mtproto.TLPaymentsResolveStarGiftOffer) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsResolveStarGiftOffer - request: %s", request)
	r, err := c.PaymentsResolveStarGiftOffer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsResolveStarGiftOffer - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsSendStarGiftOffer(ctx context.Context, request *mtproto.TLPaymentsSendStarGiftOffer) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsSendStarGiftOffer - request: %s", request)
	r, err := c.PaymentsSendStarGiftOffer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsSendStarGiftOffer - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarGiftUpgradeAttributes(ctx context.Context, request *mtproto.TLPaymentsGetStarGiftUpgradeAttributes) (*mtproto.Payments_StarGiftUpgradeAttributes, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGiftUpgradeAttributes - request: %s", request)
	r, err := c.PaymentsGetStarGiftUpgradeAttributes(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGiftUpgradeAttributes - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetCraftStarGifts(ctx context.Context, request *mtproto.TLPaymentsGetCraftStarGifts) (*mtproto.Payments_SavedStarGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetCraftStarGifts - request: %s", request)
	r, err := c.PaymentsGetCraftStarGifts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetCraftStarGifts - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsCraftStarGift(ctx context.Context, request *mtproto.TLPaymentsCraftStarGift) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsCraftStarGift - request: %s", request)
	r, err := c.PaymentsCraftStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsCraftStarGift - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetUserStarGifts(ctx context.Context, request *mtproto.TLPaymentsGetUserStarGifts) (*mtproto.Payments_UserStarGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetUserStarGifts - request: %s", request)
	r, err := c.PaymentsGetUserStarGifts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetUserStarGifts - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetUserStarGift(ctx context.Context, request *mtproto.TLPaymentsGetUserStarGift) (*mtproto.Payments_UserStarGifts, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetUserStarGift - request: %s", request)
	r, err := c.PaymentsGetUserStarGift(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetUserStarGift - reply: %s", r)
	return r, nil
}
