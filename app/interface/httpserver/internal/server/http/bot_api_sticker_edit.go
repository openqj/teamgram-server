// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func replaceStickerInSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	if _, err := botAPIStickerOwner(request, botID); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	if _, err := botAPIInputStickerSet(request.string("name")); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is invalid"
	}
	oldSticker, err := botAPIInputDocument(request.string("old_sticker"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: old_sticker is invalid"
	}
	newSticker, err := botAPIInputSticker(request.raw("sticker"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is invalid"
	}
	if _, err = ctx.StickersClient.StickersReplaceSticker(botRPCContext(r.Context(), botID), &mtproto.TLStickersReplaceSticker{
		Sticker:    oldSticker,
		NewSticker: newSticker,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setStickerEmojiList(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	emojis, err := botAPIStickerEmojiList(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	return changeSticker(ctx, r, botID, request, wrapperspb.String(strings.Join(emojis, " ")), nil, nil)
}

func setStickerKeywords(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	var keywords []string
	if request != nil && request.has("keywords") && string(request.raw("keywords")) != "null" {
		if err := request.json("keywords", &keywords); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: keywords is invalid"
		}
	}
	if len(keywords) > 20 {
		return nil, http.StatusBadRequest, 400, "Bad Request: keywords is invalid"
	}
	for _, keyword := range keywords {
		if strings.TrimSpace(keyword) == "" {
			return nil, http.StatusBadRequest, 400, "Bad Request: keywords is invalid"
		}
	}
	joined := strings.Join(keywords, ",")
	if len([]rune(joined)) > 64 {
		return nil, http.StatusBadRequest, 400, "Bad Request: keywords is invalid"
	}
	return changeSticker(ctx, r, botID, request, nil, nil, wrapperspb.String(joined))
}

func setStickerMaskPosition(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	var coords *mtproto.MaskCoords
	if request != nil && request.has("mask_position") && string(request.raw("mask_position")) != "null" {
		var position struct {
			Point  string  `json:"point"`
			XShift float64 `json:"x_shift"`
			YShift float64 `json:"y_shift"`
			Scale  float64 `json:"scale"`
		}
		if err := request.json("mask_position", &position); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: mask_position is invalid"
		}
		coords, _ = botAPIMaskCoordsValue(&botapi.MaskPosition{Point: position.Point, XShift: position.XShift, YShift: position.YShift, Scale: position.Scale})
		if coords == nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: mask_position is invalid"
		}
	}
	return changeSticker(ctx, r, botID, request, nil, coords, nil)
}

func changeSticker(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, emoji *wrapperspb.StringValue, coords *mtproto.MaskCoords, keywords *wrapperspb.StringValue) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	sticker, err := botAPIInputDocument(request.string("sticker"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is invalid"
	}
	if _, err = ctx.StickersClient.StickersChangeSticker(botRPCContext(r.Context(), botID), &mtproto.TLStickersChangeSticker{
		Sticker: sticker, Emoji: emoji, MaskCoords: coords, Keywords: keywords,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setStickerSetTitle(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	name := request.string("name")
	title := request.string("title")
	if name == "" || title == "" || len([]rune(title)) > 64 {
		return nil, http.StatusBadRequest, 400, "Bad Request: name or title is invalid"
	}
	set, err := botAPIInputStickerSet(name)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is invalid"
	}
	if _, err = ctx.StickersClient.StickersRenameStickerSet(botRPCContext(r.Context(), botID), &mtproto.TLStickersRenameStickerSet{Stickerset: set, Title: title}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setStickerSetThumbnail(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	if _, err := botAPIStickerOwner(request, botID); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	if r != nil && strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if _, _, err := r.FormFile("thumbnail"); err == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: multipart thumbnail upload is not supported by the Layer 229 sticker adapter"
		}
	}
	set, err := botAPIInputStickerSet(request.string("name"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is invalid"
	}
	if _, err = botAPIStickerFormat(request.string("format")); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: format is invalid"
	}
	var thumb *mtproto.InputDocument
	if request.has("thumbnail") && strings.TrimSpace(request.string("thumbnail")) != "" {
		thumb, err = botAPIInputDocument(request.string("thumbnail"))
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: thumbnail is invalid"
		}
	}
	if _, err = ctx.StickersClient.StickersSetStickerSetThumb(botRPCContext(r.Context(), botID), &mtproto.TLStickersSetStickerSetThumb{Stickerset: set, Thumb: thumb}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setCustomEmojiStickerSetThumbnail(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	set, err := botAPIInputStickerSet(request.string("name"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is invalid"
	}
	var documentID *wrapperspb.Int64Value
	if value := strings.TrimSpace(request.string("custom_emoji_id")); value != "" {
		id, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || id <= 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: custom_emoji_id is invalid"
		}
		documentID = wrapperspb.Int64(id)
	}
	if _, err = ctx.StickersClient.StickersSetStickerSetThumb(botRPCContext(r.Context(), botID), &mtproto.TLStickersSetStickerSetThumb{Stickerset: set, ThumbDocumentId: documentID}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func botAPIStickerEmojiList(request *botAPIRequest) ([]string, error) {
	var emojis []string
	if request == nil || request.json("emoji_list", &emojis) != nil || len(emojis) == 0 || len(emojis) > 20 {
		return nil, errors.New("emoji_list is invalid")
	}
	for _, emoji := range emojis {
		if strings.TrimSpace(emoji) == "" {
			return nil, errors.New("emoji_list is invalid")
		}
	}
	return emojis, nil
}

func botAPIStickerOwner(request *botAPIRequest, botID int64) (int64, error) {
	if request == nil {
		return 0, errors.New("user_id is invalid")
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 || botID <= 0 {
		return 0, errors.New("user_id is invalid")
	}
	return userID, nil
}
