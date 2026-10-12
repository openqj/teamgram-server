// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	rpcmetadata "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/botapi/store"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	dfspb "github.com/teamgram/teamgram-server/app/service/dfs/dfs"

	"github.com/zeromicro/go-zero/rest/httpx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botAPIPath struct {
	Token  string `path:"token"`
	Method string `path:"method"`
}

type botAPIFilePath struct {
	Token    string `path:"token"`
	Kind     string `path:"kind"`
	FileName string `path:"file_name"`
}

type botAPIRequest struct {
	values map[string]json.RawMessage
}

func newBotAPIRequest(r *http.Request) (*botAPIRequest, error) {
	result := &botAPIRequest{values: make(map[string]json.RawMessage)}
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, err
		}
		if r.MultipartForm != nil {
			for name, values := range r.MultipartForm.Value {
				if len(values) > 0 {
					result.values[name] = json.RawMessage(values[0])
				}
			}
		}
		return result, nil
	}
	if strings.Contains(contentType, "application/json") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			return nil, err
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			if err := json.Unmarshal(body, &result.values); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for name, values := range r.Form {
		if len(values) > 0 {
			result.values[name] = json.RawMessage(values[0])
		}
	}
	return result, nil
}

func (r *botAPIRequest) raw(name string) json.RawMessage {
	if r == nil {
		return nil
	}
	return r.values[name]
}

func (r *botAPIRequest) string(name string) string {
	value := r.raw(name)
	if len(value) == 0 {
		return ""
	}
	var decoded string
	if json.Unmarshal(value, &decoded) == nil {
		return strings.TrimSpace(decoded)
	}
	return strings.TrimSpace(string(value))
}

func (r *botAPIRequest) int64(name string) (int64, error) {
	value := r.string(name)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func (r *botAPIRequest) int(name string) (int, error) {
	value := r.string(name)
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func (r *botAPIRequest) float64(name string) (float64, error) {
	value := r.string(name)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}

func (r *botAPIRequest) bool(name string) (bool, error) {
	value := r.string(name)
	if value == "" {
		return false, nil
	}
	return strconv.ParseBool(value)
}

func (r *botAPIRequest) json(name string, target any) error {
	value := r.raw(name)
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	if value[0] != '[' && value[0] != '{' && value[0] != '"' {
		value = json.RawMessage(r.string(name))
		if len(value) == 0 {
			return nil
		}
	}
	return json.Unmarshal(value, target)
}

func botAPIHandler(ctx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var path botAPIPath
		if err := httpx.ParsePath(r, &path); err != nil || path.Token == "" || path.Method == "" {
			writeBotAPIError(w, http.StatusUnauthorized, 401, "Unauthorized")
			return
		}
		bot, err := botAPIUserByToken(r.Context(), ctx, path.Token)
		if err != nil || bot == nil || bot.GetUser() == nil || bot.GetUser().GetBot() == nil {
			writeBotAPIError(w, http.StatusUnauthorized, 401, "Unauthorized")
			return
		}
		request, err := newBotAPIRequest(r)
		if err != nil {
			writeBotAPIError(w, http.StatusBadRequest, 400, "Bad Request: invalid request body")
			return
		}
		result, status, code, description := dispatchBotAPIMethod(ctx, r, bot, path.Method, request)
		if code != 0 {
			writeBotAPIError(w, status, code, description)
			return
		}
		writeBotAPIOK(w, result)
	}
}

func botAPIFileHandler(ctx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var route botAPIFilePath
		if err := httpx.ParsePath(r, &route); err != nil || route.Token == "" || route.Kind == "" || route.FileName == "" {
			writeBotAPIError(w, http.StatusUnauthorized, 401, "Unauthorized")
			return
		}
		filePath := strings.Trim(route.Kind, "/") + "/" + strings.Trim(route.FileName, "/")
		bot, err := botAPIUserByToken(r.Context(), ctx, route.Token)
		if err != nil || bot == nil || bot.GetUser() == nil || bot.GetUser().GetBot() == nil {
			writeBotAPIError(w, http.StatusUnauthorized, 401, "Unauthorized")
			return
		}
		if ctx.DfsClient == nil {
			writeBotAPIError(w, http.StatusNotImplemented, 501, "Not Implemented: file service is not configured")
			return
		}
		ref, err := parseBotAPIFilePath(filePath)
		if err != nil {
			writeBotAPIError(w, http.StatusBadRequest, 400, "Bad Request: file_path is invalid")
			return
		}
		file, err := ctx.DfsClient.DfsDownloadFile(botRPCContext(r.Context(), bot.GetUser().GetId()), &dfspb.TLDfsDownloadFile{
			Location: ref.location,
			Offset:   0,
			Limit:    16 << 20,
		})
		if err != nil || file == nil || len(file.GetBytes()) == 0 {
			writeBotAPIError(w, http.StatusNotFound, 404, "Bad Request: file is not available")
			return
		}
		contentType := mime.TypeByExtension(path.Ext(filePath))
		if contentType == "" {
			contentType = http.DetectContentType(file.GetBytes())
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(file.GetBytes())))
		_, _ = w.Write(file.GetBytes())
	}
}

// normalizeBotAPIToken accepts Telegram's canonical "bot<TOKEN>" path
// segment as well as the legacy route that carries the raw token separately.
// Tokens issued by BotFather always contain a colon, which keeps this
// normalization from changing an otherwise valid raw token prefix.
func normalizeBotAPIToken(raw string) string {
	if strings.HasPrefix(raw, "bot") && strings.Contains(raw[3:], ":") {
		return raw[3:]
	}
	return raw
}

// botAPIUserByToken supports the canonical /bot<TOKEN>/method path and the
// legacy slash form. Older local fixtures may use opaque tokens without a
// colon, so the lookup retries the value without the path prefix.
func botAPIUserByToken(ctx context.Context, service *svc.ServiceContext, raw string) (*mtproto.ImmutableUser, error) {
	if service == nil || service.UserClient == nil || strings.TrimSpace(raw) == "" {
		return nil, mtproto.ErrTokenInvalid
	}
	var lastErr error
	for _, token := range botAPITokenCandidates(raw) {
		bot, err := service.UserClient.UserGetImmutableUserByToken(ctx, &userpb.TLUserGetImmutableUserByToken{Token: token})
		if err == nil && bot != nil && bot.GetUser() != nil && bot.GetUser().GetBot() != nil {
			return bot, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = mtproto.ErrTokenInvalid
	}
	return nil, lastErr
}

func botAPITokenCandidates(raw string) []string {
	canonical := normalizeBotAPIToken(raw)
	candidates := []string{canonical}
	if strings.HasPrefix(raw, "bot") && !strings.Contains(raw[3:], ":") {
		candidates = append(candidates, raw[3:])
	}
	if raw != canonical {
		candidates = append(candidates, raw)
	}
	return candidates
}

func dispatchBotAPIMethod(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, method string, request *botAPIRequest) (any, int, int, string) {
	botID := bot.GetUser().GetId()
	switch method {
	case "getMe":
		return botapi.UserFromImmutableUser(bot), http.StatusOK, 0, ""
	case "getFile":
		return getFile(request)
	case "getUpdates":
		return getUpdates(ctx, r, botID, request)
	case "setWebhook":
		return setWebhook(ctx, r.Context(), botID, request)
	case "deleteWebhook":
		return deleteWebhook(ctx, r.Context(), botID, request)
	case "getWebhookInfo":
		info, err := ctx.BotAPIStore.WebhookInfo(r.Context(), botID)
		if err != nil {
			return nil, http.StatusInternalServerError, 500, err.Error()
		}
		return info, http.StatusOK, 0, ""
	case "sendMessage":
		return sendMessage(ctx, r, bot, request)
	case "sendPhoto":
		return sendBotAPIMedia(ctx, r, bot, request, "photo", "photo")
	case "sendDocument":
		return sendBotAPIMedia(ctx, r, bot, request, "document", "document")
	case "sendVideo":
		return sendBotAPIMedia(ctx, r, bot, request, "video", "video")
	case "sendVideoNote":
		return sendBotAPIMedia(ctx, r, bot, request, "video_note", "video_note")
	case "sendSticker":
		return sendSticker(ctx, r, bot, request)
	case "sendAudio":
		return sendBotAPIMedia(ctx, r, bot, request, "audio", "audio")
	case "sendVoice":
		return sendBotAPIMedia(ctx, r, bot, request, "voice", "voice")
	case "sendAnimation":
		return sendBotAPIMedia(ctx, r, bot, request, "animation", "animation")
	case "sendLocation":
		return sendLocation(ctx, r, bot, request)
	case "sendVenue":
		return sendVenue(ctx, r, bot, request)
	case "sendContact":
		return sendContact(ctx, r, bot, request)
	case "sendPoll":
		return sendPoll(ctx, r, bot, request)
	case "sendDice":
		return sendDice(ctx, r, bot, request)
	case "sendMediaGroup":
		return sendMediaGroup(ctx, r, bot, request)
	case "forwardMessage":
		return forwardMessage(ctx, r, bot, request, false)
	case "copyMessage":
		return forwardMessage(ctx, r, bot, request, true)
	case "getChat":
		return getChat(ctx, r, botID, request)
	case "getChatAdministrators":
		return getChatAdministrators(ctx, r, botID, request)
	case "getChatMemberCount":
		return getChatMemberCount(ctx, r, botID, request)
	case "getChatMember":
		return getChatMember(ctx, r, botID, request)
	case "banChatMember", "kickChatMember":
		return banChatMember(ctx, r, botID, request)
	case "unbanChatMember":
		return unbanChatMember(ctx, r, botID, request)
	case "restrictChatMember":
		return restrictChatMember(ctx, r, botID, request)
	case "promoteChatMember":
		return promoteChatMember(ctx, r, botID, request)
	case "setChatAdministratorCustomTitle":
		return setChatAdministratorCustomTitle(ctx, r, botID, request)
	case "setChatPermissions":
		return setChatPermissions(ctx, r, botID, request)
	case "setChatTitle":
		return setChatTitle(ctx, r, botID, request)
	case "setChatDescription":
		return setChatDescription(ctx, r, botID, request)
	case "pinChatMessage":
		return pinChatMessage(ctx, r, botID, request)
	case "unpinChatMessage":
		return unpinChatMessage(ctx, r, botID, request)
	case "unpinAllChatMessages":
		return unpinAllChatMessages(ctx, r, botID, request)
	case "leaveChat":
		return leaveChat(ctx, r, botID, request)
	case "getMyCommands":
		info, err := ctx.UserClient.UserGetBotInfo(botRPCContext(r.Context(), botID), &userpb.TLUserGetBotInfo{BotId: botID})
		if err != nil || info == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return botapi.BotCommandsFromMTProto(info.GetCommands()), http.StatusOK, 0, ""
	case "setMyCommands", "deleteMyCommands":
		commands := []botapi.BotCommand{}
		if method == "setMyCommands" {
			if err := request.json("commands", &commands); err != nil {
				return nil, http.StatusBadRequest, 400, "Bad Request: commands must be a JSON array"
			}
		}
		ok, err := ctx.UserClient.UserSetBotCommands(botRPCContext(r.Context(), botID), &userpb.TLUserSetBotCommands{
			UserId:   botID,
			BotId:    botID,
			Commands: botapi.MTProtoBotCommands(commands),
		})
		if err != nil || !mtproto.FromBool(ok) {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return true, http.StatusOK, 0, ""
	case "setChatMenuButton":
		return setChatMenuButton(ctx, r.Context(), botID, request)
	case "getChatMenuButton":
		return getChatMenuButton(ctx, r.Context(), botID, request)
	case "setMyName":
		return setMyName(ctx, r.Context(), botID, request)
	case "getMyName":
		return &botapi.BotName{Name: bot.GetUser().GetFirstName()}, http.StatusOK, 0, ""
	case "setMyDescription":
		return setMyDescription(ctx, r.Context(), botID, request)
	case "getMyDescription":
		return getMyDescription(ctx, r.Context(), botID)
	case "setMyShortDescription":
		return setMyShortDescription(ctx, r.Context(), botID, request)
	case "getMyShortDescription":
		return getMyShortDescription(ctx, r.Context(), botID)
	case "editMessageText":
		return editMessageText(ctx, r, bot, request)
	case "editMessageCaption":
		return editMessageCaption(ctx, r, bot, request)
	case "editMessageMedia":
		return editMessageMedia(ctx, r, bot, request)
	case "editMessageReplyMarkup":
		return editMessageReplyMarkup(ctx, r, bot, request)
	case "deleteMessage":
		return deleteMessage(ctx, r, botID, request)
	case "setMessageReaction":
		return setMessageReaction(ctx, r, botID, request)
	case "answerCallbackQuery":
		return answerCallbackQuery(ctx, r.Context(), botID, request)
	case "answerInlineQuery":
		return answerInlineQuery(ctx, r.Context(), botID, request)
	case "answerWebAppQuery":
		return answerWebAppQuery(ctx, r.Context(), botID, request)
	case "answerShippingQuery":
		return answerShippingQuery(ctx, r.Context(), botID, request)
	case "answerPreCheckoutQuery":
		return answerPreCheckoutQuery(ctx, r.Context(), botID, request)
	default:
		return nil, http.StatusNotImplemented, 501, "Bad Request: method is not supported by this server"
	}
}

type botAPIFileRef struct {
	kind       string
	id         int64
	accessHash int64
	location   *mtproto.InputFileLocation
}

func getFile(request *botAPIRequest) (any, int, int, string) {
	ref, err := parseBotAPIFileID(request.string("file_id"))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: file_id is invalid"
	}
	return &botapi.File{
		FileID:       request.string("file_id"),
		FileUniqueID: strconv.FormatInt(ref.id, 10),
		FilePath:     botAPIFilePathForRef(ref),
	}, http.StatusOK, 0, ""
}

func parseBotAPIFileID(raw string) (*botAPIFileRef, error) {
	parts := strings.Split(strings.TrimSpace(raw), "_")
	if len(parts) != 3 || (parts[0] != "photo" && parts[0] != "document") {
		return nil, errors.New("invalid bot api file id")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New("invalid file id")
	}
	accessHash, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, errors.New("invalid file access hash")
	}
	ref := &botAPIFileRef{kind: parts[0], id: id, accessHash: accessHash}
	if parts[0] == "photo" {
		ref.location = mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
			Id: id, AccessHash: accessHash, ThumbSize: "x",
		}).To_InputFileLocation()
	} else {
		ref.location = mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
			Id: id, AccessHash: accessHash,
		}).To_InputFileLocation()
	}
	return ref, nil
}

