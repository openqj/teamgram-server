// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// botAPIStickerSet is kept local until the shared botapi package exposes the
// complete StickerSet response type. Its JSON shape matches Telegram's Bot API.
type botAPIStickerSet struct {
	Name          string           `json:"name"`
	Title         string           `json:"title"`
	StickerType   string           `json:"sticker_type"`
	IsAnimated    bool             `json:"is_animated"`
	IsVideo       bool             `json:"is_video"`
	ContainsMasks bool             `json:"contains_masks"`
	Stickers      []botapi.Sticker `json:"stickers"`
}

type botAPIInputStickerPayload struct {
	Sticker      string               `json:"sticker"`
	Format       string               `json:"format"`
	EmojiList    []string             `json:"emoji_list"`
	MaskPosition *botapi.MaskPosition `json:"mask_position,omitempty"`
	Keywords     []string             `json:"keywords,omitempty"`
}

func getStickerSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	inputSet, err := botAPIInputStickerSet(request.string("name"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is invalid"
	}
	result, err := ctx.StickersClient.MessagesGetStickerSet(botRPCContext(r.Context(), botID), &mtproto.TLMessagesGetStickerSet{
		Stickerset: inputSet,
	})
	if err != nil || result == nil || result.GetSet() == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botAPIStickerSetFromMTProto(result), http.StatusOK, 0, ""
}

func createNewStickerSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	name, title := request.string("name"), request.string("title")
	if name == "" || title == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: name and title are required"
	}
	stickers, format, err := botAPIInputStickers(request)
	if err != nil || len(stickers) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: stickers is invalid"
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	stickerType, err := botAPIStickerType(request.string("sticker_type"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker_type is invalid"
	}
	needsRepainting, err := request.bool("needs_repainting")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: needs_repainting is invalid"
	}
	result, err := ctx.StickersClient.StickersCreateStickerSet(botRPCContext(r.Context(), botID), &mtproto.TLStickersCreateStickerSet{
		Masks:     stickerType == "mask",
		Emojis:    stickerType == "custom_emoji",
		TextColor: needsRepainting,
		UserId:    inputUser,
		Title:     title,
		ShortName: name,
		Stickers:  stickers,
		Animated:  format == "animated",
		Videos:    format == "video",
	})
	if err != nil || result == nil || result.GetSet() == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func addStickerToSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	inputSet, err := botAPIInputStickerSet(request.string("name"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is invalid"
	}
	sticker, err := botAPIInputSticker(request.raw("sticker"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is invalid"
	}
	if _, err = ctx.StickersClient.StickersAddStickerToSet(botRPCContext(r.Context(), botID), &mtproto.TLStickersAddStickerToSet{Stickerset: inputSet, Sticker: sticker}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setStickerPositionInSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	sticker, err := botAPIInputDocument(request.string("sticker"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is invalid"
	}
	position, err := request.int("position")
	if err != nil || position < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: position is invalid"
	}
	if _, err = ctx.StickersClient.StickersChangeStickerPosition(botRPCContext(r.Context(), botID), &mtproto.TLStickersChangeStickerPosition{Sticker: sticker, Position: int32(position)}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteStickerFromSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	sticker, err := botAPIInputDocument(request.string("sticker"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is invalid"
	}
	if _, err = ctx.StickersClient.StickersRemoveStickerFromSet(botRPCContext(r.Context(), botID), &mtproto.TLStickersRemoveStickerFromSet{Sticker: sticker}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteStickerSet(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StickersClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: sticker service is not configured"
	}
	inputSet, err := botAPIInputStickerSet(request.string("name"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is invalid"
	}
	ok, err := ctx.StickersClient.StickersDeleteStickerSet(botRPCContext(r.Context(), botID), &mtproto.TLStickersDeleteStickerSet{Stickerset: inputSet})
	if err != nil || ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func botAPIInputStickerSet(name string) (*mtproto.InputStickerSet, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("sticker set name is required")
	}
	return mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: name}).To_InputStickerSet(), nil
}

func botAPIInputDocument(fileID string) (*mtproto.InputDocument, error) {
	ref, err := parseBotAPIFileID(fileID)
	if err != nil || ref.kind != "document" {
		return nil, errors.New("sticker file_id is invalid")
	}
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: ref.id, AccessHash: ref.accessHash}).To_InputDocument(), nil
}

func botAPIInputStickers(request *botAPIRequest) ([]*mtproto.InputStickerSetItem, string, error) {
	raw := request.raw("stickers")
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "", errors.New("stickers is required")
	}
	var inputs []botAPIInputStickerPayload
	if err := request.json("stickers", &inputs); err != nil {
		return nil, "", err
	}
	result := make([]*mtproto.InputStickerSetItem, 0, len(inputs))
	format := ""
	for _, input := range inputs {
		item, err := botAPIInputStickerValue(input)
		if err != nil {
			return nil, "", err
		}
		itemFormat, formatErr := botAPIStickerFormat(input.Format)
		if formatErr != nil {
			return nil, "", formatErr
		}
		if format == "" {
			format = itemFormat
		} else if format != itemFormat {
			return nil, "", errors.New("stickers must use one format")
		}
		result = append(result, item)
	}
	return result, format, nil
}

func botAPIInputSticker(raw []byte) (*mtproto.InputStickerSetItem, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("sticker is required")
	}
	var input botAPIInputStickerPayload
	if err := jsonUnmarshal(raw, &input); err != nil {
		return nil, err
	}
	if _, err := botAPIStickerFormat(input.Format); err != nil {
		return nil, err
	}
	return botAPIInputStickerValue(input)
}

