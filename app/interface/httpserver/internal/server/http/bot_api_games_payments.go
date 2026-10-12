// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func sendChatAction(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.DialogsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: dialog service is not configured"
	}
	botID := bot.GetUser().GetId()
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	actionName := strings.TrimSpace(request.string("action"))
	var action *mtproto.SendMessageAction
	switch actionName {
	case "typing":
		action = mtproto.MakeTLSendMessageTypingAction(nil).To_SendMessageAction()
	case "upload_photo":
		action = mtproto.MakeTLSendMessageUploadPhotoAction(nil).To_SendMessageAction()
	case "record_video":
		action = mtproto.MakeTLSendMessageRecordVideoAction(nil).To_SendMessageAction()
	case "upload_video":
		action = mtproto.MakeTLSendMessageUploadVideoAction(nil).To_SendMessageAction()
	case "record_voice":
		action = mtproto.MakeTLSendMessageRecordAudioAction(nil).To_SendMessageAction()
	case "upload_voice":
		action = mtproto.MakeTLSendMessageUploadAudioAction(nil).To_SendMessageAction()
	case "upload_document":
		action = mtproto.MakeTLSendMessageUploadDocumentAction(nil).To_SendMessageAction()
	case "choose_sticker":
		action = mtproto.MakeTLSendMessageChooseStickerAction(nil).To_SendMessageAction()
	case "find_location":
		action = mtproto.MakeTLSendMessageGeoLocationAction(nil).To_SendMessageAction()
	case "record_video_note":
		action = mtproto.MakeTLSendMessageRecordRoundAction(nil).To_SendMessageAction()
	case "upload_video_note":
		action = mtproto.MakeTLSendMessageUploadRoundAction(nil).To_SendMessageAction()
	case "cancel":
		action = mtproto.MakeTLSendMessageCancelAction(nil).To_SendMessageAction()
	default:
		return nil, http.StatusBadRequest, 400, "Bad Request: action is invalid"
	}
	topMsgID, err := botAPIOptionalInt32(request, "message_thread_id", 1, int64(^uint32(0)>>1))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	ok, err := ctx.DialogsClient.MessagesSetTyping(botRPCContext(r.Context(), botID), &mtproto.TLMessagesSetTyping{
		Peer:     botAPIInputPeer(botID, peerType, peerID),
		TopMsgId: topMsgID,
		Action:   action,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: failed to set chat action"
	}
	return true, http.StatusOK, 0, ""
}