func botAPIFilePathForRef(ref *botAPIFileRef) string {
	if ref == nil {
		return ""
	}
	return fmt.Sprintf("%s/%d_%d.dat", ref.kind, ref.id, ref.accessHash)
}

func parseBotAPIFilePath(raw string) (*botAPIFileRef, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || !strings.HasSuffix(parts[1], ".dat") {
		return nil, errors.New("invalid file path")
	}
	base := strings.TrimSuffix(parts[1], ".dat")
	if !strings.Contains(base, "_") {
		base += "_0"
	}
	return parseBotAPIFileID(parts[0] + "_" + base)
}

func getUpdates(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	webhook, _, err := ctx.BotAPIStore.GetWebhook(r.Context(), botID)
	if err != nil {
		return nil, http.StatusInternalServerError, 500, err.Error()
	}
	if webhook.URL != "" {
		return nil, http.StatusConflict, 409, "Conflict: can't use getUpdates method while webhook is active"
	}
	offset, err := request.int64("offset")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid offset"
	}
	limit, err := request.int("limit")
	if err != nil || limit < 0 || limit > 100 {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid limit"
	}
	timeout, err := request.int("timeout")
	if err != nil || timeout < 0 || timeout > 50 {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid timeout"
	}
	var allowed []string
	if err := request.json("allowed_updates", &allowed); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid allowed_updates"
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for {
		updates, err := ctx.BotAPIStore.GetUpdates(r.Context(), botID, offset, limit, allowed)
		if err != nil {
			return nil, http.StatusInternalServerError, 500, err.Error()
		}
		if len(updates) > 0 || timeout == 0 || time.Now().After(deadline) {
			return updates, http.StatusOK, 0, ""
		}
		select {
		case <-r.Context().Done():
			return nil, http.StatusRequestTimeout, 408, "Request Timeout"
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func setWebhook(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	webhookURL := request.string("url")
	if webhookURL == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: url is required"
	}
	parsedURL, parseErr := url.ParseRequestURI(webhookURL)
	if parseErr != nil || parsedURL.Host == "" || parsedURL.Scheme != "https" {
		return nil, http.StatusBadRequest, 400, "Bad Request: url is invalid"
	}
	maxConnections := 0
	if raw := request.raw("max_connections"); len(raw) > 0 && string(raw) != "null" {
		var maxConnectionsErr error
		maxConnections, maxConnectionsErr = request.int("max_connections")
		if maxConnectionsErr != nil || maxConnections < 1 || maxConnections > 100 {
			return nil, http.StatusBadRequest, 400, "Bad Request: invalid max_connections"
		}
	}
	secretToken := request.string("secret_token")
	if len(secretToken) > 256 || !validWebhookSecretToken(secretToken) {
		return nil, http.StatusBadRequest, 400, "Bad Request: secret_token is invalid"
	}
	allowed := []string{}
	if err := request.json("allowed_updates", &allowed); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid allowed_updates"
	}
	dropPending, err := request.bool("drop_pending_updates")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid drop_pending_updates"
	}
	if err := ctx.BotAPIStore.SetWebhook(requestCtx, botID, store.Webhook{
		URL:            webhookURL,
		MaxConnections: maxConnections,
		AllowedUpdates: allowed,
		SecretToken:    secretToken,
	}, dropPending); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteWebhook(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	dropPending, err := request.bool("drop_pending_updates")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid drop_pending_updates"
	}
	if err := ctx.BotAPIStore.DeleteWebhook(requestCtx, botID, dropPending); err != nil {
		return nil, http.StatusInternalServerError, 500, err.Error()
	}
	return true, http.StatusOK, 0, ""
}

func sendMessage(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	chatID, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, bot.GetUser().GetId(), request.string("chat_id"))
	if err != nil || chatID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	text := request.string("text")
	if text == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: text is empty"
	}
	randomID := randomMessageID()
	if value, parseErr := request.int64("random_id"); parseErr == nil && value != 0 {
		randomID = value
	}
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		FromId:  mtproto.MakePeerUser(bot.GetUser().GetId()),
		PeerId:  mtproto.MakePeerUtil(peerType, peerID).ToPeer(),
		Date:    int32(time.Now().Unix()),
		Message: text,
	}).To_Message()
	var entities []botapi.MessageEntity
	if err := request.json("entities", &entities); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: entities is invalid"
	}
	message.Entities = botapi.MessageEntitiesToMTProto(entities)
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	message.ReplyMarkup = markup
	noWebpage, err := botAPINoWebpage(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
	}
	if replyTo, parseErr := request.int("reply_to_message_id"); parseErr == nil && replyTo > 0 {
		message.ReplyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
			ReplyToMsgId:           int32(replyTo),
			ReplyToMsgId_INT32:     int32(replyTo),
			ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(int32(replyTo)),
		}).To_MessageReplyHeader()
	}
	updates, err := ctx.MsgClient.MsgSendMessageV2(botRPCContext(r.Context(), bot.GetUser().GetId()), &msgpb.TLMsgSendMessageV2{
		UserId:   bot.GetUser().GetId(),
		PeerType: peerType,
		PeerId:   peerID,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage: noWebpage,
			RandomId:  randomID,
			Message:   message,
		}).To_OutboxMessage()},
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result := botapi.FirstMessageFromUpdates(updates)
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: message was not returned"
	}
	_ = chatID
	return result, http.StatusOK, 0, ""
}

func forwardMessage(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest, copyMode bool) (any, int, int, string) {
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	botID := bot.GetUser().GetId()
	_, toPeerType, toPeerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || toPeerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	_, fromPeerType, fromPeerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("from_chat_id"))
	if err != nil || fromPeerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: from_chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	videoTimestamp, err := request.int("video_start_timestamp")
	if err != nil || videoTimestamp < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: video_start_timestamp is invalid"
	}
	forward := &mtproto.TLMessagesForwardMessages{
		FromPeer:   botAPIInputPeer(botID, fromPeerType, fromPeerID),
		Id:         []int32{int32(messageID)},
		RandomId:   []int64{randomMessageID()},
		ToPeer:     botAPIInputPeer(botID, toPeerType, toPeerID),
		Silent:     requestBool(request, "disable_notification"),
		Noforwards: requestBool(request, "protect_content"),
		ReplyTo:    replyTo,
	}
	if copyMode {
		forward.DropAuthor = true
	}
	if videoTimestamp > 0 {
		forward.VideoTimestamp = wrapperspb.Int32(int32(videoTimestamp))
	}
	updates, err := ctx.MessagesClient.MessagesForwardMessages(botMediaRPCContext(r.Context(), botID), forward)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result := botapi.FirstMessageFromUpdates(updates)
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: message was not returned"
	}
	if copyMode {
		return &botapi.MessageID{MessageID: result.MessageID}, http.StatusOK, 0, ""
	}
	return result, http.StatusOK, 0, ""
}