func botAPIInputStickerValue(input botAPIInputStickerPayload) (*mtproto.InputStickerSetItem, error) {
	document, err := botAPIInputDocument(input.Sticker)
	if err != nil {
		return nil, err
	}
	if len(input.EmojiList) == 0 {
		return nil, errors.New("emoji_list is required")
	}
	for _, emoji := range input.EmojiList {
		if strings.TrimSpace(emoji) == "" {
			return nil, errors.New("emoji_list contains an empty value")
		}
	}
	item := &mtproto.InputStickerSetItem{Document: document, Emoji: strings.Join(input.EmojiList, " ")}
	if len(input.Keywords) > 0 {
		item.Keywords = wrapperspb.String(strings.Join(input.Keywords, ","))
	}
	if input.MaskPosition != nil {
		coords, err := botAPIMaskCoordsValue(input.MaskPosition)
		if err != nil {
			return nil, err
		}
		item.MaskCoords = coords
	}
	return item, nil
}

func botAPIMaskCoordsValue(position *botapi.MaskPosition) (*mtproto.MaskCoords, error) {
	if position == nil || position.Scale <= 0 {
		return nil, errors.New("mask_position is invalid")
	}
	point := int32(-1)
	switch strings.ToLower(strings.TrimSpace(position.Point)) {
	case "forehead":
		point = 0
	case "eyes":
		point = 1
	case "mouth":
		point = 2
	case "chin":
		point = 3
	}
	if point < 0 {
		return nil, errors.New("mask_position point is invalid")
	}
	return mtproto.MakeTLMaskCoords(&mtproto.MaskCoords{N: point, X: position.XShift, Y: position.YShift, Zoom: position.Scale}).To_MaskCoords(), nil
}

func botAPIStickerType(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "regular", nil
	}
	switch value {
	case "regular", "mask", "custom_emoji":
		return value, nil
	default:
		return "", errors.New("invalid sticker type")
	}
}

func botAPIStickerFormat(values ...string) (string, error) {
	value := ""
	for _, candidate := range values {
		if strings.TrimSpace(candidate) != "" {
			value = strings.TrimSpace(candidate)
			break
		}
	}
	if value == "" {
		return "static", nil
	}
	switch value {
	case "static", "animated", "video":
		return value, nil
	default:
		return "", errors.New("invalid sticker format")
	}
}