func sendGame(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	shortName := strings.TrimSpace(request.string("game_short_name"))
	if shortName == "" || len(shortName) > 64 {
		return nil, http.StatusBadRequest, 400, "Bad Request: game_short_name is invalid"
	}
	inputGame := mtproto.MakeTLInputGameShortName(&mtproto.InputGame{
		BotId:     mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: bot.GetUser().GetId(), AccessHash: bot.GetUser().GetAccessHash()}).To_InputUser(),
		ShortName: shortName,
	}).To_InputGame()
	media := mtproto.MakeTLInputMediaGame(&mtproto.InputMedia{Id_INPUTGAME: inputGame}).To_InputMedia()
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func sendInvoice(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	title := strings.TrimSpace(request.string("title"))
	description := strings.TrimSpace(request.string("description"))
	payload := request.string("payload")
	currency := strings.ToUpper(strings.TrimSpace(request.string("currency")))
	provider := strings.TrimSpace(request.string("provider_token"))
	if title == "" || description == "" || payload == "" || provider == "" || len(payload) > 4096 || len(currency) != 3 {
		return nil, http.StatusBadRequest, 400, "Bad Request: invoice fields are invalid"
	}
	var prices []botapi.LabeledPrice
	if err := request.json("prices", &prices); err != nil || len(prices) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: prices is invalid"
	}
	mtPrices := make([]*mtproto.LabeledPrice, 0, len(prices))
	for _, price := range prices {
		if strings.TrimSpace(price.Label) == "" || price.Amount < 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: price is invalid"
		}
		mtPrices = append(mtPrices, mtproto.MakeTLLabeledPrice(&mtproto.LabeledPrice{Label: price.Label, Amount: price.Amount}).To_LabeledPrice())
	}
	providerData := request.string("provider_data")
	if providerData == "" {
		providerData = "{}"
	}
	if !json.Valid([]byte(providerData)) {
		return nil, http.StatusBadRequest, 400, "Bad Request: provider_data is invalid"
	}
	invoice := mtproto.MakeTLInvoice(&mtproto.Invoice{
		Currency:                 currency,
		Prices:                   mtPrices,
		Test:                     requestBool(request, "test"),
		NameRequested:            requestBool(request, "need_name"),
		PhoneRequested:           requestBool(request, "need_phone_number"),
		EmailRequested:           requestBool(request, "need_email"),
		ShippingAddressRequested: requestBool(request, "need_shipping_address"),
		Flexible:                 requestBool(request, "is_flexible"),
		PhoneToProvider:          requestBool(request, "send_phone_number_to_provider"),
		EmailToProvider:          requestBool(request, "send_email_to_provider"),
	}).To_Invoice()
	media := &mtproto.InputMedia{
		Title:           title,
		Description:     description,
		Invoice:         invoice,
		Payload_BYTES:   []byte(payload),
		Provider_STRING: provider,
		ProviderData:    mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: providerData}).To_DataJSON(),
	}
	if startParam := strings.TrimSpace(request.string("start_parameter")); startParam != "" {
		media.StartParam = wrapperspb.String(startParam)
	}
	photoURL := strings.TrimSpace(request.string("photo_url"))
	if photoURL != "" {
		parsed, err := url.ParseRequestURI(photoURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, http.StatusBadRequest, 400, "Bad Request: photo_url is invalid"
		}
		photoSize, err := request.int("photo_size")
		if err != nil || photoSize < 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: photo_size is invalid"
		}
		photoWidth, err := request.int("photo_width")
		if err != nil || photoWidth < 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: photo_width is invalid"
		}
		photoHeight, err := request.int("photo_height")
		if err != nil || photoHeight < 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: photo_height is invalid"
		}
		media.Photo = mtproto.MakeTLInputWebDocument(&mtproto.InputWebDocument{
			Url:      photoURL,
			Size2:    int32(photoSize),
			MimeType: "image/jpeg",
			Attributes: []*mtproto.DocumentAttribute{mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
				W: int32(photoWidth), H: int32(photoHeight),
			}).To_DocumentAttribute()},
		}).To_InputWebDocument()
	}
	return sendBotAPIStructuredMedia(ctx, r, bot, request, mtproto.MakeTLInputMediaInvoice(media).To_InputMedia(), "")
}

func setGameScore(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.GamesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: game service is not configured"
	}
	score, err := request.int("score")
	if err != nil || score < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: score is invalid"
	}
	force, err := request.bool("force")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: force is invalid"
	}
	editMessage := !requestBool(request, "disable_edit_message")
	userID, err := request.int64("user_id")
	if err != nil || userID < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	var user *mtproto.InputUser
	if userID > 0 {
		user = mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID}).To_InputUser()
	}
	if inlineID := strings.TrimSpace(request.string("inline_message_id")); inlineID != "" {
		id, err := parseBotAPIInlineMessageID(inlineID)
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: inline_message_id is invalid"
		}
		ok, err := ctx.GamesClient.MessagesSetInlineGameScore(botRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLMessagesSetInlineGameScore{
			EditMessage: editMessage, Force: force, Id: id, UserId: user, Score: int32(score),
		})
		if err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		if !mtproto.FromBool(ok) {
			return nil, http.StatusBadRequest, 400, "Bad Request: game score was not changed"
		}
		return true, http.StatusOK, 0, ""
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, bot.GetUser().GetId(), request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	updates, err := ctx.GamesClient.MessagesSetGameScore(botRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLMessagesSetGameScore{
		EditMessage: editMessage, Force: force, Peer: botAPIInputPeer(bot.GetUser().GetId(), peerType, peerID), Id: int32(messageID), UserId: user, Score: int32(score),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	message := botapi.FirstMessageFromUpdates(updates)
	if message == nil {
		return true, http.StatusOK, 0, ""
	}
	return message, http.StatusOK, 0, ""
}

func getGameHighScores(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.GamesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: game service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	var user *mtproto.InputUser
	if userID > 0 {
		user = mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID}).To_InputUser()
	}
	var scores *mtproto.Messages_HighScores
	if inlineID := strings.TrimSpace(request.string("inline_message_id")); inlineID != "" {
		id, err := parseBotAPIInlineMessageID(inlineID)
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: inline_message_id is invalid"
		}
		scores, err = ctx.GamesClient.MessagesGetInlineGameHighScores(botRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLMessagesGetInlineGameHighScores{Id: id, UserId: user})
		if err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
	} else {
		_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, bot.GetUser().GetId(), request.string("chat_id"))
		if err != nil || peerID == 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
		}
		messageID, err := request.int("message_id")
		if err != nil || messageID <= 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
		}
		scores, err = ctx.GamesClient.MessagesGetGameHighScores(botRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLMessagesGetGameHighScores{Peer: botAPIInputPeer(bot.GetUser().GetId(), peerType, peerID), Id: int32(messageID), UserId: user})
		if err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
	}
	return botAPIHighScores(scores), http.StatusOK, 0, ""
}