func botAPIReplyTo(request *botAPIRequest) (*mtproto.InputReplyTo, error) {
	messageID, err := request.int("reply_to_message_id")
	if err != nil || messageID < 0 {
		return nil, err
	}
	if raw := request.raw("reply_parameters"); len(raw) > 0 && string(raw) != "null" {
		var parameters struct {
			MessageID int `json:"message_id"`
		}
		if err := request.json("reply_parameters", &parameters); err != nil {
			return nil, err
		}
		if parameters.MessageID <= 0 {
			return nil, errors.New("reply_parameters.message_id is required")
		}
		messageID = parameters.MessageID
	}
	if messageID == 0 {
		return nil, nil
	}
	return mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: int32(messageID),
	}).To_InputReplyTo(), nil
}

// sendBotAPIMedia accepts both an existing Bot API file_id and a multipart
// upload. Uploaded bytes are stored through DFS and then passed to the
// existing messages.sendMedia pipeline, which owns media validation and
// persistence.
func sendBotAPIMedia(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest, field, kind string) (any, int, int, string) {
	chatID, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, bot.GetUser().GetId(), request.string("chat_id"))
	if err != nil || chatID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: media service is not configured"
	}
	duration, err := request.int("duration")
	if err != nil || duration < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: duration is invalid"
	}
	width, err := request.int("width")
	if err != nil || width < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: width is invalid"
	}
	height, err := request.int("height")
	if err != nil || height < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: height is invalid"
	}
	if kind == "video_note" {
		length, lengthErr := request.int("length")
		if lengthErr != nil || length < 0 || length > 640 {
			return nil, http.StatusBadRequest, 400, "Bad Request: length is invalid"
		}
		width, height = length, length
	}

	var media *mtproto.InputMedia
	if r != nil && strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: invalid multipart form"
		}
		if value := r.FormValue(field); value != "" {
			request.values[field] = json.RawMessage(value)
		}
	}
	if file, header, fileErr := r.FormFile(field); fileErr == nil {
		defer file.Close()
		name := filepath.Base(header.Filename)
		if name == "." || name == "" {
			name = field
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 64<<20+1))
		if readErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: failed to read uploaded file"
		}
		if len(data) > 64<<20 {
			return nil, http.StatusRequestEntityTooLarge, 413, "Bad Request: file is too large"
		}
		inputFile, uploadErr := uploadBotAPIFile(r.Context(), ctx, bot.GetUser().GetId(), name, data)
		if uploadErr != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(uploadErr)
		}
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(name))
		}
		media = botAPIUploadedMedia(kind, inputFile, name, mimeType, duration, width, height, request)
	} else {
		fileID := request.string(field)
		if fileID == "" {
			return nil, http.StatusBadRequest, 400, "Bad Request: media file is required"
		}
		media, err = botAPIExistingMedia(kind, fileID, request)
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: file_id is invalid"
		}
	}

	caption := request.string("caption")
	var entities []botapi.MessageEntity
	if err := request.json("caption_entities", &entities); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: caption_entities is invalid"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	replyTo, err := request.int("reply_to_message_id")
	if err != nil || replyTo < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_to_message_id is invalid"
	}
	var replyToMsgID *wrapperspb.Int32Value
	if replyTo > 0 {
		replyToMsgID = wrapperspb.Int32(int32(replyTo))
	}
	updates, err := ctx.MessagesClient.MessagesSendMedia(botMediaRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLMessagesSendMedia{
		Peer:         botAPIInputPeer(bot.GetUser().GetId(), peerType, peerID),
		Media:        media,
		Message:      caption,
		RandomId:     randomMessageID(),
		ReplyMarkup:  markup,
		Entities:     botapi.MessageEntitiesToMTProto(entities),
		ReplyToMsgId: replyToMsgID,
		Noforwards:   requestBool(request, "protect_content"),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result := botapi.FirstMessageFromUpdates(updates); result != nil {
		return result, http.StatusOK, 0, ""
	}
	return nil, http.StatusInternalServerError, 500, "Internal Server Error: message was not returned"
}

func sendSticker(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if _, err := botAPIMaskCoords(request); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: mask_position is invalid"
	}
	return sendBotAPIMedia(ctx, r, bot, request, "sticker", "sticker")
}

func botAPIMaskCoords(request *botAPIRequest) (*mtproto.MaskCoords, error) {
	if request == nil || len(request.raw("mask_position")) == 0 || string(request.raw("mask_position")) == "null" {
		return nil, nil
	}
	var position botapi.MaskPosition
	if err := request.json("mask_position", &position); err != nil {
		return nil, err
	}
	var point int32
	switch strings.ToLower(strings.TrimSpace(position.Point)) {
	case "forehead":
		point = 0
	case "eyes":
		point = 1
	case "mouth":
		point = 2
	case "chin":
		point = 3
	default:
		return nil, errors.New("invalid mask point")
	}
	if position.Scale <= 0 {
		return nil, errors.New("invalid mask scale")
	}
	return mtproto.MakeTLMaskCoords(&mtproto.MaskCoords{
		N: point, X: position.XShift, Y: position.YShift, Zoom: position.Scale,
	}).To_MaskCoords(), nil
}

func sendLocation(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	longitude, err := request.float64("longitude")
	if err != nil || longitude < -180 || longitude > 180 {
		return nil, http.StatusBadRequest, 400, "Bad Request: longitude is invalid"
	}
	latitude, err := request.float64("latitude")
	if err != nil || latitude < -90 || latitude > 90 {
		return nil, http.StatusBadRequest, 400, "Bad Request: latitude is invalid"
	}
	horizontalAccuracy, err := request.float64("horizontal_accuracy")
	if err != nil || horizontalAccuracy < 0 || horizontalAccuracy > 1500 {
		return nil, http.StatusBadRequest, 400, "Bad Request: horizontal_accuracy is invalid"
	}
	livePeriod, err := request.int("live_period")
	if err != nil || livePeriod < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: live_period is invalid"
	}
	heading, err := request.int("heading")
	if err != nil || heading < 0 || heading > 360 {
		return nil, http.StatusBadRequest, 400, "Bad Request: heading is invalid"
	}
	proximity, err := request.int("proximity_alert_radius")
	if err != nil || proximity < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: proximity_alert_radius is invalid"
	}
	if livePeriod == 0 && (heading != 0 || proximity != 0) {
		return nil, http.StatusBadRequest, 400, "Bad Request: live_period is required for live location options"
	}
	geo := &mtproto.InputGeoPoint{Lat: latitude, Long: longitude}
	if horizontalAccuracy > 0 {
		geo.AccuracyRadius = wrapperspb.Int32(int32(horizontalAccuracy))
	}
	media := &mtproto.InputMedia{GeoPoint: mtproto.MakeTLInputGeoPoint(geo).To_InputGeoPoint()}
	if livePeriod > 0 {
		media.Period = wrapperspb.Int32(int32(livePeriod))
		if heading != 0 {
			media.Heading = wrapperspb.Int32(int32(heading))
		}
		if proximity != 0 {
			media.ProximityNotificationRadius = wrapperspb.Int32(int32(proximity))
		}
		media = mtproto.MakeTLInputMediaGeoLive(media).To_InputMedia()
	} else {
		media = mtproto.MakeTLInputMediaGeoPoint(media).To_InputMedia()
	}
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func sendVenue(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	longitude, latitude, err := botAPILocationCoordinates(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, err.Error()
	}
	title := request.string("title")
	address := request.string("address")
	if title == "" || address == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: title and address are required"
	}
	media := mtproto.MakeTLInputMediaVenue(&mtproto.InputMedia{
		GeoPoint:        mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: latitude, Long: longitude}).To_InputGeoPoint(),
		Title:           title,
		Address:         address,
		Provider_STRING: request.string("foursquare_id"),
		VenueId:         request.string("foursquare_id"),
		VenueType:       request.string("foursquare_type"),
	}).To_InputMedia()
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func sendContact(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	phoneNumber := request.string("phone_number")
	firstName := request.string("first_name")
	if phoneNumber == "" || firstName == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: phone_number and first_name are required"
	}
	media := mtproto.MakeTLInputMediaContact(&mtproto.InputMedia{
		PhoneNumber: phoneNumber,
		FirstName:   firstName,
		LastName:    request.string("last_name"),
		Vcard:       request.string("vcard"),
	}).To_InputMedia()
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func sendPoll(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	question := request.string("question")
	if question == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: question is required"
	}
	if len([]rune(question)) > 300 {
		return nil, http.StatusBadRequest, 400, "Bad Request: question is too long"
	}
	var options []string
	if err := request.json("options", &options); err != nil || len(options) < 2 || len(options) > 10 {
		return nil, http.StatusBadRequest, 400, "Bad Request: options must contain between 2 and 10 strings"
	}
	isAnonymous := true
	if len(request.raw("is_anonymous")) > 0 {
		var err error
		isAnonymous, err = request.bool("is_anonymous")
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: is_anonymous is invalid"
		}
	}
	pollType := request.string("type")
	if pollType == "" {
		pollType = "regular"
	}
	if pollType != "regular" && pollType != "quiz" {
		return nil, http.StatusBadRequest, 400, "Bad Request: type is invalid"
	}
	allowsMultiple, err := request.bool("allows_multiple_answers")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: allows_multiple_answers is invalid"
	}
	correctOption := -1
	if raw := request.raw("correct_option_id"); len(raw) > 0 {
		correctOption, err = request.int("correct_option_id")
		if err != nil || correctOption < 0 || correctOption >= len(options) {
			return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_id is invalid"
		}
	}
	if pollType == "quiz" && correctOption < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_id is required for quiz polls"
	}
	if pollType == "regular" && correctOption >= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_id is only valid for quiz polls"
	}
	openPeriod, err := request.int("open_period")
	if err != nil || openPeriod < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: open_period is invalid"
	}
	closeDate, err := request.int64("close_date")
	if err != nil || closeDate < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: close_date is invalid"
	}
	if openPeriod > 0 && closeDate > 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: open_period and close_date are mutually exclusive"
	}
	answers := make([]*mtproto.PollAnswer, 0, len(options))
	for index, option := range options {
		if strings.TrimSpace(option) == "" || len([]rune(option)) > 100 {
			return nil, http.StatusBadRequest, 400, "Bad Request: poll option is invalid"
		}
		answers = append(answers, mtproto.MakePollAnswerWithText(option, []byte{byte(index)}))
	}
	poll := &mtproto.Poll{
		Id:              randomMessageID(),
		PublicVoters:    !isAnonymous,
		MultipleChoice:  allowsMultiple,
		Quiz:            pollType == "quiz",
		Question:        question,
		Question_STRING: question,
		Answers:         answers,
	}
	poll.Question_TEXTWITHENTITIES = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text: question,
	}).To_TextWithEntities()
	if openPeriod > 0 {
		poll.ClosePeriod = wrapperspb.Int32(int32(openPeriod))
	}
	if closeDate > 0 {
		poll.CloseDate = wrapperspb.Int32(int32(closeDate))
	}
	media := mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{
		Poll: poll,
	}).To_InputMedia()
	if correctOption >= 0 {
		media.CorrectAnswers_FLAGVECTORINT32 = []int32{int32(correctOption)}
	}
	if explanation := request.string("explanation"); explanation != "" {
		media.Solution = wrapperspb.String(explanation)
		var entities []botapi.MessageEntity
		if err := request.json("explanation_entities", &entities); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: explanation_entities is invalid"
		}
		media.SolutionEntities = botapi.MessageEntitiesToMTProto(entities)
	}
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func sendDice(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	emoji := request.string("emoji")
	if emoji == "" {
		emoji = "🎲"
	}
	switch emoji {
	case "🎲", "🎯", "🏀", "⚽", "🎳", "🎰":
	default:
		return nil, http.StatusBadRequest, 400, "Bad Request: emoji is invalid"
	}
	media := mtproto.MakeTLInputMediaDice(&mtproto.InputMedia{Emoticon: emoji}).To_InputMedia()
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func sendBotAPIStructuredMedia(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest, media *mtproto.InputMedia, caption string) (any, int, int, string) {
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: media service is not configured"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, bot.GetUser().GetId(), request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if caption == "" {
		caption = request.string("caption")
	}
	var entities []botapi.MessageEntity
	if err := request.json("caption_entities", &entities); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: caption_entities is invalid"
	}
	var markup botapi.ReplyMarkupInput
	if err := request.json("reply_markup", &markup); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	updates, err := ctx.MessagesClient.MessagesSendMedia(botMediaRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLMessagesSendMedia{
		Peer:        botAPIInputPeer(bot.GetUser().GetId(), peerType, peerID),
		Media:       media,
		Message:     caption,
		RandomId:    randomMessageID(),
		ReplyTo:     replyTo,
		ReplyMarkup: botapi.ReplyMarkupToMTProto(markup),
		Entities:    botapi.MessageEntitiesToMTProto(entities),
		Silent:      requestBool(request, "disable_notification"),
		Noforwards:  requestBool(request, "protect_content"),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result := botapi.FirstMessageFromUpdates(updates)
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: message was not returned"
	}
	return result, http.StatusOK, 0, ""
}

