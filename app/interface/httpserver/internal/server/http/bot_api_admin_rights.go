// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

func getMyDefaultAdministratorRights(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	forChannels, err := request.bool("for_channels")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: for_channels is invalid"
	}
	if ctx == nil || ctx.BotAPIStore == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: bot state store is not configured"
	}
	raw, found, err := ctx.BotAPIStore.GetDefaultAdministratorRights(requestCtx, botID, forChannels)
	if err != nil {
		return nil, http.StatusInternalServerError, 500, err.Error()
	}
	rights := &botapi.ChatAdministratorRights{}
	if found && len(raw) > 0 {
		if err := json.Unmarshal(raw, rights); err != nil {
			return nil, http.StatusInternalServerError, 500, "Internal Server Error: stored administrator rights are invalid"
		}
	}
	return rights, http.StatusOK, 0, ""
}

func setMyDefaultAdministratorRights(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	forChannels, err := request.bool("for_channels")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: for_channels is invalid"
	}
	if len(request.raw("rights")) == 0 || string(request.raw("rights")) == "null" {
		return nil, http.StatusBadRequest, 400, "Bad Request: rights is required"
	}
	var rights botapi.ChatAdministratorRights
	if err := request.json("rights", &rights); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: rights is invalid"
	}
	if ctx == nil || ctx.BotAdminRightClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: bot administrator rights service is not configured"
	}
	adminRights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		ChangeInfo:           rights.CanChangeInfo,
		PostMessages:         rights.CanPostMessages,
		EditMessages:         rights.CanEditMessages,
		DeleteMessages:       rights.CanDeleteMessages,
		BanUsers:             rights.CanRestrictMembers,
		InviteUsers:          rights.CanInviteUsers,
		PinMessages:          rights.CanPinMessages,
		AddAdmins:            rights.CanPromoteMembers,
		Anonymous:            rights.IsAnonymous,
		ManageCall:           rights.CanManageVideoChats,
		Other:                rights.CanManageChat,
		ManageTopics:         rights.CanManageTopics,
		PostStories:          rights.CanPostStories,
		EditStories:          rights.CanEditStories,
		DeleteStories:        rights.CanDeleteStories,
		ManageDirectMessages: rights.CanManageDirectMessages,
	}).To_ChatAdminRights()
	var ok *mtproto.Bool
	if forChannels {
		ok, err = ctx.BotAdminRightClient.BotsSetBotBroadcastDefaultAdminRights(botRPCContext(requestCtx, botID), &mtproto.TLBotsSetBotBroadcastDefaultAdminRights{AdminRights: adminRights})
	} else {
		ok, err = ctx.BotAdminRightClient.BotsSetBotGroupDefaultAdminRights(botRPCContext(requestCtx, botID), &mtproto.TLBotsSetBotGroupDefaultAdminRights{AdminRights: adminRights})
	}
	if err != nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ctx.BotAPIStore == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: bot state store is not configured"
	}
	raw, err := json.Marshal(rights)
	if err != nil {
		return nil, http.StatusInternalServerError, 500, err.Error()
	}
	if err := ctx.BotAPIStore.SetDefaultAdministratorRights(requestCtx, botID, forChannels, raw); err != nil {
		return nil, http.StatusInternalServerError, 500, err.Error()
	}
	return true, http.StatusOK, 0, ""
}