func botAPIHighScores(scores *mtproto.Messages_HighScores) []botapi.GameHighScore {
	if scores == nil {
		return []botapi.GameHighScore{}
	}
	users := make(map[int64]*mtproto.User, len(scores.GetUsers()))
	for _, user := range scores.GetUsers() {
		if user != nil {
			users[user.GetId()] = user
		}
	}
	result := make([]botapi.GameHighScore, 0, len(scores.GetScores()))
	for _, score := range scores.GetScores() {
		if score == nil {
			continue
		}
		result = append(result, botapi.GameHighScore{Position: int(score.GetPos()), User: botapi.UserFromMTProto(users[score.GetUserId()]), Score: int(score.GetScore())})
	}
	return result
}

func parseBotAPIInlineMessageID(raw string) (*mtproto.InputBotInlineMessageID, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 3 && len(parts) != 4 {
		return nil, errors.New("invalid inline message id")
	}
	values := make([]int64, len(parts))
	for i, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	if len(values) == 3 {
		return mtproto.MakeTLInputBotInlineMessageID(&mtproto.InputBotInlineMessageID{DcId: int32(values[0]), Id_INT64: values[1], AccessHash: values[2]}).To_InputBotInlineMessageID(), nil
	}
	return mtproto.MakeTLInputBotInlineMessageID64(&mtproto.InputBotInlineMessageID{DcId: int32(values[0]), OwnerId: values[1], Id_INT32: int32(values[2]), AccessHash: values[3]}).To_InputBotInlineMessageID(), nil
}

func getBusinessConnection(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BusinessBotsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: business connection service is not configured"
	}
	connectionID := strings.TrimSpace(request.string("business_connection_id"))
	if connectionID == "" || len(connectionID) > 128 {
		return nil, http.StatusBadRequest, 400, "Bad Request: business_connection_id is invalid"
	}
	updates, err := ctx.BusinessBotsClient.AccountGetBotBusinessConnection(botRPCContext(requestCtx, botID), &mtproto.TLAccountGetBotBusinessConnection{ConnectionId: connectionID})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if updates == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: business connection is invalid"
	}
	allUpdates := make([]*mtproto.Update, 0, 1+len(updates.GetUpdates()))
	if updates.GetUpdate() != nil {
		allUpdates = append(allUpdates, updates.GetUpdate())
	}
	allUpdates = append(allUpdates, updates.GetUpdates()...)
	for _, update := range allUpdates {
		converted, ok := botapi.UpdateFromMTProto(update, updates.GetUsers(), updates.GetChats())
		if ok && converted != nil && converted.BusinessConnection != nil {
			return converted.BusinessConnection, http.StatusOK, 0, ""
		}
	}
	return nil, http.StatusBadRequest, 400, "Bad Request: business connection is invalid"
}