type botAPIMediaGroupItem struct {
	Type              string                 `json:"type"`
	Media             string                 `json:"media"`
	Caption           string                 `json:"caption,omitempty"`
	CaptionEntities   []botapi.MessageEntity `json:"caption_entities,omitempty"`
	HasSpoiler        bool                   `json:"has_spoiler,omitempty"`
	SupportsStreaming bool                   `json:"supports_streaming,omitempty"`
	Duration          int                    `json:"duration,omitempty"`
	Width             int                    `json:"width,omitempty"`
	Height            int                    `json:"height,omitempty"`
}

func sendMediaGroup(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: media service is not configured"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, bot.GetUser().GetId(), request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	var items []botAPIMediaGroupItem
	if err := request.json("media", &items); err != nil || len(items) < 2 || len(items) > 10 {
		return nil, http.StatusBadRequest, 400, "Bad Request: media must contain between 2 and 10 items"
	}
	multiMedia := make([]*mtproto.InputSingleMedia, 0, len(items))
	for _, item := range items {
		media, err := botAPIMediaGroupInput(r, ctx, bot.GetUser().GetId(), item)
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: media item is invalid"
		}
		multiMedia = append(multiMedia, mtproto.MakeTLInputSingleMedia(&mtproto.InputSingleMedia{
			Media:    media,
			RandomId: randomMessageID(),
			Message:  item.Caption,
			Entities: botapi.MessageEntitiesToMTProto(item.CaptionEntities),
		}).To_InputSingleMedia())
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	updates, err := ctx.MessagesClient.MessagesSendMultiMedia(botMediaRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLMessagesSendMultiMedia{
		Peer:       botAPIInputPeer(bot.GetUser().GetId(), peerType, peerID),
		ReplyTo:    replyTo,
		MultiMedia: multiMedia,
		Silent:     requestBool(request, "disable_notification"),
		Noforwards: requestBool(request, "protect_content"),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result := botapi.MessagesFromUpdates(updates)
	if len(result) == 0 {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: messages were not returned"
	}
	return result, http.StatusOK, 0, ""
}

func botAPILocationCoordinates(request *botAPIRequest) (float64, float64, error) {
	longitude, err := request.float64("longitude")
	if err != nil || longitude < -180 || longitude > 180 {
		return 0, 0, errors.New("Bad Request: longitude is invalid")
	}
	latitude, err := request.float64("latitude")
	if err != nil || latitude < -90 || latitude > 90 {
		return 0, 0, errors.New("Bad Request: latitude is invalid")
	}
	return longitude, latitude, nil
}

func botAPIMediaGroupInput(r *http.Request, ctx *svc.ServiceContext, botID int64, item botAPIMediaGroupItem) (*mtproto.InputMedia, error) {
	item.Type = strings.ToLower(strings.TrimSpace(item.Type))
	if item.Type != "photo" && item.Type != "video" && item.Type != "document" && item.Type != "audio" && item.Type != "animation" {
		return nil, errors.New("unsupported media type")
	}
	item.Media = strings.TrimSpace(item.Media)
	if item.Media == "" {
		return nil, errors.New("media is required")
	}
	if strings.HasPrefix(item.Media, "attach://") {
		field := strings.TrimPrefix(item.Media, "attach://")
		if field == "" {
			return nil, errors.New("attachment is empty")
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, err
		}
		file, header, err := r.FormFile(field)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, 64<<20+1))
		if err != nil || len(data) == 0 || len(data) > 64<<20 {
			return nil, errors.New("attachment is invalid")
		}
		name := filepath.Base(header.Filename)
		if name == "." || name == "" {
			name = item.Type
		}
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(name))
		}
		fileRef, err := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
		if err != nil {
			return nil, err
		}
		return botAPIUploadedMedia(item.Type, fileRef, name, mimeType, item.Duration, item.Width, item.Height, &botAPIRequest{values: map[string]json.RawMessage{"has_spoiler": json.RawMessage(strconv.FormatBool(item.HasSpoiler)), "supports_streaming": json.RawMessage(strconv.FormatBool(item.SupportsStreaming))}}), nil
	}
	if strings.HasPrefix(item.Media, "http://") || strings.HasPrefix(item.Media, "https://") {
		if item.Type == "photo" {
			return mtproto.MakeTLInputMediaPhotoExternal(&mtproto.InputMedia{Url: item.Media, Spoiler: item.HasSpoiler}).To_InputMedia(), nil
		}
		return mtproto.MakeTLInputMediaDocumentExternal(&mtproto.InputMedia{Url: item.Media, Spoiler: item.HasSpoiler}).To_InputMedia(), nil
	}
	return botAPIExistingMedia(item.Type, item.Media, &botAPIRequest{values: map[string]json.RawMessage{"has_spoiler": json.RawMessage(strconv.FormatBool(item.HasSpoiler))}})
}

func requestBool(request *botAPIRequest, name string) bool {
	value, err := request.bool(name)
	return err == nil && value
}

func botAPINoWebpage(request *botAPIRequest) (bool, error) {
	disabled, err := request.bool("disable_web_page_preview")
	if err != nil {
		return false, err
	}
	if raw := request.raw("link_preview_options"); len(raw) > 0 && string(raw) != "null" {
		var options struct {
			IsDisabled bool `json:"is_disabled"`
		}
		if err := request.json("link_preview_options", &options); err != nil {
			return false, err
		}
		disabled = disabled || options.IsDisabled
	}
	return disabled, nil
}

func botAPIReplyMarkup(request *botAPIRequest) (*mtproto.ReplyMarkup, error) {
	if request == nil || len(request.raw("reply_markup")) == 0 || string(request.raw("reply_markup")) == "null" {
		return nil, nil
	}
	var markup botapi.ReplyMarkupInput
	if err := request.json("reply_markup", &markup); err != nil {
		return nil, err
	}
	var shape struct {
		InlineKeyboard json.RawMessage `json:"inline_keyboard"`
	}
	if err := request.json("reply_markup", &shape); err != nil {
		return nil, err
	}
	if len(shape.InlineKeyboard) > 0 && string(shape.InlineKeyboard) != "null" {
		var rows [][]botapi.InlineKeyboardButton
		if err := json.Unmarshal(shape.InlineKeyboard, &rows); err != nil {
			return nil, err
		}
		markup.InlineKeyboard = rows
		if len(rows) == 0 {
			return mtproto.MakeTLReplyInlineMarkup(&mtproto.ReplyMarkup{Rows: []*mtproto.KeyboardButtonRow{}}).To_ReplyMarkup(), nil
		}
	}
	return botapi.ReplyMarkupToMTProto(markup), nil
}

func botAPIInputPeer(selfID int64, peerType int32, peerID int64) *mtproto.InputPeer {
	switch peerType {
	case mtproto.PEER_USER:
		if peerID == selfID {
			return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
		}
		return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID}).To_InputPeer()
	case mtproto.PEER_CHANNEL:
		return mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: peerID, AccessHash: peerID}).To_InputPeer()
	default:
		return mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: peerID}).To_InputPeer()
	}
}

func botAPIExistingMedia(kind, fileID string, request *botAPIRequest) (*mtproto.InputMedia, error) {
	ref, err := parseBotAPIFileID(fileID)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "photo":
		if ref.kind != "photo" {
			return nil, errors.New("file is not a photo")
		}
		return mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{
			Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: ref.id, AccessHash: ref.accessHash}).To_InputPhoto(),
			Spoiler:       requestBool(request, "has_spoiler"),
		}).To_InputMedia(), nil
	case "document", "video", "video_note", "sticker", "audio", "voice", "animation":
		if ref.kind != "document" {
			return nil, errors.New("file is not a document")
		}
		return mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
			Id_INPUTDOCUMENT: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: ref.id, AccessHash: ref.accessHash}).To_InputDocument(),
		}).To_InputMedia(), nil
	default:
		return nil, errors.New("unsupported media kind")
	}
}

