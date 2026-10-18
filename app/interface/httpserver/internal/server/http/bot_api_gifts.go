// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
)

func getAvailableGifts(ctx *svc.ServiceContext, r *http.Request, botID int64) (any, int, int, string) {
	if ctx == nil || ctx.GiftsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts service is not configured"
	}
	result, err := ctx.GiftsClient.PaymentsGetStarGifts(botRPCContext(r.Context(), botID), &mtproto.TLPaymentsGetStarGifts{})
	if err != nil || result == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return &botapi.Gifts{Gifts: botapi.StarGiftsFromMTProto(result)}, http.StatusOK, 0, ""
}

func convertGiftToStars(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.GiftsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts service is not configured"
	}
	// Layer 229 has no separate convert-gifts right; the closest available
	// business permission is the transfer/upgrade gift right.
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanTransferGifts })
	if code != 0 {
		return nil, status, code, description
	}
	stargift, err := botAPIOwnedGift(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	ok, err := ctx.GiftsClient.PaymentsConvertStarGift(businessBotCallContext(requestContext(r), botID, scope), &mtproto.TLPaymentsConvertStarGift{Stargift: stargift})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: gift conversion result was not returned"
	}
	if !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: gift conversion failed"
	}
	return true, http.StatusOK, 0, ""
}

func upgradeGift(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.GiftsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanTransferGifts })
	if code != 0 {
		return nil, status, code, description
	}
	stargift, err := botAPIOwnedGift(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	keepOriginalDetails, err := request.bool("keep_original_details")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: keep_original_details is invalid"
	}
	starCount, err := botAPIGiftStarCount(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if starCount > 0 {
		if !scope.CanTransferStars {
			return nil, http.StatusBadRequest, 400, "Bad Request: business connection does not allow Stars transfers"
		}
		if ctx.PaymentsClient == nil || ctx.StarsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
		}
		invoice := mtproto.MakeTLInputInvoiceStarGiftUpgrade(&mtproto.InputInvoice{
			KeepOriginalDetails: keepOriginalDetails,
			Stargift:            stargift,
		}).To_InputInvoice()
		if err = settleBotAPILayer229StarsInvoiceExpected(businessBotCallContext(requestContext(r), botID, scope), ctx.PaymentsClient, ctx.StarsClient, invoice, mtproto.Predicate_payments_paymentFormStars, starCount); err != nil {
			if paymentUnsupportedFromRPC(err) {
				return nil, http.StatusNotImplemented, 501, "Not Implemented: paid gift upgrades are not supported by the Layer 229 payment adapter"
			}
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
	}
	result, err := ctx.GiftsClient.PaymentsUpgradeStarGift(businessBotCallContext(requestContext(r), botID, scope), &mtproto.TLPaymentsUpgradeStarGift{
		KeepOriginalDetails: keepOriginalDetails,
		Stargift:            stargift,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: gift upgrade result was not returned"
	}
	return true, http.StatusOK, 0, ""
}

func transferGift(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.GiftsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanTransferGifts })
	if code != 0 {
		return nil, status, code, description
	}
	stargift, err := botAPIOwnedGift(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	starCount, err := botAPIGiftStarCount(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	targetRaw := strings.TrimSpace(request.string("new_owner_chat_id"))
	if targetRaw == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: new_owner_chat_id is required"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, scope.OwnerID, targetRaw)
	if err != nil || peerID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: new_owner_chat_id is invalid"
	}
	target, err := botAPIGiftTargetPeer(ctx, r.Context(), botID, scope.OwnerID, peerType, peerID)
	if err != nil || target == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: new_owner_chat_id is invalid"
	}
	if starCount > 0 {
		if !scope.CanTransferStars {
			return nil, http.StatusBadRequest, 400, "Bad Request: business connection does not allow Stars transfers"
		}
		if ctx.PaymentsClient == nil || ctx.StarsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
		}
		invoice := mtproto.MakeTLInputInvoiceStarGiftTransfer(&mtproto.InputInvoice{
			Stargift:       stargift,
			ToId_INPUTPEER: target,
		}).To_InputInvoice()
		if err = settleBotAPILayer229StarsInvoiceExpected(businessBotCallContext(requestContext(r), botID, scope), ctx.PaymentsClient, ctx.StarsClient, invoice, mtproto.Predicate_payments_paymentFormStars, starCount); err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
	}
	result, err := ctx.GiftsClient.PaymentsTransferStarGift(businessBotCallContext(requestContext(r), botID, scope), &mtproto.TLPaymentsTransferStarGift{
		Stargift:       stargift,
		ToId_INPUTPEER: target,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: gift transfer result was not returned"
	}
	return true, http.StatusOK, 0, ""
}

func botAPIGiftTargetPeer(ctx *svc.ServiceContext, requestCtx context.Context, botID, actorID int64, peerType int32, peerID int64) (*mtproto.InputPeer, error) {
	if peerID <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peerType != mtproto.PEER_USER {
		return botAPIInputPeer(actorID, peerType, peerID), nil
	}
	if peerID == actorID {
		return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), nil
	}
	if ctx == nil || ctx.UserClient == nil || botID <= 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := ctx.UserClient.UserGetImmutableUser(botRPCContext(requestCtx, botID), &userpb.TLUserGetImmutableUser{Id: peerID})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetId() != peerID || profile.GetUser().GetAccessHash() == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId:     peerID,
		AccessHash: profile.GetUser().GetAccessHash(),
	}).To_InputPeer(), nil
}

func botAPIOwnedGift(request *botAPIRequest) (*mtproto.InputSavedStarGift, error) {
	if request == nil {
		return nil, errors.New("owned_gift_id is required")
	}
	ownedGiftID := strings.TrimSpace(request.string("owned_gift_id"))
	if ownedGiftID == "" {
		return nil, errors.New("owned_gift_id is required")
	}
	if savedID, err := strconv.ParseInt(ownedGiftID, 10, 64); err == nil {
		if savedID <= 0 {
			return nil, errors.New("owned_gift_id is invalid")
		}
		return mtproto.MakeTLInputSavedStarGiftChat(&mtproto.InputSavedStarGift{
			Peer:    mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
			SavedId: savedID,
		}).To_InputSavedStarGift(), nil
	} else {
		for i := 0; i < len(ownedGiftID); i++ {
			if ownedGiftID[i] < '0' || ownedGiftID[i] > '9' {
				return mtproto.MakeTLInputSavedStarGiftSlug(&mtproto.InputSavedStarGift{Slug: ownedGiftID}).To_InputSavedStarGift(), nil
			}
		}
		return nil, errors.New("owned_gift_id is invalid")
	}
}

func botAPIGiftStarCount(request *botAPIRequest) (int64, error) {
	if request == nil || !request.has("star_count") {
		return 0, nil
	}
	starCount, err := request.int64("star_count")
	if err != nil || starCount < 0 {
		return 0, errors.New("star_count is invalid")
	}
	return starCount, nil
}
