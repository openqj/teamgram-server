// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
)

type botAPIAcceptedGiftTypes struct {
	UnlimitedGifts      bool `json:"unlimited_gifts"`
	LimitedGifts        bool `json:"limited_gifts"`
	UniqueGifts         bool `json:"unique_gifts"`
	PremiumSubscription bool `json:"premium_subscription"`
	GiftsFromChannels   bool `json:"gifts_from_channels"`
}

func parseBotAPIAcceptedGiftTypes(request *botAPIRequest) (*botAPIAcceptedGiftTypes, error) {
	if request == nil || !request.has("accepted_gift_types") {
		return nil, errors.New("accepted_gift_types is required")
	}
	raw := request.raw("accepted_gift_types")
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, errors.New("accepted_gift_types is invalid")
	}
	var accepted botAPIAcceptedGiftTypes
	if err := request.json("accepted_gift_types", &accepted); err != nil {
		return nil, errors.New("accepted_gift_types is invalid")
	}
	return &accepted, nil
}

func disallowedGiftsFromBotAPIAcceptedTypes(accepted *botAPIAcceptedGiftTypes) *mtproto.DisallowedGiftsSettings {
	if accepted == nil {
		return nil
	}
	return mtproto.MakeTLDisallowedGiftsSettings(&mtproto.DisallowedGiftsSettings{
		DisallowUnlimitedStargifts:    !accepted.UnlimitedGifts,
		DisallowLimitedStargifts:      !accepted.LimitedGifts,
		DisallowUniqueStargifts:       !accepted.UniqueGifts,
		DisallowPremiumGifts:          !accepted.PremiumSubscription,
		DisallowStargiftsFromChannels: !accepted.GiftsFromChannels,
	}).To_DisallowedGiftsSettings()
}

func setBusinessAccountGiftSettings(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.UserClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: user service is not configured"
	}
	showGiftButton, err := requiredBotAPIBool(request, "show_gift_button")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	accepted, err := parseBotAPIAcceptedGiftTypes(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanChangeGifts })
	if code != 0 {
		return nil, status, code, description
	}
	callCtx := businessBotCallContext(requestContext(r), botID, scope)
	settings, err := ctx.UserClient.UserGetGlobalPrivacySettings(callCtx, &userpb.TLUserGetGlobalPrivacySettings{UserId: scope.OwnerID})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if settings == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: global privacy settings were not returned"
	}
	settings.DisplayGiftsButton = showGiftButton
	settings.DisallowedGifts = disallowedGiftsFromBotAPIAcceptedTypes(accepted)
	saved, err := ctx.UserClient.UserSetGlobalPrivacySettings(callCtx, &userpb.TLUserSetGlobalPrivacySettings{
		UserId:   scope.OwnerID,
		Settings: settings,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if saved == nil || !mtproto.FromBool(saved) {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: global privacy settings write was not acknowledged"
	}
	return true, http.StatusOK, 0, ""
}

func requiredBotAPIBool(request *botAPIRequest, name string) (bool, error) {
	if request == nil || !request.has(name) {
		return false, errors.New(name + " is required")
	}
	value, err := request.bool(name)
	if err != nil {
		return false, errors.New(name + " is invalid")
	}
	return value, nil
}