func botAPIUploadedMedia(kind string, file *mtproto.InputFile, name, mimeType string, duration, width, height int, request *botAPIRequest) *mtproto.InputMedia {
	if kind == "photo" {
		return mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File:    file,
			Spoiler: requestBool(request, "has_spoiler"),
		}).To_InputMedia()
	}
	attributes := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
	}
	switch kind {
	case "animation":
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeAnimated(nil).To_DocumentAttribute())
	case "video":
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			W:                 int32(width),
			H:                 int32(height),
			Duration_INT32:    int32(duration),
			SupportsStreaming: requestBool(request, "supports_streaming"),
		}).To_DocumentAttribute())
	case "video_note":
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			W:              int32(width),
			H:              int32(height),
			Duration_INT32: int32(duration),
			RoundMessage:   true,
		}).To_DocumentAttribute())
	case "sticker":
		mask, _ := botAPIMaskCoords(request)
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{
			Alt:        request.string("emoji"),
			Mask:       mask != nil,
			MaskCoords: mask,
			Stickerset: mtproto.MakeTLInputStickerSetEmpty(nil).To_InputStickerSet(),
		}).To_DocumentAttribute())
	case "audio", "voice":
		attribute := &mtproto.DocumentAttribute{Voice: kind == "voice", Duration_INT32: int32(duration)}
		if kind == "audio" {
			attribute.Title = wrapperspb.String(request.string("title"))
			attribute.Performer = wrapperspb.String(request.string("performer"))
		}
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeAudio(attribute).To_DocumentAttribute())
	}
	return mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
		File:       file,
		MimeType:   mimeType,
		Attributes: attributes,
		ForceFile:  kind == "document",
	}).To_InputMedia()
}

func uploadBotAPIFile(ctx context.Context, service *svc.ServiceContext, botID int64, name string, data []byte) (*mtproto.InputFile, error) {
	if service == nil || service.DfsClient == nil || botID <= 0 || len(data) == 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	const partSize = 512 << 10
	parts := (len(data) + partSize - 1) / partSize
	if parts <= 0 || parts > 3000 {
		return nil, mtproto.ErrFilePartsInvalid
	}
	fileID := randomMessageID()
	if fileID <= 0 {
		fileID = 1
	}
	big := len(data) > 10<<20
	for part := 0; part < parts; part++ {
		start := part * partSize
		end := start + partSize
		if end > len(data) {
			end = len(data)
		}
		_, err := service.DfsClient.DfsWriteFilePartData(botRPCContext(ctx, botID), &dfspb.TLDfsWriteFilePartData{
			Creator:        botID,
			FileId:         fileID,
			FilePart:       int32(part),
			Bytes:          data[start:end],
			Big:            big,
			FileTotalParts: wrapperspb.Int32(int32(parts)),
		})
		if err != nil {
			return nil, err
		}
	}
	input := &mtproto.InputFile{Id_INT64: fileID, Parts: int32(parts), Name: name}
	if big {
		return mtproto.MakeTLInputFileBig(input).To_InputFile(), nil
	}
	return mtproto.MakeTLInputFile(input).To_InputFile(), nil
}

func setChatMenuButton(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	if raw := request.raw("menu_button"); len(raw) == 0 || string(raw) == "null" {
		return nil, http.StatusBadRequest, 400, "Bad Request: menu_button is required"
	}
	var button botapi.MenuButton
	if err := request.json("menu_button", &button); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: menu_button is invalid"
	}
	chatID, err := optionalBotAPIChatID(requestCtx, ctx, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if err := ctx.BotAPIStore.SetBotMenuButton(requestCtx, botID, chatID, button); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func validWebhookSecretToken(value string) bool {
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func getChatMenuButton(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	chatID, err := optionalBotAPIChatID(requestCtx, ctx, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	button, err := ctx.BotAPIStore.GetBotMenuButton(requestCtx, botID, chatID)
	if err != nil {
		return nil, http.StatusInternalServerError, 500, err.Error()
	}
	return button, http.StatusOK, 0, ""
}

func optionalBotAPIChatID(requestCtx context.Context, service *svc.ServiceContext, botID int64, request *botAPIRequest) (int64, error) {
	if strings.TrimSpace(request.string("chat_id")) == "" {
		return 0, nil
	}
	chatID, _, _, err := resolveBotAPIChatID(requestCtx, service, botID, request.string("chat_id"))
	if err != nil || chatID == 0 {
		return 0, errors.New("invalid chat id")
	}
	return chatID, nil
}

func setMyName(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	name := request.string("name")
	if name == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is required"
	}
	if ctx.BotRegistryClient == nil {
		return nil, http.StatusNotImplemented, 501, "Bad Request: bot profile service is not configured"
	}
	if _, err := ctx.BotRegistryClient.SetBotInfo(botRPCContext(requestCtx, botID), &userpb.BotRegistrySetBotInfoRequest{BotId: botID, Name: wrapperspb.String(name)}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setMyDescription(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx.BotRegistryClient == nil {
		return nil, http.StatusNotImplemented, 501, "Bad Request: bot profile service is not configured"
	}
	description := request.string("description")
	if _, err := ctx.BotRegistryClient.SetBotInfo(botRPCContext(requestCtx, botID), &userpb.BotRegistrySetBotInfoRequest{BotId: botID, Description: wrapperspb.String(description)}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func getMyDescription(ctx *svc.ServiceContext, requestCtx context.Context, botID int64) (any, int, int, string) {
	info, err := ctx.UserClient.UserGetBotInfo(botRPCContext(requestCtx, botID), &userpb.TLUserGetBotInfo{BotId: botID})
	if err != nil || info == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return &botapi.BotDescription{Description: info.GetDescription_STRING()}, http.StatusOK, 0, ""
}

func setMyShortDescription(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx.BotRegistryClient == nil {
		return nil, http.StatusNotImplemented, 501, "Bad Request: bot profile service is not configured"
	}
	description := request.string("short_description")
	if _, err := ctx.BotRegistryClient.SetBotInfo(botRPCContext(requestCtx, botID), &userpb.BotRegistrySetBotInfoRequest{BotId: botID, About: wrapperspb.String(description)}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func getMyShortDescription(ctx *svc.ServiceContext, requestCtx context.Context, botID int64) (any, int, int, string) {
	description, err := ctx.BotAPIStore.GetBotAbout(requestCtx, botID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return &botapi.BotShortDescription{ShortDescription: description}, http.StatusOK, 0, ""
}

func editMessageText(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	text := request.string("text")
	if text == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: text is empty"
	}
	var entities []botapi.MessageEntity
	if err := request.json("entities", &entities); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: entities is invalid"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessage(ctx, r, bot, request, text, entities, markup)
}

func editMessageCaption(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	caption := request.string("caption")
	if len(request.raw("caption")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: caption is empty"
	}
	var entities []botapi.MessageEntity
	if err := request.json("caption_entities", &entities); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: caption_entities is invalid"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessageWithMedia(ctx, r, bot, request, caption, true, entities, markup, nil)
}

func editMessageMedia(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if len(request.raw("media")) == 0 || string(request.raw("media")) == "null" {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is required"
	}
	var item botAPIMediaGroupItem
	if err := request.json("media", &item); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is invalid"
	}
	media, err := botAPIMediaGroupInput(r, ctx, bot.GetUser().GetId(), item)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is invalid"
	}
	var mediaObject map[string]json.RawMessage
	if err := request.json("media", &mediaObject); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is invalid"
	}
	_, hasCaption := mediaObject["caption"]
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessageWithMedia(ctx, r, bot, request, item.Caption, hasCaption, item.CaptionEntities, markup, media)
}

func editMessageReplyMarkup(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if len(request.raw("reply_markup")) == 0 || string(request.raw("reply_markup")) == "null" {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is required"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessage(ctx, r, bot, request, "", nil, markup)
}

func editBotAPIMessage(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest, text string, entities []botapi.MessageEntity, markup *mtproto.ReplyMarkup) (any, int, int, string) {
	return editBotAPIMessageWithMedia(ctx, r, bot, request, text, text != "", entities, markup, nil)
}

func editBotAPIMessageWithMedia(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest, text string, setText bool, entities []botapi.MessageEntity, markup *mtproto.ReplyMarkup, media *mtproto.InputMedia) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, bot.GetUser().GetId(), request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	noWebpage, err := botAPINoWebpage(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
	}
	edit := &mtproto.TLMessagesEditMessage{
		NoWebpage:   noWebpage,
		Peer:        botAPIInputPeer(bot.GetUser().GetId(), peerType, peerID),
		Id:          int32(messageID),
		ReplyMarkup: markup,
		Media:       media,
	}
	if entities != nil {
		edit.Entities = botapi.MessageEntitiesToMTProto(entities)
	}
	if setText {
		edit.Message = wrapperspb.String(text)
	}
	updates, err := ctx.MessagesClient.MessagesEditMessage(botRPCContext(r.Context(), bot.GetUser().GetId()), edit)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result := botapi.FirstMessageFromUpdates(updates); result != nil {
		return result, http.StatusOK, 0, ""
	}
	return true, http.StatusOK, 0, ""
}

func setMessageReaction(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	isBig, err := request.bool("is_big")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_big is invalid"
	}
	var requested []botapi.ReactionType
	if raw := request.raw("reaction"); len(raw) > 0 && string(raw) != "null" {
		if err := request.json("reaction", &requested); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: reaction is invalid"
		}
	}
	if len(requested) > 1 {
		return nil, http.StatusBadRequest, 400, "Bad Request: reaction can contain at most one item"
	}
	reactions := make([]*mtproto.Reaction, 0, len(requested))
	if len(requested) == 1 {
		item := requested[0]
		switch item.Type {
		case "emoji":
			if item.Emoji == "" {
				return nil, http.StatusBadRequest, 400, "Bad Request: emoji reaction is empty"
			}
			reactions = append(reactions, mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: item.Emoji}).To_Reaction())
		case "custom_emoji":
			documentID, parseErr := strconv.ParseInt(item.CustomEmojiID, 10, 64)
			if parseErr != nil || documentID <= 0 {
				return nil, http.StatusBadRequest, 400, "Bad Request: custom_emoji_id is invalid"
			}
			reactions = append(reactions, mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: documentID}).To_Reaction())
		default:
			return nil, http.StatusBadRequest, 400, "Bad Request: reaction type is invalid"
		}
	}
	if ctx == nil || ctx.ReactionsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: reaction service is not configured"
	}
	if _, err := ctx.ReactionsClient.MessagesSendReaction(botRPCContext(r.Context(), botID), &mtproto.TLMessagesSendReaction{
		Big:                         isBig,
		Peer:                        botAPIInputPeer(botID, peerType, peerID),
		MsgId:                       int32(messageID),
		Reaction_FLAGVECTORREACTION: reactions,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteMessage(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	if _, err := ctx.MsgClient.MsgDeleteMessages(botRPCContext(r.Context(), botID), &msgpb.TLMsgDeleteMessages{UserId: botID, PeerType: peerType, PeerId: peerID, Revoke: true, Id: []int32{int32(messageID)}}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func answerCallbackQuery(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	queryID, err := strconv.ParseInt(strings.TrimSpace(request.string("callback_query_id")), 10, 64)
	if err != nil || queryID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: callback_query_id is required"
	}
	if ctx == nil || ctx.InlineBotClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: callback answer service is not configured"
	}
	cacheTime, err := request.int("cache_time")
	if err != nil || cacheTime < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: cache_time is invalid"
	}
	showAlert, err := request.bool("show_alert")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: show_alert is invalid"
	}
	answer := &mtproto.TLMessagesSetBotCallbackAnswer{
		QueryId:   queryID,
		Alert:     showAlert,
		CacheTime: int32(cacheTime),
	}
	if message := request.string("text"); message != "" {
		answer.Message = wrapperspb.String(message)
	}
	if url := request.string("url"); url != "" {
		answer.Url = wrapperspb.String(url)
	}
	ok, err := ctx.InlineBotClient.MessagesSetBotCallbackAnswer(botRPCContext(requestCtx, botID), answer)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: callback query is invalid or expired"
	}
	return true, http.StatusOK, 0, ""
}

