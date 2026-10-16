// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func savePreparedInlineMessage(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.PreparedInlineClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: prepared inline message service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	var rawResult map[string]json.RawMessage
	rawResultValue := request.raw("result")
	if len(rawResultValue) > 0 && rawResultValue[0] == '"' {
		var encoded string
		if json.Unmarshal(rawResultValue, &encoded) == nil {
			rawResultValue = json.RawMessage(encoded)
		}
	}
	if len(rawResultValue) == 0 || json.Unmarshal(rawResultValue, &rawResult) != nil || len(rawResult) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: result is invalid"
	}
	result, err := botAPIWebAppInlineResult(rawResult)
	if err != nil {
		if errors.Is(err, errBotAPIWebAppResultUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: this inline result type is not supported"
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: result is invalid"
	}
	peerTypes, err := botAPIPreparedInlinePeerTypes(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	saved, err := ctx.PreparedInlineClient.MessagesSavePreparedInlineMessage(botRPCContext(r.Context(), botID), &mtproto.TLMessagesSavePreparedInlineMessage{
		Result:    result,
		UserId:    inputUser,
		PeerTypes: peerTypes,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if saved == nil || strings.TrimSpace(saved.GetId()) == "" {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: prepared inline message was not returned"
	}
	return &botapi.PreparedInlineMessage{
		ID:             saved.GetId(),
		ExpirationDate: int64(saved.GetExpireDate()),
	}, http.StatusOK, 0, ""
}

func botAPIPreparedInlinePeerTypes(request *botAPIRequest) ([]*mtproto.InlineQueryPeerType, error) {
	allowUsers, err := request.bool("allow_user_chats")
	if err != nil {
		return nil, errors.New("allow_user_chats is invalid")
	}
	allowBots, err := request.bool("allow_bot_chats")
	if err != nil {
		return nil, errors.New("allow_bot_chats is invalid")
	}
	allowGroups, err := request.bool("allow_group_chats")
	if err != nil {
		return nil, errors.New("allow_group_chats is invalid")
	}
	allowChannels, err := request.bool("allow_channel_chats")
	if err != nil {
		return nil, errors.New("allow_channel_chats is invalid")
	}
	peerTypes := make([]*mtproto.InlineQueryPeerType, 0, 5)
	if allowUsers {
		peerTypes = append(peerTypes, mtproto.MakeTLInlineQueryPeerTypePM(nil).To_InlineQueryPeerType())
	}
	if allowBots {
		peerTypes = append(peerTypes, mtproto.MakeTLInlineQueryPeerTypeBotPM(nil).To_InlineQueryPeerType())
	}
	if allowGroups {
		peerTypes = append(peerTypes,
			mtproto.MakeTLInlineQueryPeerTypeChat(nil).To_InlineQueryPeerType(),
			mtproto.MakeTLInlineQueryPeerTypeMegagroup(nil).To_InlineQueryPeerType(),
		)
	}
	if allowChannels {
		peerTypes = append(peerTypes, mtproto.MakeTLInlineQueryPeerTypeBroadcast(nil).To_InlineQueryPeerType())
	}
	return peerTypes, nil
}

func deleteEphemeralMessage(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.EphemeralClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral message service is not configured"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a group or supergroup"
	}
	receiverID, err := request.int64("receiver_user_id")
	if err != nil || receiverID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: receiver_user_id is invalid"
	}
	messageID, err := request.int("ephemeral_message_id")
	if err != nil || messageID <= 0 || messageID > int(^uint32(0)>>1) {
		return nil, http.StatusBadRequest, 400, "Bad Request: ephemeral_message_id is invalid"
	}
	receiver, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, receiverID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	ok, err := ctx.EphemeralClient.EphemeralDeleteMessage(botRPCContext(r.Context(), botID), &mtproto.TLEphemeralDeleteMessage{
		Peer:       botAPIInputPeer(botID, peerType, peerID),
		ReceiverId: receiver,
		Id:         int32(messageID),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: ephemeral message is invalid or already deleted"
	}
	return true, http.StatusOK, 0, ""
}

func botAPIEphemeralTarget(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (int32, int64, *mtproto.InputUser, int32, error) {
	if ctx == nil || ctx.EphemeralClient == nil {
		return 0, 0, nil, 0, errors.New("ephemeral message service is not configured")
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return 0, 0, nil, 0, errors.New("chat_id must be a group or supergroup")
	}
	receiverID, err := request.int64("receiver_user_id")
	if err != nil || receiverID <= 0 {
		return 0, 0, nil, 0, errors.New("receiver_user_id is invalid")
	}
	messageID, err := request.int("ephemeral_message_id")
	if err != nil || messageID <= 0 || messageID > int(^uint32(0)>>1) {
		return 0, 0, nil, 0, errors.New("ephemeral_message_id is invalid")
	}
	receiver, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, receiverID)
	if err != nil {
		return 0, 0, nil, 0, err
	}
	return peerType, peerID, receiver, int32(messageID), nil
}

func editEphemeralMessage(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, message string, entities []botapi.MessageEntity, media *mtproto.InputMedia, rich *mtproto.InputRichMessage, markup *mtproto.ReplyMarkup) (any, int, int, string) {
	peerType, peerID, receiver, messageID, err := botAPIEphemeralTarget(ctx, r, botID, request)
	if err != nil {
		if strings.Contains(err.Error(), "service is not configured") {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral message service is not configured"
		}
		if strings.Contains(err.Error(), "receiver") || strings.Contains(err.Error(), "ephemeral") || strings.Contains(err.Error(), "chat_id") {
			return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	_, _, linkErr := botAPILinkPreviewOptions(request)
	if linkErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
	}
	updates, err := ctx.EphemeralClient.EphemeralSendMessage(botRPCContext(r.Context(), botID), &mtproto.TLEphemeralSendMessage{
		Peer:        botAPIInputPeer(botID, peerType, peerID),
		ReceiverId:  receiver,
		QueryId:     wrapperspb.Int64(-int64(messageID)),
		Message:     message,
		Entities:    botapi.MessageEntitiesToMTProto(entities),
		Media:       media,
		RichMessage: rich,
		ReplyMarkup: markup,
		RandomId:    randomMessageID(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if updates == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: ephemeral message edit was not accepted"
	}
	return true, http.StatusOK, 0, ""
}

func editEphemeralMessageText(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	botID := bot.GetUser().GetId()
	text, entities, err := botAPITextEntities(request, "text", "entities", "parse_mode")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if request.has("rich_message") && strings.TrimSpace(string(request.raw("rich_message"))) != "null" {
		if request.has("text") && request.text("text") != "" {
			return nil, http.StatusBadRequest, 400, "Bad Request: text and rich_message are mutually exclusive"
		}
		rich, richErr := botAPIInputRichMessage(request)
		if richErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: rich_message is invalid"
		}
		markup, markupErr := botAPIReplyMarkup(request)
		if markupErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
		}
		return editEphemeralMessage(ctx, r, botID, request, "", nil, nil, rich, markup)
	}
	if text == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: text is required when rich_message is absent"
	}
	if utf8.RuneCountInString(text) > 4096 {
		return nil, http.StatusBadRequest, 400, "Bad Request: text is too long"
	}
	markup, markupErr := botAPIReplyMarkup(request)
	if markupErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editEphemeralMessage(ctx, r, botID, request, text, entities, nil, nil, markup)
}

func editEphemeralMessageMedia(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	botID := bot.GetUser().GetId()
	if len(request.raw("media")) == 0 || string(request.raw("media")) == "null" {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is required"
	}
	showAbove, boolErr := request.bool("show_caption_above_media")
	if boolErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: show_caption_above_media is invalid"
	}
	if showAbove {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: show_caption_above_media is not supported for ephemeral messages"
	}
	if _, _, _, _, err := botAPIEphemeralTarget(ctx, r, botID, request); err != nil {
		if strings.Contains(err.Error(), "service is not configured") {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral message service is not configured"
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	var item botAPIMediaGroupItem
	if err := request.json("media", &item); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is invalid"
	}
	media, err := botAPIMediaGroupInput(r, ctx, botID, item)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is invalid"
	}
	caption, entities, captionErr := botAPIMediaGroupCaption(item)
	if captionErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + captionErr.Error()
	}
	if utf8.RuneCountInString(caption) > 1024 {
		return nil, http.StatusBadRequest, 400, "Bad Request: caption is too long"
	}
	markup, markupErr := botAPIReplyMarkup(request)
	if markupErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editEphemeralMessage(ctx, r, botID, request, caption, entities, media, nil, markup)
}

func editEphemeralMessageCaption(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	botID := bot.GetUser().GetId()
	caption, entities, err := botAPICaptionEntities(request, "caption", "caption_entities")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if utf8.RuneCountInString(caption) > 1024 {
		return nil, http.StatusBadRequest, 400, "Bad Request: caption is too long"
	}
	showAbove, boolErr := request.bool("show_caption_above_media")
	if boolErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: show_caption_above_media is invalid"
	}
	if showAbove {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: show_caption_above_media is not supported for ephemeral messages"
	}
	markup, markupErr := botAPIReplyMarkup(request)
	if markupErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editEphemeralMessage(ctx, r, botID, request, caption, entities, nil, nil, markup)
}

func editEphemeralMessageReplyMarkup(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	botID := bot.GetUser().GetId()
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	if markup == nil {
		markup = mtproto.MakeTLReplyInlineMarkup(&mtproto.ReplyMarkup{Rows: []*mtproto.KeyboardButtonRow{}}).To_ReplyMarkup()
	}
	return editEphemeralMessage(ctx, r, botID, request, "", nil, nil, nil, markup)
}
