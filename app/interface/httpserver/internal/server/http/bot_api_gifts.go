// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
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
	userID, stargift, err := botAPIOwnedGift(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if userID != botID {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts are currently limited to the bot owner"
	}
	ok, err := ctx.GiftsClient.PaymentsConvertStarGift(botRPCContext(r.Context(), botID), &mtproto.TLPaymentsConvertStarGift{Stargift: stargift})
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
	userID, stargift, err := botAPIOwnedGift(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if userID != botID {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts are currently limited to the bot owner"
	}
	keepOriginalDetails, err := request.bool("keep_original_details")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: keep_original_details is invalid"
	}
	if starCount, err := botAPIGiftStarCount(request); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	} else if starCount > 0 {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: star_count is not supported by the gifts service"
	}
	result, err := ctx.GiftsClient.PaymentsUpgradeStarGift(botRPCContext(r.Context(), botID), &mtproto.TLPaymentsUpgradeStarGift{
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
	userID, stargift, err := botAPIOwnedGift(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if userID != botID {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts are currently limited to the bot owner"
	}
	if starCount, err := botAPIGiftStarCount(request); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	} else if starCount > 0 {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: star_count is not supported by the gifts service"
	}
	targetRaw := strings.TrimSpace(request.string("new_owner_chat_id"))
	if targetRaw == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: new_owner_chat_id is required"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, targetRaw)
	if err != nil || peerID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: new_owner_chat_id is invalid"
	}
	target := botAPIInputPeer(botID, peerType, peerID)
	if target == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: new_owner_chat_id is invalid"
	}
	result, err := ctx.GiftsClient.PaymentsTransferStarGift(botRPCContext(r.Context(), botID), &mtproto.TLPaymentsTransferStarGift{
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

func botAPIOwnedGift(request *botAPIRequest) (int64, *mtproto.InputSavedStarGift, error) {
	if request == nil {
		return 0, nil, errors.New("user_id and owned_gift_id are required")
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return 0, nil, errors.New("user_id is invalid")
	}
	ownedGiftID := strings.TrimSpace(request.string("owned_gift_id"))
	if ownedGiftID == "" {
		return 0, nil, errors.New("owned_gift_id is required")
	}
	return userID, mtproto.MakeTLInputSavedStarGiftSlug(&mtproto.InputSavedStarGift{Slug: ownedGiftID}).To_InputSavedStarGift(), nil
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