func answerWebAppQuery(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	queryID := strings.TrimSpace(request.string("web_app_query_id"))
	if queryID == "" || len(queryID) > 128 {
		return nil, http.StatusBadRequest, 400, "Bad Request: web_app_query_id is required"
	}
	if ctx == nil || ctx.MiniBotAppsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: Mini App service is not configured"
	}

	var raw map[string]json.RawMessage
	if err := request.json("result", &raw); err != nil || raw == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: result must be a JSON object"
	}
	inlineResult, err := botAPIWebAppInlineResult(raw)
	if err != nil {
		if errors.Is(err, errBotAPIWebAppResultUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: this Web App inline result is not supported"
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: result is invalid"
	}
	sent, err := ctx.MiniBotAppsClient.MessagesSendWebViewResultMessage(botRPCContext(requestCtx, botID), &mtproto.TLMessagesSendWebViewResultMessage{
		BotQueryId: queryID,
		Result:     inlineResult,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if sent == nil || sent.GetMsgId() == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: Web App message was not returned"
	}
	messageID := sent.GetMsgId()
	return &botapi.SentWebAppMessage{
		InlineMessageID: fmt.Sprintf("%d:%d:%d", messageID.GetDcId(), messageID.GetId_INT64(), messageID.GetAccessHash()),
	}, http.StatusOK, 0, ""
}

func answerShippingQuery(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	queryID, err := request.int64("shipping_query_id")
	if err != nil || queryID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: shipping_query_id is invalid"
	}
	if ctx == nil || ctx.PaymentsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	if len(request.raw("ok")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: ok is required"
	}
	ok, err := request.bool("ok")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: ok is invalid"
	}
	result := &mtproto.TLMessagesSetBotShippingResults{QueryId: queryID}
	if ok {
		var options []botapi.ShippingOption
		if err := request.json("shipping_options", &options); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: shipping_options is invalid"
		}
		result.ShippingOptions = make([]*mtproto.ShippingOption, 0, len(options))
		for _, option := range options {
			if strings.TrimSpace(option.ID) == "" || strings.TrimSpace(option.Title) == "" {
				return nil, http.StatusBadRequest, 400, "Bad Request: shipping option is invalid"
			}
			prices := make([]*mtproto.LabeledPrice, 0, len(option.Prices))
			for _, price := range option.Prices {
				if strings.TrimSpace(price.Label) == "" {
					return nil, http.StatusBadRequest, 400, "Bad Request: shipping price is invalid"
				}
				prices = append(prices, mtproto.MakeTLLabeledPrice(&mtproto.LabeledPrice{
					Label: price.Label, Amount: price.Amount,
				}).To_LabeledPrice())
			}
			if len(prices) == 0 {
				return nil, http.StatusBadRequest, 400, "Bad Request: shipping option must contain prices"
			}
			result.ShippingOptions = append(result.ShippingOptions, mtproto.MakeTLShippingOption(&mtproto.ShippingOption{
				Id: option.ID, Title: option.Title, Prices: prices,
			}).To_ShippingOption())
		}
	} else {
		errorMessage := request.string("error_message")
		if errorMessage == "" {
			return nil, http.StatusBadRequest, 400, "Bad Request: error_message is required when ok is false"
		}
		result.Error = wrapperspb.String(errorMessage)
	}
	answered, err := ctx.PaymentsClient.MessagesSetBotShippingResults(botRPCContext(requestCtx, botID), result)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !mtproto.FromBool(answered) {
		return nil, http.StatusBadRequest, 400, "Bad Request: shipping query is invalid or expired"
	}
	return true, http.StatusOK, 0, ""
}

func answerPreCheckoutQuery(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	queryID, err := request.int64("pre_checkout_query_id")
	if err != nil || queryID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: pre_checkout_query_id is invalid"
	}
	if ctx == nil || ctx.PaymentsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	if len(request.raw("ok")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: ok is required"
	}
	ok, err := request.bool("ok")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: ok is invalid"
	}
	result := &mtproto.TLMessagesSetBotPrecheckoutResults{QueryId: queryID, Success: ok}
	if !ok {
		errorMessage := request.string("error_message")
		if errorMessage == "" {
			return nil, http.StatusBadRequest, 400, "Bad Request: error_message is required when ok is false"
		}
		result.Error = wrapperspb.String(errorMessage)
	}
	answered, err := ctx.PaymentsClient.MessagesSetBotPrecheckoutResults(botRPCContext(requestCtx, botID), result)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !mtproto.FromBool(answered) {
		return nil, http.StatusBadRequest, 400, "Bad Request: pre-checkout query is invalid or expired"
	}
	return true, http.StatusOK, 0, ""
}

func answerInlineQuery(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	queryID, err := request.int64("inline_query_id")
	if err != nil || queryID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: inline_query_id is invalid"
	}
	if ctx == nil || ctx.InlineBotClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: inline query service is not configured"
	}
	var rawResults []json.RawMessage
	if err := request.json("results", &rawResults); err != nil || len(rawResults) > 50 {
		return nil, http.StatusBadRequest, 400, "Bad Request: results must contain at most 50 JSON objects"
	}
	results := make([]*mtproto.InputBotInlineResult, 0, len(rawResults))
	for _, raw := range rawResults {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: inline result is invalid"
		}
		result, err := botAPIWebAppInlineResult(item)
		if err != nil {
			if errors.Is(err, errBotAPIWebAppResultUnsupported) {
				return nil, http.StatusNotImplemented, 501, "Not Implemented: only article inline results are supported"
			}
			return nil, http.StatusBadRequest, 400, "Bad Request: inline result is invalid"
		}
		results = append(results, result)
	}
	cacheTime := 300
	if len(request.raw("cache_time")) > 0 {
		cacheTime, err = request.int("cache_time")
		if err != nil || cacheTime < 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: cache_time is invalid"
		}
	}
	nextOffset := optionalString(request.string("next_offset"))
	private, err := request.bool("is_personal")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_personal is invalid"
	}
	gallery, err := request.bool("is_gallery")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_gallery is invalid"
	}
	switchPMText := request.string("switch_pm_text")
	switchPMParameter := request.string("switch_pm_parameter")
	var switchPM *mtproto.InlineBotSwitchPM
	if switchPMText != "" || switchPMParameter != "" {
		if switchPMText == "" || switchPMParameter == "" {
			return nil, http.StatusBadRequest, 400, "Bad Request: switch_pm_text and switch_pm_parameter must be provided together"
		}
		switchPM = mtproto.MakeTLInlineBotSwitchPM(&mtproto.InlineBotSwitchPM{Text: switchPMText, StartParam: switchPMParameter}).To_InlineBotSwitchPM()
	}
	ok, err := ctx.InlineBotClient.MessagesSetInlineBotResults(botRPCContext(requestCtx, botID), &mtproto.TLMessagesSetInlineBotResults{
		Gallery: gallery, Private: private, QueryId: queryID, Results: results, CacheTime: int32(cacheTime), NextOffset: nextOffset, SwitchPm: switchPM,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: inline query is invalid or expired"
	}
	return true, http.StatusOK, 0, ""
}

var errBotAPIWebAppResultUnsupported = errors.New("unsupported Web App inline result")

func botAPIWebAppInlineResult(raw map[string]json.RawMessage) (*mtproto.InputBotInlineResult, error) {
	resultType := rawString(raw, "type")
	resultID := rawString(raw, "id")
	if resultType == "" || resultID == "" {
		return nil, errors.New("result type and id are required")
	}
	if resultType != "article" {
		return nil, errBotAPIWebAppResultUnsupported
	}
	content := rawObject(raw, "input_message_content")
	messageText := rawString(content, "message_text")
	if messageText == "" {
		return nil, errors.New("input_message_content.message_text is required")
	}
	var entities []botapi.MessageEntity
	if entityValue, ok := content["entities"]; ok {
		if err := json.Unmarshal(entityValue, &entities); err != nil {
			return nil, err
		}
	}
	var markup botapi.ReplyMarkupInput
	if markupValue, ok := raw["reply_markup"]; ok {
		if err := json.Unmarshal(markupValue, &markup); err != nil {
			return nil, err
		}
	}
	message := mtproto.MakeTLInputBotInlineMessageText(&mtproto.InputBotInlineMessage{
		Message:     messageText,
		Entities:    botapi.MessageEntitiesToMTProto(entities),
		ReplyMarkup: botapi.ReplyMarkupToMTProto(markup),
		NoWebpage:   rawBool(content, "disable_web_page_preview"),
	}).To_InputBotInlineMessage()
	result := mtproto.MakeTLInputBotInlineResult(&mtproto.InputBotInlineResult{
		Id:          resultID,
		Type:        resultType,
		Title:       optionalString(rawString(raw, "title")),
		Description: optionalString(rawString(raw, "description")),
		Url:         optionalString(rawString(raw, "url")),
		SendMessage: message,
	}).To_InputBotInlineResult()
	return result, nil
}

func rawString(values map[string]json.RawMessage, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	var decoded string
	if json.Unmarshal(value, &decoded) == nil {
		return strings.TrimSpace(decoded)
	}
	return ""
}

func rawObject(values map[string]json.RawMessage, key string) map[string]json.RawMessage {
	if len(values) == 0 {
		return nil
	}
	var result map[string]json.RawMessage
	if value, ok := values[key]; ok {
		_ = json.Unmarshal(value, &result)
	}
	return result
}

func rawBool(values map[string]json.RawMessage, key string) bool {
	if len(values) == 0 {
		return false
	}
	var result bool
	if value, ok := values[key]; ok {
		_ = json.Unmarshal(value, &result)
	}
	return result
}

func optionalString(value string) *wrapperspb.StringValue {
	if value == "" {
		return nil
	}
	return wrapperspb.String(value)
}

