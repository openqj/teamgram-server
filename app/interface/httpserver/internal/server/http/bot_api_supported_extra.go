// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func setUserEmojiStatus(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.EmojiStatusClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: emoji status service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}

	customEmojiID := strings.TrimSpace(request.string("emoji_status_custom_emoji_id"))
	status := mtproto.MakeTLEmojiStatusEmpty(nil).To_EmojiStatus()
	if customEmojiID != "" {
		documentID, parseErr := strconv.ParseInt(customEmojiID, 10, 64)
		if parseErr != nil || documentID <= 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: emoji_status_custom_emoji_id is invalid"
		}
		expiration, parseErr := request.int64("emoji_status_expiration_date")
		if parseErr != nil || expiration < 0 || expiration > int64(^uint32(0)>>1) {
			return nil, http.StatusBadRequest, 400, "Bad Request: emoji_status_expiration_date is invalid"
		}
		var until *wrapperspb.Int32Value
		if expiration > 0 {
			until = wrapperspb.Int32(int32(expiration))
		}
		status = mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{
			DocumentId:      documentID,
			Until_FLAGINT32: until,
		}).To_EmojiStatus()
	} else if request.has("emoji_status_expiration_date") {
		return nil, http.StatusBadRequest, 400, "Bad Request: emoji_status_expiration_date requires emoji_status_custom_emoji_id"
	}

	ok, err := ctx.EmojiStatusClient.BotsUpdateUserEmojiStatus(botRPCContext(r.Context(), botID), &mtproto.TLBotsUpdateUserEmojiStatus{
		UserId:      inputUser,
		EmojiStatus: status,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: emoji status update failed"
	}
	return true, http.StatusOK, 0, ""
}

func setChatStickerSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a supergroup"
	}
	name := strings.TrimSpace(request.string("sticker_set_name"))
	if name == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker_set_name is required"
	}
	set, err := botAPIInputStickerSet(name)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker_set_name is invalid"
	}
	ok, err := ctx.ChannelsClient.ChannelsSetStickers(botRPCContext(r.Context(), botID), &mtproto.TLChannelsSetStickers{
		Channel:    botAPIForumChannel(peerID),
		Stickerset: set,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker set was not updated"
	}
	return true, http.StatusOK, 0, ""
}

func deleteChatStickerSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a supergroup"
	}
	ok, err := ctx.ChannelsClient.ChannelsSetStickers(botRPCContext(r.Context(), botID), &mtproto.TLChannelsSetStickers{
		Channel:    botAPIForumChannel(peerID),
		Stickerset: mtproto.MakeTLInputStickerSetEmpty(nil).To_InputStickerSet(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker set was not removed"
	}
	return true, http.StatusOK, 0, ""
}
