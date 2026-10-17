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
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
)

var errBotAPIGiftFilterUnsupported = errors.New("gift filter is not supported by the Layer 229 ledger")

func botAPISavedGiftRequest(request *botAPIRequest) (*mtproto.TLPaymentsGetSavedStarGifts, error) {
	if request == nil {
		return nil, errors.New("gift request is required")
	}
	readBool := func(name string) (bool, error) {
		value, err := request.bool(name)
		if err != nil {
			return false, errors.New(name + " is invalid")
		}
		return value, nil
	}
	excludeUnsaved, err := readBool("exclude_unsaved")
	if err != nil {
		return nil, err
	}
	excludeSaved, err := readBool("exclude_saved")
	if err != nil {
		return nil, err
	}
	// The generated Layer 229 request exposes legacy filter names, but the
	// current Bot API uses the names below. None of these filters has an
	// equivalent that the ledger can evaluate authoritatively, so reject an
	// enabled value instead of silently returning an unfiltered page.
	for _, name := range []string{
		"exclude_unlimited",
		"exclude_unique",
		"sort_by_value",
		"sort_by_price",
		"exclude_upgradable",
		"exclude_unupgradable",
		"exclude_limited_upgradable",
		"exclude_limited_non_upgradable",
		"exclude_from_blockchain",
	} {
		value, err := readBool(name)
		if err != nil {
			return nil, err
		}
		if value {
			return nil, errBotAPIGiftFilterUnsupported
		}
	}
	limit := 100
	if request.has("limit") {
		limit, err = request.int("limit")
		if err != nil || limit < 1 || limit > 100 {
			return nil, errors.New("limit is invalid")
		}
	}
	return &mtproto.TLPaymentsGetSavedStarGifts{
		ExcludeUnsaved: excludeUnsaved,
		ExcludeSaved:   excludeSaved,
		Offset:         strings.TrimSpace(request.string("offset")),
		Limit:          int32(limit),
	}, nil
}

func getBusinessAccountGifts(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.GiftsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts service is not configured"
	}
	input, err := botAPISavedGiftRequest(request)
	if err != nil {
		if errors.Is(err, errBotAPIGiftFilterUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanViewGifts })
	if code != 0 {
		return nil, status, code, description
	}
	result, err := ctx.GiftsClient.PaymentsGetSavedStarGifts(businessBotCallContext(requestContext(r), botID, scope), input)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: gifts were not returned"
	}
	return botapi.OwnedGiftsFromSavedStarGifts(result), http.StatusOK, 0, ""
}

func getUserGifts(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.GiftsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: gifts service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	if userID != botID {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: user gifts are currently limited to the bot owner"
	}
	input, err := botAPISavedGiftRequest(request)
	if err != nil {
		if errors.Is(err, errBotAPIGiftFilterUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	input.Peer = mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	result, err := ctx.GiftsClient.PaymentsGetSavedStarGifts(botRPCContext(requestContext(r), botID), input)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: gifts were not returned"
	}
	return botapi.OwnedGiftsFromSavedStarGifts(result), http.StatusOK, 0, ""
}

func getChatGifts(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if _, _, _, err := resolveBotAPIChatID(requestContext(r), ctx, botID, request.string("chat_id")); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	return nil, http.StatusNotImplemented, 501, "Not Implemented: chat gift ownership is not available in the Layer 229 ledger"
}