func getChat(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_USER {
		user, err := ctx.UserClient.UserGetImmutableUser(callCtx, &userpb.TLUserGetImmutableUser{Id: peerID})
		if err != nil || user == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		converted := botapi.UserFromImmutableUser(user)
		return &botapi.Chat{ID: converted.ID, Type: "private", FirstName: converted.FirstName, LastName: converted.LastName, Username: converted.Username}, http.StatusOK, 0, ""
	}
	if peerType == mtproto.PEER_CHAT {
		if ctx.ChatClient == nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: chat is not available"
		}
		chat, err := ctx.ChatClient.ChatGetMutableChat(callCtx, &chatpb.TLChatGetMutableChat{ChatId: peerID})
		if err != nil || chat == nil || chat.GetChat() == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return botapi.ChatFromImmutableChat(chat.GetChat(), "group"), http.StatusOK, 0, ""
	}
	if peerType == mtproto.PEER_CHANNEL && ctx.ChannelsClient != nil {
		result, err := ctx.ChannelsClient.ChannelsGetChannels(callCtx, &mtproto.TLChannelsGetChannels{
			Id: []*mtproto.InputChannel{mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel()},
		})
		if err != nil || result == nil || len(result.GetChats()) == 0 {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		if chat := botapi.ChatFromMTProtoChat(result.GetChats()[0]); chat != nil {
			return chat, http.StatusOK, 0, ""
		}
	}
	return nil, http.StatusBadRequest, 400, "Bad Request: chat is not available"
}

func getChatAdministrators(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	callCtx := botRPCContext(r.Context(), botID)
	result := make([]*botapi.ChatMember, 0)
	if peerType == mtproto.PEER_CHANNEL {
		if ctx == nil || ctx.ChannelsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
		}
		participants, err := ctx.ChannelsClient.ChannelsGetParticipants(callCtx, &mtproto.TLChannelsGetParticipants{
			Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
			Filter:  mtproto.MakeTLChannelParticipantsAdmins(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter(),
			Limit:   200,
		})
		if err != nil || participants == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		for _, participant := range participants.GetParticipants() {
			if member := botapi.ChatMemberFromChannelParticipant(participant, participants.GetUsers()); member != nil {
				result = append(result, member)
			}
		}
		return result, http.StatusOK, 0, ""
	}
	full, err := getBotAPIGroupFullChat(ctx, callCtx, peerID)
	if err != nil || full == nil || full.GetFullChat() == nil || full.GetFullChat().GetParticipants() == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	for _, participant := range full.GetFullChat().GetParticipants().GetParticipants() {
		member := botapi.ChatMemberFromChatParticipant(participant, full.GetUsers())
		if member != nil && (member.Status == "administrator" || member.Status == "creator") {
			result = append(result, member)
		}
	}
	return result, http.StatusOK, 0, ""
}

func getChatMemberCount(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		if ctx == nil || ctx.ChannelsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
		}
		full, err := ctx.ChannelsClient.ChannelsGetFullChannel(callCtx, &mtproto.TLChannelsGetFullChannel{
			Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		})
		if err != nil || full == nil || full.GetFullChat() == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		if count := full.GetFullChat().GetParticipantsCount(); count != nil {
			return int(count.GetValue()), http.StatusOK, 0, ""
		}
		if participants := full.GetFullChat().GetParticipants(); participants != nil {
			return len(participants.GetParticipants()), http.StatusOK, 0, ""
		}
		return 0, http.StatusOK, 0, ""
	}
	full, err := getBotAPIGroupFullChat(ctx, callCtx, peerID)
	if err != nil || full == nil || full.GetFullChat() == nil || full.GetFullChat().GetParticipants() == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return len(full.GetFullChat().GetParticipants().GetParticipants()), http.StatusOK, 0, ""
}

func getChatMember(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_USER {
		if userID != peerID {
			return nil, http.StatusBadRequest, 400, "Bad Request: user is not a member of the chat"
		}
		user, err := ctx.UserClient.UserGetImmutableUser(callCtx, &userpb.TLUserGetImmutableUser{Id: userID})
		if err != nil || user == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return &botapi.ChatMember{Status: "member", User: botapi.UserFromImmutableUser(user), IsMember: true}, http.StatusOK, 0, ""
	}
	if peerType == mtproto.PEER_CHANNEL {
		if ctx == nil || ctx.ChannelsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
		}
		participant, err := ctx.ChannelsClient.ChannelsGetParticipant(callCtx, &mtproto.TLChannelsGetParticipant{
			Channel:     mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
			Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID, AccessHash: userID}).To_InputPeer(),
		})
		if err != nil || participant == nil || participant.GetParticipant() == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return botapi.ChatMemberFromChannelParticipant(participant.GetParticipant(), participant.GetUsers()), http.StatusOK, 0, ""
	}
	full, err := getBotAPIGroupFullChat(ctx, callCtx, peerID)
	if err != nil || full == nil || full.GetFullChat() == nil || full.GetFullChat().GetParticipants() == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	for _, participant := range full.GetFullChat().GetParticipants().GetParticipants() {
		if participant != nil && participant.GetUserId() == userID {
			return botapi.ChatMemberFromChatParticipant(participant, full.GetUsers()), http.StatusOK, 0, ""
		}
	}
	return nil, http.StatusBadRequest, 400, "Bad Request: user is not a member of the chat"
}