func botAPIStickerSetFromMTProto(result *mtproto.Messages_StickerSet) *botAPIStickerSet {
	set := result.GetSet()
	response := &botAPIStickerSet{
		Name:          set.GetShortName(),
		Title:         set.GetTitle(),
		StickerType:   "regular",
		IsAnimated:    set.GetAnimated(),
		IsVideo:       set.GetVideos(),
		ContainsMasks: set.GetMasks(),
		Stickers:      make([]botapi.Sticker, 0, len(result.GetDocuments())),
	}
	if set.GetMasks() {
		response.StickerType = "mask"
	} else if set.GetEmojis() {
		response.StickerType = "custom_emoji"
	}
	packEmoji := make(map[int64]string)
	for _, pack := range result.GetPacks() {
		if pack == nil {
			continue
		}
		for _, id := range pack.GetDocuments() {
			packEmoji[id] = pack.GetEmoticon()
		}
	}
	for _, document := range result.GetDocuments() {
		if sticker := botAPIStickerFromDocument(document, set.GetShortName(), packEmoji[document.GetId()]); sticker != nil {
			response.Stickers = append(response.Stickers, *sticker)
		}
	}
	return response
}

func botAPIStickerFromDocument(document *mtproto.Document, setName, packEmoji string) *botapi.Sticker {
	if document == nil {
		return nil
	}
	result := &botapi.Sticker{
		FileID:       "document_" + strconv.FormatInt(document.GetId(), 10) + "_" + strconv.FormatInt(document.GetAccessHash(), 10),
		FileUniqueID: strconv.FormatInt(document.GetId(), 10),
		SetName:      setName,
		Type:         "regular",
	}
	for _, attr := range document.GetAttributes() {
		if attr == nil {
			continue
		}
		switch attr.GetPredicateName() {
		case mtproto.Predicate_documentAttributeSticker:
			result.Emoji = attr.GetAlt()
			if attr.GetStickerset() != nil && attr.GetStickerset().GetShortName() != "" {
				result.SetName = attr.GetStickerset().GetShortName()
			}
			if attr.GetMask() {
				result.Type = "mask"
				result.MaskPosition = botAPIMaskPositionFromCoords(attr.GetMaskCoords())
			}
		case mtproto.Predicate_documentAttributeCustomEmoji:
			result.Type = "custom_emoji"
			result.Emoji = attr.GetAlt()
			if attr.GetStickerset() != nil && attr.GetStickerset().GetShortName() != "" {
				result.SetName = attr.GetStickerset().GetShortName()
			}
		case mtproto.Predicate_documentAttributeImageSize:
			result.Width, result.Height = int(attr.GetW()), int(attr.GetH())
		case mtproto.Predicate_documentAttributeVideo:
			result.Width, result.Height = int(attr.GetW()), int(attr.GetH())
			result.IsVideo = true
		case mtproto.Predicate_documentAttributeAnimated:
			result.IsAnimated = true
		}
	}
	if result.Emoji == "" {
		result.Emoji = packEmoji
	}
	return result
}

func botAPIMaskPositionFromCoords(coords *mtproto.MaskCoords) *botapi.MaskPosition {
	if coords == nil {
		return nil
	}
	points := []string{"forehead", "eyes", "mouth", "chin"}
	point := int(coords.GetN())
	if point < 0 || point >= len(points) {
		return nil
	}
	return &botapi.MaskPosition{Point: points[point], XShift: coords.GetX(), YShift: coords.GetY(), Scale: coords.GetZoom()}
}

// jsonUnmarshal is isolated so form and JSON request parsing share the same
// decoder semantics without exposing another public Bot API type.
func jsonUnmarshal(raw []byte, target any) error {
	return json.Unmarshal(raw, target)
}