func banChatMember(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	untilDate, err := botAPIUntilDate(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: until_date is invalid"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHAT {
		if ctx == nil || ctx.ChatsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
		}
		revoke, err := request.bool("revoke_messages")
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: revoke_messages is invalid"
		}
		_, err = ctx.ChatsClient.MessagesDeleteChatUser(callCtx, &mtproto.TLMessagesDeleteChatUser{
			RevokeHistory: revoke,
			ChatId:        peerID,
			UserId:        mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: userID}).To_InputUser(),
		})
		if err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return true, http.StatusOK, 0, ""
	}
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	_, err = ctx.ChannelsClient.ChannelsEditBanned(callCtx, &mtproto.TLChannelsEditBanned{
		Channel:     mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID, AccessHash: userID}).To_InputPeer(),
		BannedRights: mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
			ViewMessages: true, SendMessages: true, SendMedia: true, SendStickers: true, SendGifs: true,
			SendGames: true, SendInline: true, EmbedLinks: true, SendPolls: true, ChangeInfo: true,
			InviteUsers: true, PinMessages: true, ManageTopics: true, SendPhotos: true, SendVideos: true,
			SendRoundvideos: true, SendAudios: true, SendVoices: true, SendDocs: true, SendPlain: true,
			SendReactions: true, UntilDate: untilDate,
		}).To_ChatBannedRights(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func unbanChatMember(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a supergroup or channel"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	if _, err := request.bool("only_if_banned"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: only_if_banned is invalid"
	}
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	_, err = ctx.ChannelsClient.ChannelsEditBanned(botRPCContext(r.Context(), botID), &mtproto.TLChannelsEditBanned{
		Channel:      mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		Participant:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID, AccessHash: userID}).To_InputPeer(),
		BannedRights: mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func restrictChatMember(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a supergroup or channel"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	if len(request.raw("permissions")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: permissions is required"
	}
	var permissions botapi.ChatPermissions
	if err := request.json("permissions", &permissions); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: permissions is invalid"
	}
	untilDate, err := botAPIUntilDate(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: until_date is invalid"
	}
	if _, err := request.bool("use_independent_chat_permissions"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: use_independent_chat_permissions is invalid"
	}
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	_, err = ctx.ChannelsClient.ChannelsEditBanned(botRPCContext(r.Context(), botID), &mtproto.TLChannelsEditBanned{
		Channel:      mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		Participant:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID, AccessHash: userID}).To_InputPeer(),
		BannedRights: botAPIChatBannedRights(permissions, untilDate),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func promoteChatMember(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	rights, err := botAPIChatAdminRights(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, err.Error()
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHAT {
		if ctx == nil || ctx.ChatsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
		}
		ok, err := ctx.ChatsClient.MessagesEditChatAdmin(callCtx, &mtproto.TLMessagesEditChatAdmin{
			ChatId:  peerID,
			UserId:  mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: userID}).To_InputUser(),
			IsAdmin: mtproto.ToBool(chatAdminRightsSet(rights)),
		})
		if err != nil || !mtproto.FromBool(ok) {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return true, http.StatusOK, 0, ""
	}
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	customTitle := request.string("custom_title")
	_, err = ctx.ChannelsClient.ChannelsEditAdmin(callCtx, &mtproto.TLChannelsEditAdmin{
		Channel:     mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		UserId:      mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: userID}).To_InputUser(),
		AdminRights: rights,
		Rank_STRING: customTitle,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setChatAdministratorCustomTitle(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a supergroup or channel"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	title := request.string("custom_title")
	if utf8.RuneCountInString(title) > 16 {
		return nil, http.StatusBadRequest, 400, "Bad Request: custom_title is too long"
	}
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	callCtx := botRPCContext(r.Context(), botID)
	participant, err := ctx.ChannelsClient.ChannelsGetParticipant(callCtx, &mtproto.TLChannelsGetParticipant{
		Channel:     mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID, AccessHash: userID}).To_InputPeer(),
	})
	if err != nil || participant == nil || participant.GetParticipant() == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	current := participant.GetParticipant()
	if current.GetPredicateName() != mtproto.Predicate_channelParticipantAdmin && current.GetPredicateName() != mtproto.Predicate_channelParticipantCreator {
		return nil, http.StatusBadRequest, 400, "Bad Request: user is not an administrator"
	}
	rights := current.GetAdminRights()
	if rights == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: administrator rights are unavailable"
	}
	_, err = ctx.ChannelsClient.ChannelsEditAdmin(callCtx, &mtproto.TLChannelsEditAdmin{
		Channel:     mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		UserId:      mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: userID}).To_InputUser(),
		AdminRights: rights,
		Rank_STRING: title,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setChatPermissions(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if len(request.raw("permissions")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: permissions is required"
	}
	var permissions botapi.ChatPermissions
	if err := request.json("permissions", &permissions); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: permissions is invalid"
	}
	if ctx == nil || ctx.ChatsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
	}
	_, err = ctx.ChatsClient.MessagesEditChatDefaultBannedRights(botRPCContext(r.Context(), botID), &mtproto.TLMessagesEditChatDefaultBannedRights{
		Peer:         botAPIInputPeer(botID, peerType, peerID),
		BannedRights: botAPIChatBannedRights(permissions, 0),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setChatTitle(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	title := request.string("title")
	if title == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: title is required"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		if ctx == nil || ctx.ChannelsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
		}
		_, err = ctx.ChannelsClient.ChannelsEditTitle(callCtx, &mtproto.TLChannelsEditTitle{
			Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
			Title:   title,
		})
	} else {
		if ctx == nil || ctx.ChatsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
		}
		_, err = ctx.ChatsClient.MessagesEditChatTitle(callCtx, &mtproto.TLMessagesEditChatTitle{ChatId: peerID, Title: title})
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setChatDescription(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if ctx == nil || ctx.ChatsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
	}
	_, err = ctx.ChatsClient.MessagesEditChatAbout(botRPCContext(r.Context(), botID), &mtproto.TLMessagesEditChatAbout{
		Peer:  botAPIInputPeer(botID, peerType, peerID),
		About: request.string("description"),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func pinChatMessage(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	disableNotification, err := request.bool("disable_notification")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: disable_notification is invalid"
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	_, err = ctx.MessagesClient.MessagesUpdatePinnedMessage(botRPCContext(r.Context(), botID), &mtproto.TLMessagesUpdatePinnedMessage{
		Silent: disableNotification,
		Peer:   botAPIInputPeer(botID, peerType, peerID),
		Id:     int32(messageID),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func unpinChatMessage(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	if len(request.raw("message_id")) == 0 {
		return unpinAllChatMessages(ctx, r, botID, request)
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	_, err = ctx.MessagesClient.MessagesUpdatePinnedMessage(botRPCContext(r.Context(), botID), &mtproto.TLMessagesUpdatePinnedMessage{
		Unpin: true,
		Peer:  botAPIInputPeer(botID, peerType, peerID),
		Id:    int32(messageID),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func unpinAllChatMessages(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	_, err = ctx.MessagesClient.MessagesUnpinAllMessages(botRPCContext(r.Context(), botID), &mtproto.TLMessagesUnpinAllMessages{
		Peer: botAPIInputPeer(botID, peerType, peerID),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func leaveChat(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		if ctx == nil || ctx.ChannelsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
		}
		_, err = ctx.ChannelsClient.ChannelsLeaveChannel(callCtx, &mtproto.TLChannelsLeaveChannel{
			Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
		})
	} else {
		if ctx == nil || ctx.ChatsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
		}
		_, err = ctx.ChatsClient.MessagesDeleteChatUser(callCtx, &mtproto.TLMessagesDeleteChatUser{
			ChatId: peerID,
			UserId: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{UserId: botID}).To_InputUser(),
		})
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func getBotAPIGroupFullChat(ctx *svc.ServiceContext, callCtx context.Context, chatID int64) (*mtproto.Messages_ChatFull, error) {
	if ctx == nil || ctx.ChatsClient == nil {
		return nil, errors.New("chat service is not configured")
	}
	return ctx.ChatsClient.MessagesGetFullChat(callCtx, &mtproto.TLMessagesGetFullChat{ChatId: chatID})
}

func botAPIUntilDate(request *botAPIRequest) (int32, error) {
	if len(request.raw("until_date")) == 0 {
		return 0, nil
	}
	value, err := request.int64("until_date")
	if err != nil || value < 0 || value > int64(1<<31-1) {
		return 0, errors.New("until_date is invalid")
	}
	return int32(value), nil
}

func botAPIChatBannedRights(permissions botapi.ChatPermissions, untilDate int32) *mtproto.ChatBannedRights {
	return mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMessages:    !permissions.CanSendMessages,
		SendMedia:       !permissions.CanSendAudios && !permissions.CanSendDocuments && !permissions.CanSendPhotos && !permissions.CanSendVideos && !permissions.CanSendVideoNotes && !permissions.CanSendVoiceNotes,
		SendStickers:    !permissions.CanSendOtherMessages,
		SendGifs:        !permissions.CanSendOtherMessages,
		SendGames:       !permissions.CanSendOtherMessages,
		SendInline:      !permissions.CanSendOtherMessages,
		EmbedLinks:      !permissions.CanAddWebPagePreviews,
		SendPolls:       !permissions.CanSendPolls,
		ChangeInfo:      !permissions.CanChangeInfo,
		InviteUsers:     !permissions.CanInviteUsers,
		PinMessages:     !permissions.CanPinMessages,
		ManageTopics:    !permissions.CanManageTopics,
		SendPhotos:      !permissions.CanSendPhotos,
		SendVideos:      !permissions.CanSendVideos,
		SendRoundvideos: !permissions.CanSendVideoNotes,
		SendAudios:      !permissions.CanSendAudios,
		SendVoices:      !permissions.CanSendVoiceNotes,
		SendDocs:        !permissions.CanSendDocuments,
		SendPlain:       !permissions.CanSendMessages,
		UntilDate:       untilDate,
	}).To_ChatBannedRights()
}

func botAPIChatAdminRights(request *botAPIRequest) (*mtproto.ChatAdminRights, error) {
	parse := func(name string) (bool, error) {
		value, err := request.bool(name)
		if err != nil {
			return false, fmt.Errorf("Bad Request: %s is invalid", name)
		}
		return value, nil
	}
	var rights mtproto.ChatAdminRights
	var err error
	if rights.ChangeInfo, err = parse("can_change_info"); err != nil {
		return nil, err
	}
	if rights.PostMessages, err = parse("can_post_messages"); err != nil {
		return nil, err
	}
	if rights.EditMessages, err = parse("can_edit_messages"); err != nil {
		return nil, err
	}
	if rights.DeleteMessages, err = parse("can_delete_messages"); err != nil {
		return nil, err
	}
	if rights.BanUsers, err = parse("can_restrict_members"); err != nil {
		return nil, err
	}
	if rights.InviteUsers, err = parse("can_invite_users"); err != nil {
		return nil, err
	}
	if rights.PinMessages, err = parse("can_pin_messages"); err != nil {
		return nil, err
	}
	if rights.AddAdmins, err = parse("can_promote_members"); err != nil {
		return nil, err
	}
	if rights.Anonymous, err = parse("is_anonymous"); err != nil {
		return nil, err
	}
	if rights.ManageCall, err = parse("can_manage_video_chats"); err != nil {
		return nil, err
	}
	if rights.ManageTopics, err = parse("can_manage_topics"); err != nil {
		return nil, err
	}
	if rights.PostStories, err = parse("can_post_stories"); err != nil {
		return nil, err
	}
	if rights.EditStories, err = parse("can_edit_stories"); err != nil {
		return nil, err
	}
	if rights.DeleteStories, err = parse("can_delete_stories"); err != nil {
		return nil, err
	}
	if rights.ManageDirectMessages, err = parse("can_manage_direct_messages"); err != nil {
		return nil, err
	}
	if rights.Other, err = parse("can_manage_chat"); err != nil {
		return nil, err
	}
	return mtproto.MakeTLChatAdminRights(&rights).To_ChatAdminRights(), nil
}

func chatAdminRightsSet(rights *mtproto.ChatAdminRights) bool {
	if rights == nil {
		return false
	}
	return rights.GetChangeInfo() || rights.GetPostMessages() || rights.GetEditMessages() || rights.GetDeleteMessages() ||
		rights.GetBanUsers() || rights.GetInviteUsers() || rights.GetPinMessages() || rights.GetAddAdmins() ||
		rights.GetAnonymous() || rights.GetManageCall() || rights.GetOther() || rights.GetManageTopics() ||
		rights.GetPostStories() || rights.GetEditStories() || rights.GetDeleteStories() || rights.GetManageDirectMessages()
}

func parseBotAPIChatID(raw string) (int64, int32, int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "@") {
		return 0, 0, 0, errors.New("username chat id requires resolution")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, 0, 0, err
	}
	if id > 0 {
		return id, mtproto.PEER_USER, id, nil
	}
	if id <= -1000000000000 {
		return id, mtproto.PEER_CHANNEL, -1000000000000 - id, nil
	}
	return id, mtproto.PEER_CHAT, -id, nil
}

func resolveBotAPIChatID(ctx context.Context, service *svc.ServiceContext, botID int64, raw string) (int64, int32, int64, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "@") {
		return parseBotAPIChatID(raw)
	}
	username := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if username == "" || service == nil || service.UserClient == nil || botID <= 0 {
		return 0, 0, 0, errors.New("username chat id is invalid")
	}
	peer, err := service.UserClient.UserResolveUsername(botRPCContext(ctx, botID), &userpb.TLUserResolveUsername{Username: username})
	if err != nil || peer == nil {
		if err != nil {
			return 0, 0, 0, err
		}
		return 0, 0, 0, errors.New("username chat id was not found")
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		if peer.GetUserId() <= 0 {
			return 0, 0, 0, errors.New("username chat id is invalid")
		}
		return peer.GetUserId(), mtproto.PEER_USER, peer.GetUserId(), nil
	case mtproto.Predicate_peerChat:
		if peer.GetChatId() <= 0 {
			return 0, 0, 0, errors.New("username chat id is invalid")
		}
		return -peer.GetChatId(), mtproto.PEER_CHAT, peer.GetChatId(), nil
	case mtproto.Predicate_peerChannel:
		if peer.GetChannelId() <= 0 {
			return 0, 0, 0, errors.New("username chat id is invalid")
		}
		return -1000000000000 - peer.GetChannelId(), mtproto.PEER_CHANNEL, peer.GetChannelId(), nil
	default:
		return 0, 0, 0, errors.New("username chat id is invalid")
	}
}

func botRPCContext(parent context.Context, userID int64) context.Context {
	ctx, err := rpcmetadata.RpcMetadataToOutgoing(parent, &rpcmetadata.RpcMetadata{
		UserId:      userID,
		IsBot:       true,
		Layer:       229,
		ReceiveTime: time.Now().Unix(),
	})
	if err != nil {
		return parent
	}
	outgoing, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return ctx
	}
	return metadata.NewIncomingContext(ctx, outgoing)
}

func botMediaRPCContext(parent context.Context, userID int64) context.Context {
	ctx, err := rpcmetadata.RpcMetadataToOutgoing(parent, &rpcmetadata.RpcMetadata{
		UserId:        userID,
		PermAuthKeyId: userID,
		IsBot:         true,
		Layer:         229,
		ReceiveTime:   time.Now().Unix(),
	})
	if err != nil {
		return parent
	}
	outgoing, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return ctx
	}
	return metadata.NewIncomingContext(ctx, outgoing)
}

func randomMessageID() int64 {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		value := int64(binary.LittleEndian.Uint64(raw[:]) & 0x7fffffffffffffff)
		if value != 0 {
			return value
		}
	}
	return time.Now().UnixNano()
}

func rpcDescription(err error) string {
	if err == nil {
		return "Bad Request"
	}
	return fmt.Sprintf("Bad Request: %s", err.Error())
}

func writeBotAPIOK(w http.ResponseWriter, result any) {
	writeBotAPI(w, http.StatusOK, botapi.Response{OK: true, Result: result})
}

func writeBotAPIError(w http.ResponseWriter, status, code int, description string) {
	writeBotAPI(w, status, botapi.Response{OK: false, ErrorCode: code, Description: description})
}

func writeBotAPI(w http.ResponseWriter, status int, response botapi.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
