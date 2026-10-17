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
	"math"
	"mime"
	"net"
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
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"

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

func (r *botAPIRequest) has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.values[name]
	return ok
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

// text returns user-authored message content without normalizing whitespace.
// Telegram preserves leading/trailing whitespace in text and captions, while
// the generic string helper remains trimmed for identifiers and options.
func (r *botAPIRequest) text(name string) string {
	value := r.raw(name)
	if len(value) == 0 {
		return ""
	}
	var decoded string
	if json.Unmarshal(value, &decoded) == nil {
		return decoded
	}
	return string(value)
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

var botAPIMethodNames = func() map[string]string {
	names := []string{
		"getMe", "getFile", "getUpdates", "setWebhook", "deleteWebhook", "getWebhookInfo",
		"sendMessage", "sendPhoto", "sendDocument", "sendVideo", "sendVideoNote", "sendSticker",
		"sendAudio", "sendVoice", "sendAnimation", "sendLocation", "sendVenue", "sendContact",
		"sendPoll", "sendDice", "sendChatAction", "sendGame", "sendInvoice", "createInvoiceLink",
		"getMyStarBalance", "getStarTransactions", "refundStarPayment", "editUserStarSubscription",
		"sendMediaGroup", "forwardMessage", "copyMessage", "forwardMessages", "copyMessages",
		"getStickerSet", "createNewStickerSet", "addStickerToSet", "setStickerPositionInSet", "deleteStickerFromSet", "replaceStickerInSet", "setStickerEmojiList", "setStickerKeywords", "setStickerMaskPosition", "setStickerSetTitle", "setStickerSetThumbnail", "setCustomEmojiStickerSetThumbnail", "deleteStickerSet",
		"getChat", "getUserProfilePhotos", "getUserProfileAudios", "getCustomEmojiStickers", "getForumTopicIconStickers", "getUserPersonalChatMessages",
		"setChatPhoto", "deleteChatPhoto", "getChatAdministrators", "getChatMemberCount", "getChatMember",
		"exportChatInviteLink", "createChatInviteLink", "editChatInviteLink", "revokeChatInviteLink",
		"approveChatJoinRequest", "declineChatJoinRequest", "createForumTopic", "editForumTopic",
		"closeForumTopic", "reopenForumTopic", "hideGeneralForumTopic", "unhideGeneralForumTopic",
		"closeGeneralForumTopic", "editGeneralForumTopic", "reopenGeneralForumTopic",
		"unpinAllGeneralForumTopicMessages", "deleteForumTopic", "unpinAllForumTopicMessages", "banChatMember", "kickChatMember",
		"banChatSenderChat", "unbanChatSenderChat", "unbanChatMember", "restrictChatMember", "promoteChatMember", "setChatAdministratorCustomTitle",
		"setChatPermissions", "setChatTitle", "setChatDescription", "pinChatMessage", "unpinChatMessage",
		"unpinAllChatMessages", "leaveChat", "getMyCommands", "setMyCommands", "deleteMyCommands",
		"setChatMenuButton", "getChatMenuButton", "setMyName", "getMyName", "setMyDescription",
		"getMyDescription", "setMyShortDescription", "getMyShortDescription", "getMyDefaultAdministratorRights",
		"setMyDefaultAdministratorRights", "setMyProfilePhoto", "removeMyProfilePhoto", "editMessageText", "editMessageCaption", "editMessageMedia",
		"editMessageReplyMarkup", "editMessageLiveLocation", "stopMessageLiveLocation", "deleteMessage",
		"deleteMessages", "deleteMessageReaction", "deleteAllMessageReactions", "setMessageReaction", "setGameScore", "getGameHighScores",
		"getBusinessConnection", "readBusinessMessage", "deleteBusinessMessages", "answerCallbackQuery",
		"answerInlineQuery", "answerWebAppQuery", "answerShippingQuery", "answerPreCheckoutQuery",
		// Methods introduced after the original Bot API surface. Keep these in
		// the canonical name table even when the matching Layer 229 adapter is
		// not available, so clients receive a method-specific 501 response.
		"logOut", "close", "sendLivePhoto", "sendPaidMedia", "sendChecklist", "sendMessageDraft",
		"sendRichMessage", "sendRichMessageDraft",
		"setUserEmojiStatus", "setChatMemberTag", "createChatSubscriptionInviteLink", "editChatSubscriptionInviteLink",
		"answerChatJoinRequestQuery", "sendChatJoinRequestWebApp", "setChatStickerSet", "deleteChatStickerSet",
		"answerGuestQuery", "getUserChatBoosts", "getManagedBotToken", "replaceManagedBotToken",
		"getManagedBotAccessSettings", "setManagedBotAccessSettings", "getAvailableGifts", "sendGift",
		"giftPremiumSubscription", "verifyUser", "verifyChat", "removeUserVerification", "removeChatVerification",
		"setBusinessAccountName", "setBusinessAccountUsername", "setBusinessAccountBio", "setBusinessAccountProfilePhoto",
		"removeBusinessAccountProfilePhoto", "setBusinessAccountGiftSettings", "getBusinessAccountStarBalance",
		"transferBusinessAccountStars", "getBusinessAccountGifts", "getUserGifts", "getChatGifts",
		"convertGiftToStars", "upgradeGift", "transferGift", "postStory", "repostStory", "editStory", "deleteStory",
		"savePreparedInlineMessage", "savePreparedKeyboardButton", "editMessageChecklist", "stopPoll",
		"editEphemeralMessageText", "editEphemeralMessageMedia", "editEphemeralMessageCaption",
		"editEphemeralMessageReplyMarkup", "approveSuggestedPost", "declineSuggestedPost", "deleteEphemeralMessage",
		"uploadStickerFile", "setPassportDataErrors",
	}
	result := make(map[string]string, len(names))
	for _, name := range names {
		result[strings.ToLower(name)] = name
	}
	return result
}()

func canonicalBotAPIMethod(method string) string {
	if canonical, ok := botAPIMethodNames[strings.ToLower(method)]; ok {
		return canonical
	}
	return method
}

func dispatchBotAPIMethod(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, method string, request *botAPIRequest) (any, int, int, string) {
	botID := bot.GetUser().GetId()
	method = canonicalBotAPIMethod(method)
	switch method {
	case "getMe":
		return botapi.UserFromImmutableUser(bot), http.StatusOK, 0, ""
	case "logOut", "close":
		return true, http.StatusOK, 0, ""
	case "getAvailableGifts":
		return getAvailableGifts(ctx, r, botID)
	case "convertGiftToStars":
		return convertGiftToStars(ctx, r, botID, request)
	case "upgradeGift":
		return upgradeGift(ctx, r, botID, request)
	case "transferGift":
		return transferGift(ctx, r, botID, request)
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
	case "getStickerSet":
		return getStickerSet(ctx, r, botID, request)
	case "createNewStickerSet":
		return createNewStickerSet(ctx, r, botID, request)
	case "addStickerToSet":
		return addStickerToSet(ctx, r, botID, request)
	case "setStickerPositionInSet":
		return setStickerPositionInSet(ctx, r, botID, request)
	case "deleteStickerFromSet":
		return deleteStickerFromSet(ctx, r, botID, request)
	case "replaceStickerInSet":
		return replaceStickerInSet(ctx, r, botID, request)
	case "setStickerEmojiList":
		return setStickerEmojiList(ctx, r, botID, request)
	case "setStickerKeywords":
		return setStickerKeywords(ctx, r, botID, request)
	case "setStickerMaskPosition":
		return setStickerMaskPosition(ctx, r, botID, request)
	case "setStickerSetTitle":
		return setStickerSetTitle(ctx, r, botID, request)
	case "setStickerSetThumbnail":
		return setStickerSetThumbnail(ctx, r, botID, request)
	case "setCustomEmojiStickerSetThumbnail":
		return setCustomEmojiStickerSetThumbnail(ctx, r, botID, request)
	case "deleteStickerSet":
		return deleteStickerSet(ctx, r, botID, request)
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
	case "stopPoll":
		return stopPoll(ctx, r, bot, request)
	case "sendDice":
		return sendDice(ctx, r, bot, request)
	case "sendChatAction":
		return sendChatAction(ctx, r, bot, request)
	case "sendGame":
		return sendGame(ctx, r, bot, request)
	case "sendInvoice":
		return sendInvoice(ctx, r, bot, request)
	case "createInvoiceLink":
		return createInvoiceLink(ctx, r.Context(), botID, request)
	case "getMyStarBalance":
		return getMyStarBalance(ctx, r.Context(), botID)
	case "getStarTransactions":
		return getStarTransactions(ctx, r.Context(), botID, request)
	case "refundStarPayment":
		return refundStarPayment(ctx, r.Context(), botID, request)
	case "editUserStarSubscription":
		return editUserStarSubscription(ctx, r.Context(), botID, request)
	case "sendMediaGroup":
		return sendMediaGroup(ctx, r, bot, request)
	case "forwardMessage":
		return forwardMessage(ctx, r, bot, request, false)
	case "copyMessage":
		return forwardMessage(ctx, r, bot, request, true)
	case "forwardMessages":
		return forwardMessages(ctx, r, bot, request, false)
	case "copyMessages":
		return forwardMessages(ctx, r, bot, request, true)
	case "getChat":
		return getChat(ctx, r, botID, request)
	case "getUserProfilePhotos":
		return getUserProfilePhotos(ctx, r, botID, request)
	case "getUserProfileAudios":
		return getUserProfileAudios(ctx, r, botID, request)
	case "getCustomEmojiStickers":
		return getCustomEmojiStickers(ctx, r, botID, request)
	case "getForumTopicIconStickers":
		return getForumTopicIconStickers(ctx, r, botID)
	case "getUserPersonalChatMessages":
		return getUserPersonalChatMessages(ctx, r, botID, request)
	case "setUserEmojiStatus":
		return setUserEmojiStatus(ctx, r, botID, request)
	case "setChatMemberTag":
		return setChatMemberTag(ctx, r, botID, request)
	case "setChatStickerSet":
		return setChatStickerSet(ctx, r, botID, request)
	case "deleteChatStickerSet":
		return deleteChatStickerSet(ctx, r, botID, request)
	case "setChatPhoto":
		return setChatPhoto(ctx, r, botID, request)
	case "deleteChatPhoto":
		return deleteChatPhoto(ctx, r, botID, request)
	case "getChatAdministrators":
		return getChatAdministrators(ctx, r, botID, request)
	case "getChatMemberCount":
		return getChatMemberCount(ctx, r, botID, request)
	case "getChatMember":
		return getChatMember(ctx, r, botID, request)
	case "exportChatInviteLink":
		return exportChatInviteLink(ctx, r, botID, request)
	case "createChatInviteLink":
		return createChatInviteLink(ctx, r, botID, request)
	case "createChatSubscriptionInviteLink":
		return createChatSubscriptionInviteLink(ctx, r, botID, request)
	case "editChatInviteLink":
		return editChatInviteLink(ctx, r, botID, request)
	case "editChatSubscriptionInviteLink":
		return editChatSubscriptionInviteLink(ctx, r, botID, request)
	case "revokeChatInviteLink":
		return revokeChatInviteLink(ctx, r, botID, request)
	case "approveChatJoinRequest":
		return handleChatJoinRequest(ctx, r, botID, request, true)
	case "declineChatJoinRequest":
		return handleChatJoinRequest(ctx, r, botID, request, false)
	case "createForumTopic":
		return createForumTopic(ctx, r, botID, request)
	case "editForumTopic":
		return editForumTopic(ctx, r, botID, request, false, false, false, false)
	case "closeForumTopic":
		return closeForumTopic(ctx, r, botID, request)
	case "reopenForumTopic":
		return reopenForumTopic(ctx, r, botID, request)
	case "hideGeneralForumTopic":
		return hideGeneralForumTopic(ctx, r, botID, request)
	case "unhideGeneralForumTopic":
		return unhideGeneralForumTopic(ctx, r, botID, request)
	case "closeGeneralForumTopic":
		return closeGeneralForumTopic(ctx, r, botID, request)
	case "editGeneralForumTopic":
		return editGeneralForumTopic(ctx, r, botID, request)
	case "reopenGeneralForumTopic":
		return reopenGeneralForumTopic(ctx, r, botID, request)
	case "unpinAllGeneralForumTopicMessages":
		return unpinAllGeneralForumTopicMessages(ctx, r, botID, request)
	case "deleteForumTopic":
		return deleteForumTopic(ctx, r, botID, request)
	case "unpinAllForumTopicMessages":
		return unpinAllForumTopicMessages(ctx, r, botID, request)
	case "banChatMember", "kickChatMember":
		return banChatMember(ctx, r, botID, request)
	case "banChatSenderChat":
		return banChatSenderChat(ctx, r, botID, request)
	case "unbanChatSenderChat":
		return unbanChatSenderChat(ctx, r, botID, request)
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
		if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
			return nil, status, code, description
		}
		info, err := ctx.UserClient.UserGetBotInfo(botRPCContext(r.Context(), botID), &userpb.TLUserGetBotInfo{BotId: botID})
		if err != nil || info == nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return botapi.BotCommandsFromMTProto(info.GetCommands()), http.StatusOK, 0, ""
	case "setMyCommands", "deleteMyCommands":
		if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
			return nil, status, code, description
		}
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
		if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
			return nil, status, code, description
		}
		return &botapi.BotName{Name: bot.GetUser().GetFirstName()}, http.StatusOK, 0, ""
	case "setMyDescription":
		return setMyDescription(ctx, r.Context(), botID, request)
	case "getMyDescription":
		if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
			return nil, status, code, description
		}
		return getMyDescription(ctx, r.Context(), botID)
	case "setMyShortDescription":
		return setMyShortDescription(ctx, r.Context(), botID, request)
	case "getMyShortDescription":
		if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
			return nil, status, code, description
		}
		return getMyShortDescription(ctx, r.Context(), botID)
	case "getMyDefaultAdministratorRights":
		return getMyDefaultAdministratorRights(ctx, r.Context(), botID, request)
	case "setMyDefaultAdministratorRights":
		return setMyDefaultAdministratorRights(ctx, r.Context(), botID, request)
	case "setMyProfilePhoto":
		return setMyProfilePhoto(ctx, r, botID, request)
	case "removeMyProfilePhoto":
		return removeMyProfilePhoto(ctx, r, bot)
	case "editMessageText":
		return editMessageText(ctx, r, bot, request)
	case "editMessageCaption":
		return editMessageCaption(ctx, r, bot, request)
	case "editMessageMedia":
		return editMessageMedia(ctx, r, bot, request)
	case "editMessageReplyMarkup":
		return editMessageReplyMarkup(ctx, r, bot, request)
	case "editMessageLiveLocation":
		return editMessageLiveLocation(ctx, r, bot, request)
	case "stopMessageLiveLocation":
		return stopMessageLiveLocation(ctx, r, bot, request)
	case "deleteMessage":
		return deleteMessage(ctx, r, botID, request)
	case "deleteMessages":
		return deleteMessages(ctx, r, botID, request)
	case "deleteMessageReaction":
		return deleteMessageReaction(ctx, r, botID, request)
	case "deleteAllMessageReactions":
		return deleteAllMessageReactions(ctx, r, botID, request)
	case "setMessageReaction":
		return setMessageReaction(ctx, r, botID, request)
	case "setGameScore":
		return setGameScore(ctx, r, bot, request)
	case "getGameHighScores":
		return getGameHighScores(ctx, r, bot, request)
	case "getBusinessConnection":
		return getBusinessConnection(ctx, r.Context(), botID, request)
	case "readBusinessMessage":
		return readBusinessMessage(ctx, r, botID, request)
	case "deleteBusinessMessages":
		return deleteBusinessMessages(ctx, r, botID, request)
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
	case "answerChatJoinRequestQuery":
		return answerChatJoinRequestQuery(ctx, r, botID, request)
	case "sendChatJoinRequestWebApp":
		return sendChatJoinRequestWebApp(ctx, r, botID, request)
	case "answerGuestQuery":
		return answerGuestQuery(ctx, r, botID, request)
	case "setBusinessAccountName":
		return setBusinessAccountName(ctx, r, botID, request)
	case "setBusinessAccountUsername":
		return setBusinessAccountUsername(ctx, r, botID, request)
	case "setBusinessAccountBio":
		return setBusinessAccountBio(ctx, r, botID, request)
	case "setBusinessAccountProfilePhoto":
		return setBusinessAccountProfilePhoto(ctx, r, botID, request)
	case "removeBusinessAccountProfilePhoto":
		return removeBusinessAccountProfilePhoto(ctx, r, botID, request)
	case "getBusinessAccountStarBalance":
		return getBusinessAccountStarBalance(ctx, r, botID, request)
	case "getBusinessAccountGifts":
		return getBusinessAccountGifts(ctx, r, botID, request)
	case "getUserGifts":
		return getUserGifts(ctx, r, botID, request)
	case "getChatGifts":
		return getChatGifts(ctx, r, botID, request)
	case "savePreparedInlineMessage":
		return savePreparedInlineMessage(ctx, r, botID, request)
	case "deleteEphemeralMessage":
		return deleteEphemeralMessage(ctx, r, botID, request)
	case "editEphemeralMessageText":
		return editEphemeralMessageText(ctx, r, bot, request)
	case "editEphemeralMessageMedia":
		return editEphemeralMessageMedia(ctx, r, bot, request)
	case "editEphemeralMessageCaption":
		return editEphemeralMessageCaption(ctx, r, bot, request)
	case "editEphemeralMessageReplyMarkup":
		return editEphemeralMessageReplyMarkup(ctx, r, bot, request)
	case "sendLivePhoto":
		return sendLivePhoto(ctx, r, bot, request)
	case "sendPaidMedia":
		return sendPaidMedia(ctx, r, bot, request)
	case "sendChecklist":
		return sendChecklist(ctx, r, bot, request)
	case "sendMessageDraft":
		return sendMessageDraft(ctx, r, bot, request)
	case "sendRichMessage":
		return sendRichMessage(ctx, r, bot, request)
	case "sendRichMessageDraft":
		return sendRichMessageDraft(ctx, r, bot, request)
	case "editMessageChecklist":
		return editMessageChecklist(ctx, r, bot, request)
	case "uploadStickerFile":
		return uploadStickerFile(ctx, r, botID, request)
	case "getUserChatBoosts":
		return getUserChatBoosts(ctx, r, botID, request)
	case "getManagedBotToken":
		return getManagedBotToken(ctx, r, botID, request)
	case "replaceManagedBotToken":
		return replaceManagedBotToken(ctx, r, botID, request)
	case "getManagedBotAccessSettings":
		return getManagedBotAccessSettings(ctx, r, botID, request)
	case "setManagedBotAccessSettings":
		return setManagedBotAccessSettings(ctx, r, botID, request)
	case "verifyUser":
		return verifyUser(ctx, r, botID, request)
	case "verifyChat":
		return verifyChat(ctx, r, botID, request)
	case "removeUserVerification":
		return removeUserVerification(ctx, r, botID, request)
	case "removeChatVerification":
		return removeChatVerification(ctx, r, botID, request)
	case "setPassportDataErrors":
		return setPassportDataErrors(ctx, r, botID, request)
	case "postStory":
		return postStory(ctx, r, botID, request)
	case "repostStory":
		return repostStory(ctx, r, botID, request)
	case "editStory":
		return editStory(ctx, r, botID, request)
	case "deleteStory":
		return deleteStory(ctx, r, botID, request)
	case "approveSuggestedPost":
		return suggestedPost(ctx, r, botID, request, false)
	case "declineSuggestedPost":
		return suggestedPost(ctx, r, botID, request, true)
	case "sendGift":
		return sendGift(ctx, r, botID, request)
	case "giftPremiumSubscription":
		return giftPremiumSubscription(ctx, r, botID, request)
	case "transferBusinessAccountStars":
		return transferBusinessAccountStars(ctx, r, botID, request)
	case "savePreparedKeyboardButton":
		return savePreparedKeyboardButton(ctx, r, botID, request)
	case "setBusinessAccountGiftSettings":
		return setBusinessAccountGiftSettings(ctx, r, botID, request)
	default:
		return nil, http.StatusNotImplemented, 501, "Bad Request: method is not supported by this server"
	}
}

func botAPIUnsupportedMethod(method string) (any, int, int, string) {
	return nil, http.StatusNotImplemented, 501, "Not Implemented: " + method + " requires a Layer 229/service adapter"
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
	if err != nil || (request.has("limit") && limit < 1) || limit > 100 {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid limit"
	}
	timeout, err := request.int("timeout")
	if err != nil || timeout < 0 || timeout > 50 {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid timeout"
	}
	allowed, allowedPresent, err := parseBotAPIAllowedUpdates(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid allowed_updates"
	}
	if !allowedPresent {
		// The Bot API keeps the previous filter when allowed_updates is omitted.
		// The store exposes the last webhook filter when it is available; a
		// missing row means the default (all update types) remains in effect.
		allowed = append([]string(nil), webhook.AllowedUpdates...)
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
	if parameter := botAPIUnsupportedWebhookParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the webhook store"
	}
	dropPending, err := request.bool("drop_pending_updates")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid drop_pending_updates"
	}
	webhookURL := request.string("url")
	if webhookURL == "" {
		if ctx == nil || ctx.BotAPIStore == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: webhook store is not configured"
		}
		if err := ctx.BotAPIStore.DeleteWebhook(requestCtx, botID, dropPending); err != nil {
			return nil, http.StatusInternalServerError, 500, err.Error()
		}
		return true, http.StatusOK, 0, ""
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
	previousWebhook, _, err := ctx.BotAPIStore.GetWebhook(requestCtx, botID)
	if err != nil {
		return nil, http.StatusInternalServerError, 500, err.Error()
	}
	allowed, allowedPresent, err := parseBotAPIAllowedUpdates(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: invalid allowed_updates"
	}
	if !allowedPresent {
		// Telegram preserves the previous allowed_updates filter when this
		// optional field is omitted while updating a webhook.
		allowed = append([]string(nil), previousWebhook.AllowedUpdates...)
	}
	if ctx == nil || ctx.BotAPIStore == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: webhook store is not configured"
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

func botAPIUnsupportedWebhookParameter(request *botAPIRequest) string {
	if request == nil {
		return ""
	}
	for _, name := range []string{"certificate", "ip_address"} {
		if !request.has(name) {
			continue
		}
		raw := strings.TrimSpace(string(request.raw(name)))
		if raw != "" && raw != "null" && raw != `""` {
			return name
		}
	}
	return ""
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

func validateBotAPIAllowedUpdates(allowed []string) error {
	for _, name := range allowed {
		if !botapi.IsValidAllowedUpdateType(name) {
			return errors.New("unknown allowed update type")
		}
	}
	return nil
}

func parseBotAPIAllowedUpdates(request *botAPIRequest) ([]string, bool, error) {
	if request == nil || !request.has("allowed_updates") {
		return nil, false, nil
	}
	raw := request.raw("allowed_updates")
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	var allowed []string
	if err := request.json("allowed_updates", &allowed); err != nil {
		return nil, true, err
	}
	if err := validateBotAPIAllowedUpdates(allowed); err != nil {
		return nil, true, err
	}
	return allowed, true, nil
}

func sendMessage(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if parameter := botAPIUnsupportedMessageParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 adapter"
	}
	effect, effectErr := botAPIMessageEffect(request)
	if effectErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + effectErr.Error()
	}
	suggestedPost, suggestedPostErr := botAPISuggestedPost(request)
	if suggestedPostErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + suggestedPostErr.Error()
	}
	botID := bot.GetUser().GetId()
	scope, business, scopeErr := resolveBotAPISendScope(ctx, r.Context(), botID, request)
	if scopeErr != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(scopeErr)
	}
	senderID := botID
	callCtx := botRPCContext(r.Context(), botID)
	if business {
		senderID = scope.OwnerID
		callCtx = businessBotCallContext(r.Context(), botID, scope)
	}
	chatID, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, senderID, request.string("chat_id"))
	if err != nil || chatID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	text, entities, formatErr := botAPITextEntities(request, "text", "entities", "parse_mode")
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if text == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: text is empty"
	}
	if err := validateBotAPITextLength(text, 4096, "text"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	randomID := randomMessageID()
	if value, parseErr := request.int64("random_id"); parseErr == nil && value != 0 {
		randomID = value
	}
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		FromId:  mtproto.MakePeerUser(senderID),
		PeerId:  mtproto.MakePeerUtil(peerType, peerID).ToPeer(),
		Date:    int32(time.Now().Unix()),
		Message: text,
	}).To_Message()
	message.Entities = botapi.MessageEntitiesToMTProto(entities)
	message.Effect = effect
	message.Silent = requestBool(request, "disable_notification")
	message.Noforwards = requestBool(request, "protect_content")
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	message.ReplyMarkup = markup
	noWebpage, invertMedia, err := botAPILinkPreviewOptions(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
	}
	message.InvertMedia = invertMedia
	replyTo, replyErr := botAPIReplyTo(request)
	if replyErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	topMsgID, topErr := botAPIOptionalInt32(request, "message_thread_id", 1, int64(^uint32(0)>>1))
	if topErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	if replyTo != nil {
		message.ReplyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
			ReplyToMsgId:           replyTo.GetReplyToMsgId(),
			ReplyToMsgId_INT32:     replyTo.GetReplyToMsgId(),
			ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(replyTo.GetReplyToMsgId()),
			ReplyToTopId:           topMsgID,
		}).To_MessageReplyHeader()
	}
	if business {
		if ctx == nil || ctx.MessagesClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
		}
		replyTo, replyErr := botAPIReplyTo(request)
		if replyErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
		}
		updates, sendErr := ctx.MessagesClient.MessagesSendMessage(callCtx, &mtproto.TLMessagesSendMessage{
			NoWebpage:          noWebpage,
			Silent:             requestBool(request, "disable_notification"),
			Noforwards:         requestBool(request, "protect_content"),
			InvertMedia:        invertMedia,
			AllowPaidFloodskip: requestBool(request, "allow_paid_broadcast"),
			Effect:             effect,
			Peer:               botAPIInputPeer(senderID, peerType, peerID),
			ReplyTo:            replyTo,
			TopMsgId:           topMsgID,
			Message:            text,
			RandomId:           randomID,
			ReplyMarkup:        markup,
			Entities:           message.Entities,
			SuggestedPost:      suggestedPost,
		})
		if sendErr != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(sendErr)
		}
		result := botapi.FirstMessageFromUpdates(updates)
		if result == nil {
			return nil, http.StatusInternalServerError, 500, "Internal Server Error: message was not returned"
		}
		return result, http.StatusOK, 0, ""
	}
	if suggestedPost != nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: suggested_post_parameters requires the Layer 229 message service"
	}
	updates, err := ctx.MsgClient.MsgSendMessageV2(callCtx, &msgpb.TLMsgSendMessageV2{
		UserId:   botID,
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
	if parameter := botAPIUnsupportedMessageParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 adapter"
	}
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
	effect, effectErr := botAPIMessageEffect(request)
	if effectErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + effectErr.Error()
	}
	forward.Effect = effect
	forward.AllowPaidFloodskip = requestBool(request, "allow_paid_broadcast")
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
	if parameter := botAPIUnsupportedMessageParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 adapter"
	}
	effect, effectErr := botAPIMessageEffect(request)
	if effectErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + effectErr.Error()
	}
	suggestedPost, suggestedPostErr := botAPISuggestedPost(request)
	if suggestedPostErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + suggestedPostErr.Error()
	}
	botID := bot.GetUser().GetId()
	scope, business, scopeErr := resolveBotAPISendScope(ctx, r.Context(), botID, request)
	if scopeErr != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(scopeErr)
	}
	senderID := botID
	callCtx := botMediaRPCContext(r.Context(), botID)
	if business {
		senderID = scope.OwnerID
		callCtx = businessBotMediaCallContext(r.Context(), botID, scope)
	}
	chatID, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, senderID, request.string("chat_id"))
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
		inputFile, uploadErr := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
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
		if botAPIHTTPMediaURL(fileID) {
			data, mimeType, name, fetchErr := fetchBotAPIRemoteMedia(r.Context(), fileID)
			if fetchErr != nil {
				return nil, http.StatusBadRequest, 400, "Bad Request: media URL is invalid"
			}
			fileRef, uploadErr := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
			if uploadErr != nil {
				return nil, http.StatusBadRequest, 400, rpcDescription(uploadErr)
			}
			media = botAPIUploadedMedia(kind, fileRef, name, mimeType, duration, width, height, request)
		} else {
			media, err = botAPIExistingMedia(kind, fileID, request)
		}
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: file_id is invalid"
		}
	}

	caption, entities, formatErr := botAPICaptionEntities(request, "caption", "caption_entities")
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if err := validateBotAPITextLength(caption, 1024, "caption"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	topMsgID, err := botAPIOptionalInt32(request, "message_thread_id", 1, int64(^uint32(0)>>1))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	invertMedia, err := botAPIMessageInvertMedia(request)
	if err != nil {
		if errors.Is(err, errBotAPIShowCaptionAboveMediaInvalid) {
			return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
	}
	updates, err := ctx.MessagesClient.MessagesSendMedia(callCtx, &mtproto.TLMessagesSendMedia{
		Peer:               botAPIInputPeer(senderID, peerType, peerID),
		Media:              media,
		Message:            caption,
		RandomId:           randomMessageID(),
		ReplyMarkup:        markup,
		Entities:           botapi.MessageEntitiesToMTProto(entities),
		ReplyTo:            replyTo,
		TopMsgId:           topMsgID,
		InvertMedia:        invertMedia,
		Silent:             requestBool(request, "disable_notification"),
		Noforwards:         requestBool(request, "protect_content"),
		AllowPaidFloodskip: requestBool(request, "allow_paid_broadcast"),
		Effect:             effect,
		SuggestedPost:      suggestedPost,
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
	longitude, latitude, err := botAPILocationCoordinates(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, err.Error()
	}
	var horizontalAccuracy float64
	horizontalAccuracy, err = request.float64("horizontal_accuracy")
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
	provider, venueID, venueType := botAPIVenueProvider(request)
	media := mtproto.MakeTLInputMediaVenue(&mtproto.InputMedia{
		GeoPoint:        mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: latitude, Long: longitude}).To_InputGeoPoint(),
		Title:           title,
		Address:         address,
		Provider_STRING: provider,
		VenueId:         venueID,
		VenueType:       venueType,
	}).To_InputMedia()
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func botAPIVenueProvider(request *botAPIRequest) (provider, venueID, venueType string) {
	if request == nil {
		return "", "", ""
	}
	if googleID := request.string("google_place_id"); googleID != "" {
		return "google", googleID, request.string("google_place_type")
	}
	if foursquareID := request.string("foursquare_id"); foursquareID != "" {
		return "foursquare", foursquareID, request.string("foursquare_type")
	}
	return "", "", ""
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
	if parameter := botAPIPollUnsupportedParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 poll adapter"
	}
	question, questionEntities, err := botAPITextEntities(request, "question", "question_entities", "question_parse_mode")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if strings.TrimSpace(question) == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: question is required"
	}
	if utf8.RuneCountInString(question) > 300 {
		return nil, http.StatusBadRequest, 400, "Bad Request: question is too long"
	}
	answers, err := botAPIPollOptions(request)
	if err != nil {
		if errors.Is(err, errBotAPIPollOptionMediaUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	isAnonymous := true
	if request.has("is_anonymous") {
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
	allowsRevoting := pollType == "regular"
	if request.has("allows_revoting") {
		allowsRevoting, err = request.bool("allows_revoting")
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: allows_revoting is invalid"
		}
	}
	shuffleOptions, err := request.bool("shuffle_options")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: shuffle_options is invalid"
	}
	allowAddingOptions, err := request.bool("allow_adding_options")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: allow_adding_options is invalid"
	}
	if allowAddingOptions && (isAnonymous || pollType == "quiz") {
		return nil, http.StatusBadRequest, 400, "Bad Request: allow_adding_options is not supported for anonymous or quiz polls"
	}
	hideResultsUntilCloses, err := request.bool("hide_results_until_closes")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: hide_results_until_closes is invalid"
	}
	membersOnly, err := request.bool("members_only")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: members_only is invalid"
	}
	countryCodes, err := botAPIPollCountryCodes(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	correctOptionIDs := make([]int32, 0)
	hasPluralCorrectOptions := request.has("correct_option_ids") && strings.TrimSpace(string(request.raw("correct_option_ids"))) != "null"
	hasSingularCorrectOption := request.has("correct_option_id") && strings.TrimSpace(string(request.raw("correct_option_id"))) != "null"
	if hasPluralCorrectOptions && hasSingularCorrectOption {
		return nil, http.StatusBadRequest, 400, "Bad Request: only one of correct_option_ids and correct_option_id may be specified"
	}
	if hasPluralCorrectOptions {
		var ids []int
		if err := request.json("correct_option_ids", &ids); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_ids is invalid"
		}
		previous := -1
		for _, id := range ids {
			if id < 0 || id >= len(answers) || id <= previous {
				return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_ids is invalid"
			}
			previous = id
			correctOptionIDs = append(correctOptionIDs, int32(id))
		}
	} else if hasSingularCorrectOption {
		id, parseErr := request.int("correct_option_id")
		if parseErr != nil || id < 0 || id >= len(answers) {
			return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_id is invalid"
		}
		correctOptionIDs = append(correctOptionIDs, int32(id))
	}
	if pollType == "quiz" && len(correctOptionIDs) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_ids is required for quiz polls"
	}
	if pollType == "regular" && len(correctOptionIDs) > 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: correct_option_ids is only valid for quiz polls"
	}
	openPeriod := 0
	if request.has("open_period") && strings.TrimSpace(string(request.raw("open_period"))) != "null" {
		openPeriod, err = request.int("open_period")
		if err != nil || openPeriod < 5 || openPeriod > 2628000 {
			return nil, http.StatusBadRequest, 400, "Bad Request: open_period is invalid"
		}
	}
	closeDate := int64(0)
	if request.has("close_date") && strings.TrimSpace(string(request.raw("close_date"))) != "null" {
		closeDate, err = request.int64("close_date")
		if err != nil || closeDate < time.Now().Unix()+5 || closeDate > time.Now().Unix()+2628000 {
			return nil, http.StatusBadRequest, 400, "Bad Request: close_date is invalid"
		}
	}
	if openPeriod > 0 && closeDate > 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: open_period and close_date are mutually exclusive"
	}
	isClosed, err := request.bool("is_closed")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_closed is invalid"
	}
	if request.has("explanation_entities") && !request.has("explanation") && strings.TrimSpace(string(request.raw("explanation_entities"))) != "null" && strings.TrimSpace(string(request.raw("explanation_entities"))) != "[]" {
		return nil, http.StatusBadRequest, 400, "Bad Request: explanation is required when explanation_entities is provided"
	}
	explanation, explanationEntities, formatErr := botAPITextEntities(request, "explanation", "explanation_entities", "explanation_parse_mode")
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if request.has("explanation") && utf8.RuneCountInString(explanation) > 200 {
		return nil, http.StatusBadRequest, 400, "Bad Request: explanation is too long"
	}
	if strings.Count(explanation, "\n") > 2 {
		return nil, http.StatusBadRequest, 400, "Bad Request: explanation has too many line feeds"
	}
	poll := &mtproto.Poll{
		Id:                    randomMessageID(),
		Closed:                isClosed,
		PublicVoters:          !isAnonymous,
		MultipleChoice:        allowsMultiple,
		Quiz:                  pollType == "quiz",
		OpenAnswers:           allowAddingOptions,
		RevotingDisabled:      !allowsRevoting,
		ShuffleAnswers:        shuffleOptions,
		HideResultsUntilClose: hideResultsUntilCloses,
		SubscribersOnly:       membersOnly,
		CountriesIso2:         countryCodes,
		Question:              question,
		Question_STRING:       question,
		Answers:               answers,
	}
	poll.Question_TEXTWITHENTITIES = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text:     question,
		Entities: botapi.MessageEntitiesToMTProto(questionEntities),
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
	if len(correctOptionIDs) > 0 {
		media.CorrectAnswers_FLAGVECTORINT32 = correctOptionIDs
	}
	if request.has("explanation") && strings.TrimSpace(string(request.raw("explanation"))) != "null" {
		media.Solution = wrapperspb.String(explanation)
		media.SolutionEntities = botapi.MessageEntitiesToMTProto(explanationEntities)
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
	if parameter := botAPIUnsupportedMessageParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 adapter"
	}
	effect, effectErr := botAPIMessageEffect(request)
	if effectErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + effectErr.Error()
	}
	suggestedPost, suggestedPostErr := botAPISuggestedPost(request)
	if suggestedPostErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + suggestedPostErr.Error()
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: media service is not configured"
	}
	botID := bot.GetUser().GetId()
	scope, business, scopeErr := resolveBotAPISendScope(ctx, r.Context(), botID, request)
	if scopeErr != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(scopeErr)
	}
	senderID := botID
	callCtx := botMediaRPCContext(r.Context(), botID)
	if business {
		senderID = scope.OwnerID
		callCtx = businessBotMediaCallContext(r.Context(), botID, scope)
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, senderID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	caption, entities, formatErr := botAPICaptionEntities(request, "caption", "caption_entities")
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if err := validateBotAPITextLength(caption, 1024, "caption"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	var markup botapi.ReplyMarkupInput
	if err := request.json("reply_markup", &markup); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	invertMedia, err := botAPIMessageInvertMedia(request)
	if err != nil {
		if errors.Is(err, errBotAPIShowCaptionAboveMediaInvalid) {
			return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
	}
	topMsgID, err := botAPIOptionalInt32(request, "message_thread_id", 1, int64(^uint32(0)>>1))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	updates, err := ctx.MessagesClient.MessagesSendMedia(callCtx, &mtproto.TLMessagesSendMedia{
		Peer:               botAPIInputPeer(senderID, peerType, peerID),
		Media:              media,
		Message:            caption,
		RandomId:           randomMessageID(),
		ReplyTo:            replyTo,
		TopMsgId:           topMsgID,
		InvertMedia:        invertMedia,
		ReplyMarkup:        botapi.ReplyMarkupToMTProto(markup),
		Entities:           botapi.MessageEntitiesToMTProto(entities),
		Silent:             requestBool(request, "disable_notification"),
		Noforwards:         requestBool(request, "protect_content"),
		AllowPaidFloodskip: requestBool(request, "allow_paid_broadcast"),
		Effect:             effect,
		SuggestedPost:      suggestedPost,
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
	Type                  string                 `json:"type"`
	Media                 string                 `json:"media"`
	Photo                 string                 `json:"photo,omitempty"`
	Caption               string                 `json:"caption,omitempty"`
	CaptionEntities       []botapi.MessageEntity `json:"caption_entities,omitempty"`
	ParseMode             string                 `json:"parse_mode,omitempty"`
	CaptionParseMode      string                 `json:"caption_parse_mode,omitempty"`
	ShowCaptionAboveMedia bool                   `json:"show_caption_above_media,omitempty"`
	HasSpoiler            bool                   `json:"has_spoiler,omitempty"`
	SupportsStreaming     bool                   `json:"supports_streaming,omitempty"`
	Duration              int                    `json:"duration,omitempty"`
	Width                 int                    `json:"width,omitempty"`
	Height                int                    `json:"height,omitempty"`
}

func sendMediaGroup(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if parameter := botAPIUnsupportedMessageParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 adapter"
	}
	effect, effectErr := botAPIMessageEffect(request)
	if effectErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + effectErr.Error()
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: media service is not configured"
	}
	botID := bot.GetUser().GetId()
	scope, business, scopeErr := resolveBotAPISendScope(ctx, r.Context(), botID, request)
	if scopeErr != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(scopeErr)
	}
	senderID := botID
	callCtx := botMediaRPCContext(r.Context(), botID)
	if business {
		senderID = scope.OwnerID
		callCtx = businessBotMediaCallContext(r.Context(), botID, scope)
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, senderID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	var items []botAPIMediaGroupItem
	if err := request.json("media", &items); err != nil || len(items) < 2 || len(items) > 10 {
		return nil, http.StatusBadRequest, 400, "Bad Request: media must contain between 2 and 10 items"
	}
	multiMedia := make([]*mtproto.InputSingleMedia, 0, len(items))
	invertMedia := false
	for _, item := range items {
		media, err := botAPIMediaGroupInput(r, ctx, botID, item)
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: media item is invalid"
		}
		caption, entities, formatErr := botAPIMediaGroupCaption(item)
		if formatErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
		}
		if err := validateBotAPITextLength(caption, 1024, "caption"); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
		}
		invertMedia = invertMedia || item.ShowCaptionAboveMedia
		multiMedia = append(multiMedia, mtproto.MakeTLInputSingleMedia(&mtproto.InputSingleMedia{
			Media:    media,
			RandomId: randomMessageID(),
			Message:  caption,
			Entities: botapi.MessageEntitiesToMTProto(entities),
		}).To_InputSingleMedia())
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	topMsgID, err := botAPIOptionalInt32(request, "message_thread_id", 1, int64(^uint32(0)>>1))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	updates, err := ctx.MessagesClient.MessagesSendMultiMedia(callCtx, &mtproto.TLMessagesSendMultiMedia{
		Peer:               botAPIInputPeer(senderID, peerType, peerID),
		ReplyTo:            replyTo,
		TopMsgId:           topMsgID,
		MultiMedia:         multiMedia,
		Silent:             requestBool(request, "disable_notification"),
		Noforwards:         requestBool(request, "protect_content"),
		AllowPaidFloodskip: requestBool(request, "allow_paid_broadcast"),
		InvertMedia:        invertMedia,
		Effect:             effect,
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
	if request == nil || !request.has("longitude") || len(request.raw("longitude")) == 0 || string(request.raw("longitude")) == "null" {
		return 0, 0, errors.New("Bad Request: longitude is invalid")
	}
	if !request.has("latitude") || len(request.raw("latitude")) == 0 || string(request.raw("latitude")) == "null" {
		return 0, 0, errors.New("Bad Request: latitude is invalid")
	}
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
	if item.Type != "photo" && item.Type != "video" && item.Type != "document" && item.Type != "audio" && item.Type != "live_photo" {
		return nil, errors.New("unsupported media type")
	}
	item.Media = strings.TrimSpace(item.Media)
	if item.Media == "" {
		return nil, errors.New("media is required")
	}
	if item.Type == "live_photo" {
		item.Photo = strings.TrimSpace(item.Photo)
		if item.Photo == "" {
			return nil, errors.New("photo is required")
		}
		options := &botAPIRequest{values: map[string]json.RawMessage{
			"has_spoiler": json.RawMessage(strconv.FormatBool(item.HasSpoiler)),
		}}
		photo, err := botAPILayer229MediaInput(r, ctx, botID, item.Photo, "photo", options, false, "")
		if err != nil {
			return nil, err
		}
		video, err := botAPILayer229VideoDocument(r, ctx, botID, item.Media, options, "", item.Duration, item.Width, item.Height, item.SupportsStreaming)
		if err != nil {
			return nil, err
		}
		photo.LivePhoto = true
		photo.Video = video
		return photo, nil
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
	if botAPIHTTPMediaURL(item.Media) {
		data, mimeType, name, err := fetchBotAPIRemoteMedia(r.Context(), item.Media)
		if err != nil {
			return nil, err
		}
		fileRef, err := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
		if err != nil {
			return nil, err
		}
		return botAPIUploadedMedia(item.Type, fileRef, name, mimeType, item.Duration, item.Width, item.Height, &botAPIRequest{values: map[string]json.RawMessage{
			"has_spoiler":        json.RawMessage(strconv.FormatBool(item.HasSpoiler)),
			"supports_streaming": json.RawMessage(strconv.FormatBool(item.SupportsStreaming)),
		}}), nil
	}
	return botAPIExistingMedia(item.Type, item.Media, &botAPIRequest{values: map[string]json.RawMessage{"has_spoiler": json.RawMessage(strconv.FormatBool(item.HasSpoiler))}})
}

func requestBool(request *botAPIRequest, name string) bool {
	value, err := request.bool(name)
	return err == nil && value
}

var errBotAPIShowCaptionAboveMediaInvalid = errors.New("show_caption_above_media is invalid")

func botAPIUnsupportedMessageParameter(request *botAPIRequest) string {
	for _, name := range []string{
		"direct_messages_topic_id",
		"ephemeral_message_parameters",
	} {
		if request == nil || !request.has(name) {
			continue
		}
		raw := strings.TrimSpace(string(request.raw(name)))
		if raw != "" && raw != "null" && raw != `""` {
			return name
		}
	}
	return ""
}

type botAPISuggestedPostPrice struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

type botAPISuggestedPostParameters struct {
	Price    *botAPISuggestedPostPrice `json:"price,omitempty"`
	SendDate *int64                    `json:"send_date,omitempty"`
}

func botAPISuggestedPost(request *botAPIRequest) (*mtproto.SuggestedPost, error) {
	if request == nil || !request.has("suggested_post_parameters") {
		return nil, nil
	}
	raw := strings.TrimSpace(string(request.raw("suggested_post_parameters")))
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var input botAPISuggestedPostParameters
	if err := request.json("suggested_post_parameters", &input); err != nil {
		return nil, errors.New("suggested_post_parameters is invalid")
	}
	var price *mtproto.StarsAmount
	if input.Price != nil {
		currency := strings.ToUpper(strings.TrimSpace(input.Price.Currency))
		switch currency {
		case "XTR":
			if input.Price.Amount < 5 || input.Price.Amount > 100000 {
				return nil, errors.New("suggested_post_parameters.price.amount is invalid")
			}
			price = mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: input.Price.Amount}).To_StarsAmount()
		case "TON":
			if input.Price.Amount < 10000000 || input.Price.Amount > 10000000000000 {
				return nil, errors.New("suggested_post_parameters.price.amount is invalid")
			}
			price = mtproto.MakeTLStarsTonAmount(&mtproto.StarsAmount{Amount: input.Price.Amount}).To_StarsAmount()
		default:
			return nil, errors.New("suggested_post_parameters.price.currency is invalid")
		}
	}
	var scheduleDate *wrapperspb.Int32Value
	if input.SendDate != nil {
		now := time.Now().Unix()
		if *input.SendDate < now+300 || *input.SendDate > now+30*24*60*60 || *input.SendDate > int64(math.MaxInt32) {
			return nil, errors.New("suggested_post_parameters.send_date is invalid")
		}
		scheduleDate = wrapperspb.Int32(int32(*input.SendDate))
	}
	return mtproto.MakeTLSuggestedPost(&mtproto.SuggestedPost{Price: price, ScheduleDate: scheduleDate}).To_SuggestedPost(), nil
}

func botAPIMessageEffect(request *botAPIRequest) (*wrapperspb.Int64Value, error) {
	if request == nil || !request.has("message_effect_id") || len(request.raw("message_effect_id")) == 0 || string(request.raw("message_effect_id")) == "null" {
		return nil, nil
	}
	value := request.string("message_effect_id")
	effectID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || effectID <= 0 {
		return nil, errors.New("message_effect_id is invalid")
	}
	return wrapperspb.Int64(effectID), nil
}

func botAPIMessageInvertMedia(request *botAPIRequest) (bool, error) {
	_, invertMedia, err := botAPILinkPreviewOptions(request)
	if err != nil {
		return false, err
	}
	showCaptionAboveMedia, err := request.bool("show_caption_above_media")
	if err != nil {
		return false, errBotAPIShowCaptionAboveMediaInvalid
	}
	return invertMedia || showCaptionAboveMedia, nil
}

func botAPINoWebpage(request *botAPIRequest) (bool, error) {
	disabled, _, err := botAPILinkPreviewOptions(request)
	return disabled, err
}

func botAPILinkPreviewOptions(request *botAPIRequest) (disabled, invertMedia bool, err error) {
	disabled, err = request.bool("disable_web_page_preview")
	if err != nil {
		return false, false, err
	}
	if raw := request.raw("link_preview_options"); len(raw) > 0 && string(raw) != "null" {
		var options struct {
			IsDisabled    bool `json:"is_disabled"`
			ShowAboveText bool `json:"show_above_text"`
		}
		if err := request.json("link_preview_options", &options); err != nil {
			return false, false, err
		}
		disabled = disabled || options.IsDisabled
		invertMedia = options.ShowAboveText
	}
	return disabled, invertMedia, nil
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

func botAPIHTTPMediaURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}

const botAPIRemoteMediaMaxBytes = 20 << 20

func fetchBotAPIRemoteMedia(parent context.Context, rawURL string) ([]byte, string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return nil, "", "", errors.New("invalid remote media URL")
	}
	if err := rejectBotAPIRemoteHost(parsed.Hostname()); err != nil {
		return nil, "", "", err
	}
	requestContext, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				return nil, splitErr
			}
			if err := rejectBotAPIRemoteHost(host); err != nil {
				return nil, err
			}
			ips, err := net.LookupIP(host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("remote media host cannot be resolved")
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req == nil || req.URL == nil || req.URL.User != nil || req.URL.Fragment != "" ||
				(!strings.EqualFold(req.URL.Scheme, "http") && !strings.EqualFold(req.URL.Scheme, "https")) {
				return errors.New("invalid remote media redirect")
			}
			return rejectBotAPIRemoteHost(req.URL.Hostname())
		},
	}
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("Accept", "image/*,audio/*,video/*,application/octet-stream;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > botAPIRemoteMediaMaxBytes {
		return nil, "", "", errors.New("remote media response is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, botAPIRemoteMediaMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > botAPIRemoteMediaMaxBytes {
		return nil, "", "", errors.New("remote media response is too large or empty")
	}
	name := path.Base(resp.Request.URL.Path)
	if name == "." || name == "/" || name == "" {
		name = "bot-api-media"
	}
	mimeType := resp.Header.Get("Content-Type")
	if parsedType, _, parseErr := mime.ParseMediaType(mimeType); parseErr == nil {
		mimeType = parsedType
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(name))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return data, mimeType, name, nil
}

func rejectBotAPIRemoteHost(host string) error {
	if strings.TrimSpace(host) == "" {
		return errors.New("remote media host is empty")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return errors.New("remote media host cannot be resolved")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("remote media host is not public")
		}
	}
	return nil
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
	return uploadBotAPIFileWithContext(botRPCContext(ctx, botID), service, botID, name, data)
}

// uploadBotAPIFileWithContext writes the temporary Bot API upload under the
// same creator that the downstream media service uses to read it. The caller
// supplies the already-authenticated RPC context so business connections keep
// their outgoing scope metadata while the file remains owned by the bot's
// media auth key.
func uploadBotAPIFileWithContext(ctx context.Context, service *svc.ServiceContext, creatorID int64, name string, data []byte) (*mtproto.InputFile, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if service == nil || service.DfsClient == nil || creatorID <= 0 || len(data) == 0 {
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
		_, err := service.DfsClient.DfsWriteFilePartData(ctx, &dfspb.TLDfsWriteFilePartData{
			Creator:        creatorID,
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
	if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
		return nil, status, code, description
	}
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
	if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
		return nil, status, code, description
	}
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
	if status, code, description, failed := botAPIProfileOptionsResult(request); failed {
		return nil, status, code, description
	}
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
	text, entities, formatErr := botAPITextEntities(request, "text", "entities", "parse_mode")
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if text == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: text is empty"
	}
	if err := validateBotAPITextLength(text, 4096, "text"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessage(ctx, r, bot, request, text, entities, markup)
}

func editMessageCaption(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	caption, entities, formatErr := botAPICaptionEntities(request, "caption", "caption_entities")
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if len(request.raw("caption")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: caption is empty"
	}
	if err := validateBotAPITextLength(caption, 1024, "caption"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
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
	if showCaptionAboveMedia, ok := mediaObject["show_caption_above_media"]; ok {
		request.values["show_caption_above_media"] = showCaptionAboveMedia
	}
	_, hasCaption := mediaObject["caption"]
	caption, captionEntities, formatErr := botAPIMediaGroupCaption(item)
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if err := validateBotAPITextLength(caption, 1024, "caption"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessageWithMedia(ctx, r, bot, request, caption, hasCaption, captionEntities, markup, media)
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
	if inlineID := strings.TrimSpace(request.string("inline_message_id")); inlineID != "" {
		if strings.TrimSpace(request.string("business_connection_id")) != "" {
			return nil, http.StatusBadRequest, 400, "Bad Request: business_connection_id cannot be used with inline_message_id"
		}
		if ctx == nil || ctx.InlineBotClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: inline message service is not configured"
		}
		id, err := parseBotAPIInlineMessageID(inlineID)
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: inline_message_id is invalid"
		}
		noWebpage, invertMedia, err := botAPILinkPreviewOptions(request)
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
		}
		showCaptionAboveMedia, showErr := request.bool("show_caption_above_media")
		if showErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: show_caption_above_media is invalid"
		}
		invertMedia = invertMedia || showCaptionAboveMedia
		edit := &mtproto.TLMessagesEditInlineBotMessage{
			NoWebpage:   noWebpage,
			InvertMedia: invertMedia,
			Id:          id,
			Media:       media,
			ReplyMarkup: markup,
		}
		if entities != nil {
			edit.Entities = botapi.MessageEntitiesToMTProto(entities)
		}
		if setText {
			edit.Message = wrapperspb.String(text)
		}
		ok, err := ctx.InlineBotClient.MessagesEditInlineBotMessage(botRPCContext(r.Context(), bot.GetUser().GetId()), edit)
		if err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		if !mtproto.FromBool(ok) {
			return nil, http.StatusBadRequest, 400, "Bad Request: inline message was not modified"
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
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	callCtx := botRPCContext(r.Context(), bot.GetUser().GetId())
	selfID := bot.GetUser().GetId()
	if connectionID := strings.TrimSpace(request.string("business_connection_id")); connectionID != "" {
		scope, scopeErr := resolveBotAPIBusinessScope(ctx, r.Context(), bot.GetUser().GetId(), connectionID)
		if scopeErr != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(scopeErr)
		}
		if !scope.CanReply {
			return nil, http.StatusBadRequest, 400, "Bad Request: business connection does not allow message edits"
		}
		callCtx = businessctx.WithOutgoingScope(callCtx, scope)
		selfID = scope.OwnerID
	}
	noWebpage, invertMedia, err := botAPILinkPreviewOptions(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: link_preview_options is invalid"
	}
	showCaptionAboveMedia, showErr := request.bool("show_caption_above_media")
	if showErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: show_caption_above_media is invalid"
	}
	invertMedia = invertMedia || showCaptionAboveMedia
	edit := &mtproto.TLMessagesEditMessage{
		NoWebpage:   noWebpage,
		InvertMedia: invertMedia,
		Peer:        botAPIInputPeer(selfID, peerType, peerID),
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
	updates, err := ctx.MessagesClient.MessagesEditMessage(callCtx, edit)
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
	if connectionID := strings.TrimSpace(request.string("business_connection_id")); connectionID != "" {
		scope, err := resolveBotAPIBusinessScope(ctx, r.Context(), botID, connectionID)
		if err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		messageID, err := request.int64("message_id")
		if err != nil || messageID <= 0 || messageID > int64(^uint32(0)>>1) {
			return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
		}
		if ctx == nil || ctx.MessagesClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
		}
		if _, err = ctx.MessagesClient.MessagesDeleteMessages(businessBotCallContext(r.Context(), botID, scope), &mtproto.TLMessagesDeleteMessages{
			Revoke: true,
			Id:     []int32{int32(messageID)},
		}); err != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(err)
		}
		return true, http.StatusOK, 0, ""
	}
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
	if len(request.raw("ok")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: ok is required"
	}
	ok, err := request.bool("ok")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: ok is invalid"
	}
	result := &mtproto.TLMessagesSetBotShippingResults{QueryId: queryID}
	if ok {
		if !request.has("shipping_options") || len(request.raw("shipping_options")) == 0 || string(request.raw("shipping_options")) == "null" {
			return nil, http.StatusBadRequest, 400, "Bad Request: shipping_options is required when ok is true"
		}
		var options []botapi.ShippingOption
		if err := request.json("shipping_options", &options); err != nil || len(options) == 0 {
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
	if ctx == nil || ctx.PaymentsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
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
	if len(request.raw("results")) == 0 || request.json("results", &rawResults) != nil || len(rawResults) > 50 {
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
				return nil, http.StatusNotImplemented, 501, "Not Implemented: this inline result type is not supported"
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
	if nextOffset != nil && len(nextOffset.GetValue()) > 64 {
		return nil, http.StatusBadRequest, 400, "Bad Request: next_offset is too long"
	}
	private, err := request.bool("is_personal")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_personal is invalid"
	}
	gallery, err := request.bool("is_gallery")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_gallery is invalid"
	}
	var resultButton botapi.InlineQueryResultsButton
	hasResultButton := len(request.raw("button")) > 0 && string(request.raw("button")) != "null"
	var switchPM *mtproto.InlineBotSwitchPM
	var switchWebview *mtproto.InlineBotWebView
	if hasResultButton {
		if err := request.json("button", &resultButton); err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: button is invalid"
		}
		if text := strings.TrimSpace(resultButton.Text); text == "" || utf8.RuneCountInString(text) > 64 {
			return nil, http.StatusBadRequest, 400, "Bad Request: button.text is invalid"
		} else {
			resultButton.Text = text
		}
		if resultButton.WebApp != nil {
			if resultButton.StartParameter != "" || !validBotAPIWebAppURL(resultButton.WebApp.URL) {
				return nil, http.StatusBadRequest, 400, "Bad Request: button.web_app is invalid"
			}
			switchWebview = mtproto.MakeTLInlineBotWebView(&mtproto.InlineBotWebView{
				Text: resultButton.Text,
				Url:  strings.TrimSpace(resultButton.WebApp.URL),
			}).To_InlineBotWebView()
		} else if resultButton.StartParameter != "" {
			if !validBotAPIDeepLinkParameter(resultButton.StartParameter) {
				return nil, http.StatusBadRequest, 400, "Bad Request: button.start_parameter is invalid"
			}
			switchPM = mtproto.MakeTLInlineBotSwitchPM(&mtproto.InlineBotSwitchPM{
				Text:       resultButton.Text,
				StartParam: resultButton.StartParameter,
			}).To_InlineBotSwitchPM()
		} else {
			return nil, http.StatusBadRequest, 400, "Bad Request: button.web_app or button.start_parameter is required"
		}
	}
	switchPMText := request.string("switch_pm_text")
	switchPMParameter := request.string("switch_pm_parameter")
	if switchPMText != "" || switchPMParameter != "" {
		if hasResultButton {
			return nil, http.StatusBadRequest, 400, "Bad Request: button cannot be combined with switch_pm_*"
		}
		if utf8.RuneCountInString(switchPMText) == 0 || utf8.RuneCountInString(switchPMText) > 64 || !validBotAPIDeepLinkParameter(switchPMParameter) {
			return nil, http.StatusBadRequest, 400, "Bad Request: switch_pm_text and switch_pm_parameter must be provided together"
		}
		switchPM = mtproto.MakeTLInlineBotSwitchPM(&mtproto.InlineBotSwitchPM{Text: switchPMText, StartParam: switchPMParameter}).To_InlineBotSwitchPM()
	}
	ok, err := ctx.InlineBotClient.MessagesSetInlineBotResults(botRPCContext(requestCtx, botID), &mtproto.TLMessagesSetInlineBotResults{
		Gallery: gallery, Private: private, QueryId: queryID, Results: results, CacheTime: int32(cacheTime), NextOffset: nextOffset, SwitchPm: switchPM, SwitchWebview: switchWebview,
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
	if resultType == "" || resultID == "" || len(resultID) > 64 {
		return nil, errors.New("result type and id are required")
	}
	if resultURL := rawString(raw, "url"); resultURL != "" && !validBotAPIInlineURL(resultURL) {
		return nil, errors.New("inline result url is invalid")
	}
	if err := validateBotAPIInlineResultFields(raw, resultType); err != nil {
		return nil, err
	}
	sendMessage, err := botAPIInlineSendMessage(raw, resultType)
	if err != nil {
		return nil, err
	}
	base := &mtproto.InputBotInlineResult{
		Id:          resultID,
		Type:        botAPIInlineResultMTProtoType(resultType),
		Title:       optionalString(rawString(raw, "title")),
		Description: optionalString(rawString(raw, "description")),
		Url:         optionalString(rawString(raw, "url")),
		SendMessage: sendMessage,
	}
	switch resultType {
	case "article":
		if strings.TrimSpace(rawString(raw, "title")) == "" || sendMessage == nil ||
			(sendMessage.GetPredicateName() == mtproto.Predicate_inputBotInlineMessageText && strings.TrimSpace(sendMessage.GetMessage()) == "") {
			return nil, errors.New("article message is required")
		}
		if thumbURL := rawString(raw, "thumb_url"); thumbURL != "" {
			if !validBotAPIInlineURL(thumbURL) {
				return nil, errors.New("inline thumbnail url is invalid")
			}
			base.Thumb = botAPIInputWebDocument(thumbURL, "image/jpeg", rawInt(raw, "thumb_width"), rawInt(raw, "thumb_height"))
		}
		return mtproto.MakeTLInputBotInlineResult(base).To_InputBotInlineResult(), nil
	case "photo", "gif", "mpeg4_gif", "video", "audio", "voice", "document":
		contentURL, thumbURL, mimeType := botAPIInlineMediaURLs(raw, resultType)
		if contentURL == "" {
			return nil, errors.New("inline media url is required")
		}
		if !validBotAPIInlineURL(contentURL) || (thumbURL != "" && !validBotAPIInlineURL(thumbURL)) {
			return nil, errors.New("inline media url is invalid")
		}
		base.Content = botAPIInlineMediaDocument(raw, resultType, contentURL, mimeType)
		if thumbURL != "" {
			base.Thumb = botAPIInputWebDocument(thumbURL, "image/jpeg", rawInt(raw, "thumbnail_width"), rawInt(raw, "thumbnail_height"))
		}
		return mtproto.MakeTLInputBotInlineResult(base).To_InputBotInlineResult(), nil
	case "location":
		return mtproto.MakeTLInputBotInlineResult(base).To_InputBotInlineResult(), nil
	case "venue", "contact", "invoice":
		return mtproto.MakeTLInputBotInlineResult(base).To_InputBotInlineResult(), nil
	case "game":
		shortName := strings.TrimSpace(rawString(raw, "game_short_name"))
		if shortName == "" {
			return nil, errors.New("game_short_name is required")
		}
		base.ShortName = shortName
		return mtproto.MakeTLInputBotInlineResultGame(base).To_InputBotInlineResult(), nil
	case "cached_photo", "cached_document", "cached_gif", "cached_mpeg4_gif", "cached_video", "cached_audio", "cached_voice", "cached_sticker":
		field, expectedKind := botAPICachedInlineFileField(resultType)
		fileID := rawString(raw, field)
		ref, parseErr := parseBotAPIFileID(fileID)
		if parseErr != nil || ref.kind != expectedKind {
			if parseErr == nil {
				parseErr = errors.New("cached inline file kind does not match result type")
			}
			return nil, parseErr
		}
		if expectedKind == "photo" {
			base.Photo = mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: ref.id, AccessHash: ref.accessHash}).To_InputPhoto()
			return mtproto.MakeTLInputBotInlineResultPhoto(base).To_InputBotInlineResult(), nil
		}
		base.Document = mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: ref.id, AccessHash: ref.accessHash}).To_InputDocument()
		return mtproto.MakeTLInputBotInlineResultDocument(base).To_InputBotInlineResult(), nil
	default:
		return nil, errBotAPIWebAppResultUnsupported
	}
}

func botAPIInlineResultMTProtoType(resultType string) string {
	switch resultType {
	case "cached_photo":
		return "photo"
	case "cached_document":
		return "document"
	case "cached_gif":
		return "gif"
	case "cached_mpeg4_gif":
		return "mpeg4_gif"
	case "cached_video":
		return "video"
	case "cached_audio":
		return "audio"
	case "cached_voice":
		return "voice"
	case "cached_sticker":
		return "sticker"
	default:
		return resultType
	}
}

func validateBotAPIInlineResultFields(raw map[string]json.RawMessage, resultType string) error {
	require := func(key string) error {
		if strings.TrimSpace(rawString(raw, key)) == "" {
			return fmt.Errorf("%s is required", key)
		}
		return nil
	}
	requireURL := func(key string) error {
		value := rawString(raw, key)
		if value == "" || !validBotAPIInlineURL(value) {
			return fmt.Errorf("%s is invalid", key)
		}
		return nil
	}
	requireDimensions := func(widthKey, heightKey string) error {
		if rawInt(raw, widthKey) <= 0 || rawInt(raw, heightKey) <= 0 {
			return fmt.Errorf("%s and %s are required", widthKey, heightKey)
		}
		return nil
	}
	switch resultType {
	case "article":
		return require("title")
	case "photo":
		if err := requireURL("photo_url"); err != nil {
			return err
		}
		if err := requireURL("thumbnail_url"); err != nil {
			return err
		}
		return requireDimensions("photo_width", "photo_height")
	case "gif":
		if err := requireURL("gif_url"); err != nil {
			return err
		}
		return requireURL("thumbnail_url")
	case "mpeg4_gif":
		if err := requireURL("mpeg4_url"); err != nil {
			return err
		}
		return requireURL("thumbnail_url")
	case "video":
		if err := require("title"); err != nil {
			return err
		}
		if err := requireURL("video_url"); err != nil {
			return err
		}
		if err := require("mime_type"); err != nil {
			return err
		}
		if err := requireURL("thumbnail_url"); err != nil {
			return err
		}
		return requireDimensions("video_width", "video_height")
	case "audio":
		if err := require("title"); err != nil {
			return err
		}
		return requireURL("audio_url")
	case "voice":
		if err := require("title"); err != nil {
			return err
		}
		return requireURL("voice_url")
	case "document":
		if err := require("title"); err != nil {
			return err
		}
		if err := requireURL("document_url"); err != nil {
			return err
		}
		return require("mime_type")
	case "location":
		if err := require("title"); err != nil {
			return err
		}
		latitude, latitudeSet, latitudeErr := rawInlineFloat(raw, nil, "latitude")
		longitude, longitudeSet, longitudeErr := rawInlineFloat(raw, nil, "longitude")
		if latitudeErr != nil || longitudeErr != nil || !latitudeSet || !longitudeSet || math.IsNaN(latitude) || math.IsInf(latitude, 0) || latitude < -90 || latitude > 90 || math.IsNaN(longitude) || math.IsInf(longitude, 0) || longitude < -180 || longitude > 180 {
			return errors.New("location coordinates are invalid")
		}
	case "venue":
		if err := require("title"); err != nil {
			return err
		}
		if err := require("address"); err != nil {
			return err
		}
		latitude, latitudeSet, latitudeErr := rawInlineFloat(raw, nil, "latitude")
		longitude, longitudeSet, longitudeErr := rawInlineFloat(raw, nil, "longitude")
		if latitudeErr != nil || longitudeErr != nil || !latitudeSet || !longitudeSet || math.IsNaN(latitude) || math.IsInf(latitude, 0) || latitude < -90 || latitude > 90 || math.IsNaN(longitude) || math.IsInf(longitude, 0) || longitude < -180 || longitude > 180 {
			return errors.New("venue coordinates are invalid")
		}
	case "contact":
		if err := require("phone_number"); err != nil {
			return err
		}
		return require("first_name")
	case "game":
		return require("game_short_name")
	case "invoice":
		_, err := botAPIInvoiceMedia(&botAPIRequest{values: raw})
		return err
	case "cached_photo", "cached_document", "cached_gif", "cached_mpeg4_gif", "cached_video", "cached_audio", "cached_voice", "cached_sticker":
		field, _ := botAPICachedInlineFileField(resultType)
		return require(field)
	default:
		return errBotAPIWebAppResultUnsupported
	}
	return nil
}

func botAPIInlineSendMessage(raw map[string]json.RawMessage, resultType string) (*mtproto.InputBotInlineMessage, error) {
	content := rawObject(raw, "input_message_content")
	messageText := rawString(content, "message_text")
	if messageText == "" {
		messageText = rawString(raw, "message_text")
	}
	if messageText == "" {
		messageText = rawString(raw, "caption")
	}
	var entities []botapi.MessageEntity
	if entityValue, ok := content["entities"]; ok {
		if err := json.Unmarshal(entityValue, &entities); err != nil {
			return nil, err
		}
	}
	if entityValue, ok := raw["caption_entities"]; len(entities) == 0 && ok {
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
	replyMarkup := botapi.ReplyMarkupToMTProto(markup)
	noWebpage, invertMedia, forceLargeMedia, forceSmallMedia, err := botAPIInlineLinkPreviewOptions(raw, content)
	if err != nil {
		return nil, err
	}
	inlineMessage := &mtproto.InputBotInlineMessage{
		NoWebpage:       noWebpage,
		InvertMedia:     invertMedia,
		ForceLargeMedia: forceLargeMedia,
		ForceSmallMedia: forceSmallMedia,
		ReplyMarkup:     replyMarkup,
	}
	messageType := resultType
	if resultType == "article" {
		messageType = botAPIInlineInputContentType(content)
	}
	switch messageType {
	case "location":
		latitude, latitudeSet, latitudeErr := rawInlineFloat(raw, content, "latitude")
		longitude, longitudeSet, longitudeErr := rawInlineFloat(raw, content, "longitude")
		if latitudeErr != nil || longitudeErr != nil || !latitudeSet || !longitudeSet || math.IsNaN(latitude) || math.IsNaN(longitude) || math.IsInf(latitude, 0) || math.IsInf(longitude, 0) || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
			return nil, errors.New("location coordinates are invalid")
		}
		horizontalAccuracy, hasAccuracy, accuracyErr := rawInlineFloat(raw, content, "horizontal_accuracy")
		if accuracyErr != nil || (hasAccuracy && (math.IsNaN(horizontalAccuracy) || math.IsInf(horizontalAccuracy, 0) || horizontalAccuracy < 0 || horizontalAccuracy > float64(^uint32(0)>>1))) {
			return nil, errors.New("horizontal_accuracy is invalid")
		}
		livePeriod, hasLivePeriod, livePeriodErr := rawInlineInt(raw, content, "live_period")
		heading, hasHeading, headingErr := rawInlineInt(raw, content, "heading")
		proximity, hasProximity, proximityErr := rawInlineInt(raw, content, "proximity_alert_radius")
		if livePeriodErr != nil || headingErr != nil || proximityErr != nil || (hasLivePeriod && (livePeriod < 0 || livePeriod > int64(^uint32(0)>>1))) || (hasHeading && (heading < 1 || heading > 360)) || (hasProximity && proximity <= 0) {
			return nil, errors.New("location options are invalid")
		}
		if (hasHeading || hasProximity) && (!hasLivePeriod || livePeriod == 0) {
			return nil, errors.New("live_period is required for live location options")
		}
		inlineMessage.GeoPoint = mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: latitude, Long: longitude, AccuracyRadius: inlineAccuracyRadius(horizontalAccuracy, hasAccuracy)}).To_InputGeoPoint()
		inlineMessage.Heading = inlineInt32Value(heading, hasHeading)
		inlineMessage.Period = inlineInt32Value(livePeriod, hasLivePeriod)
		inlineMessage.ProximityNotificationRadius = inlineInt32Value(proximity, hasProximity)
		return mtproto.MakeTLInputBotInlineMessageMediaGeo(inlineMessage).To_InputBotInlineMessage(), nil
	case "venue":
		latitude, latitudeSet, latitudeErr := rawInlineFloat(raw, content, "latitude")
		longitude, longitudeSet, longitudeErr := rawInlineFloat(raw, content, "longitude")
		if latitudeErr != nil || longitudeErr != nil || !latitudeSet || !longitudeSet || math.IsNaN(latitude) || math.IsNaN(longitude) || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
			return nil, errors.New("venue coordinates are invalid")
		}
		title := rawInlineString(raw, content, "title")
		address := rawInlineString(raw, content, "address")
		if strings.TrimSpace(title) == "" || strings.TrimSpace(address) == "" {
			return nil, errors.New("venue title and address are required")
		}
		provider, venueID, venueType := botAPIInlineVenueProvider(raw, content)
		inlineMessage.GeoPoint = mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: latitude, Long: longitude}).To_InputGeoPoint()
		inlineMessage.Title = title
		inlineMessage.Address = address
		inlineMessage.Provider = provider
		inlineMessage.VenueId = venueID
		inlineMessage.VenueType = venueType
		return mtproto.MakeTLInputBotInlineMessageMediaVenue(inlineMessage).To_InputBotInlineMessage(), nil
	case "contact":
		phoneNumber := rawInlineString(raw, content, "phone_number")
		firstName := rawInlineString(raw, content, "first_name")
		if strings.TrimSpace(phoneNumber) == "" || strings.TrimSpace(firstName) == "" {
			return nil, errors.New("contact phone_number and first_name are required")
		}
		inlineMessage.PhoneNumber = phoneNumber
		inlineMessage.FirstName = firstName
		inlineMessage.LastName = rawInlineString(raw, content, "last_name")
		inlineMessage.Vcard = rawInlineString(raw, content, "vcard")
		return mtproto.MakeTLInputBotInlineMessageMediaContact(inlineMessage).To_InputBotInlineMessage(), nil
	case "invoice":
		media, err := botAPIInvoiceMedia(&botAPIRequest{values: raw})
		if err != nil || media == nil || media.GetInvoice() == nil {
			if err == nil {
				err = errors.New("invoice fields are invalid")
			}
			return nil, err
		}
		inlineMessage.Title = media.GetTitle()
		inlineMessage.Description = media.GetDescription()
		inlineMessage.Photo = media.GetPhoto()
		inlineMessage.Invoice = media.GetInvoice()
		inlineMessage.Payload = media.GetPayload_BYTES()
		inlineMessage.Provider = media.GetProvider_STRING()
		inlineMessage.ProviderData = media.GetProviderData()
		if startParam := media.GetStartParam(); startParam != nil {
			inlineMessage.Url = startParam.GetValue()
		}
		return mtproto.MakeTLInputBotInlineMessageMediaInvoice(inlineMessage).To_InputBotInlineMessage(), nil
	case "game":
		return mtproto.MakeTLInputBotInlineMessageGame(inlineMessage).To_InputBotInlineMessage(), nil
	case "photo", "gif", "mpeg4_gif", "video", "audio", "voice", "document",
		"cached_photo", "cached_document", "cached_gif", "cached_mpeg4_gif", "cached_video", "cached_audio", "cached_voice", "cached_sticker":
		inlineMessage.Message = messageText
		inlineMessage.Entities = botapi.MessageEntitiesToMTProto(entities)
		return mtproto.MakeTLInputBotInlineMessageMediaAuto(inlineMessage).To_InputBotInlineMessage(), nil
	default:
		if messageText == "" && resultType == "article" {
			return nil, errors.New("input_message_content.message_text is required")
		}
		inlineMessage.Message = messageText
		inlineMessage.Entities = botapi.MessageEntitiesToMTProto(entities)
		return mtproto.MakeTLInputBotInlineMessageText(inlineMessage).To_InputBotInlineMessage(), nil
	}
}

func botAPIInlineLinkPreviewOptions(raw, content map[string]json.RawMessage) (noWebpage, invertMedia, forceLargeMedia, forceSmallMedia bool, err error) {
	if noWebpage, _, err = rawInlineBool(raw, content, "disable_web_page_preview"); err != nil {
		return false, false, false, false, err
	}
	optionsValue, ok := rawInlineValue(raw, content, "link_preview_options")
	if !ok || string(optionsValue) == "null" {
		return noWebpage, false, false, false, nil
	}
	var options map[string]json.RawMessage
	if err = json.Unmarshal(optionsValue, &options); err != nil || options == nil {
		if err == nil {
			err = errors.New("link_preview_options must be an object")
		}
		return false, false, false, false, err
	}
	readOption := func(name string) (bool, error) {
		value, present := options[name]
		if !present || string(value) == "null" {
			return false, nil
		}
		var result bool
		if err := json.Unmarshal(value, &result); err != nil {
			return false, err
		}
		return result, nil
	}
	if value, readErr := readOption("is_disabled"); readErr != nil {
		return false, false, false, false, readErr
	} else {
		noWebpage = noWebpage || value
	}
	if invertMedia, err = readOption("show_above_text"); err != nil {
		return false, false, false, false, err
	}
	if forceLargeMedia, err = readOption("prefer_large_media"); err != nil {
		return false, false, false, false, err
	}
	if forceSmallMedia, err = readOption("prefer_small_media"); err != nil {
		return false, false, false, false, err
	}
	if forceLargeMedia && forceSmallMedia {
		return false, false, false, false, errors.New("prefer_large_media and prefer_small_media are mutually exclusive")
	}
	return noWebpage, invertMedia, forceLargeMedia, forceSmallMedia, nil
}

func botAPIInlineInputContentType(content map[string]json.RawMessage) string {
	if len(content) == 0 {
		return "article"
	}
	_, latitude := content["latitude"]
	_, longitude := content["longitude"]
	if latitude && longitude {
		if _, title := content["title"]; title {
			if _, address := content["address"]; address {
				return "venue"
			}
		}
		return "location"
	}
	_, phoneNumber := content["phone_number"]
	_, firstName := content["first_name"]
	if phoneNumber && firstName {
		return "contact"
	}
	return "article"
}

func botAPICachedInlineFileField(resultType string) (field, kind string) {
	switch resultType {
	case "cached_photo":
		return "photo_file_id", "photo"
	case "cached_document":
		return "document_file_id", "document"
	case "cached_gif":
		return "gif_file_id", "document"
	case "cached_mpeg4_gif":
		return "mpeg4_file_id", "document"
	case "cached_video":
		return "video_file_id", "document"
	case "cached_audio":
		return "audio_file_id", "document"
	case "cached_voice":
		return "voice_file_id", "document"
	case "cached_sticker":
		return "sticker_file_id", "document"
	default:
		return "", ""
	}
}

func validBotAPIInlineURL(rawURL string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	return err == nil && parsed.Host != "" && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
}

func validBotAPIWebAppURL(rawURL string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Scheme, "https")
}

func validBotAPIDeepLinkParameter(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func botAPIInlineVenueProvider(raw, content map[string]json.RawMessage) (provider, venueID, venueType string) {
	if googleID := rawInlineString(raw, content, "google_place_id"); googleID != "" {
		return "google", googleID, rawInlineString(raw, content, "google_place_type")
	}
	if foursquareID := rawInlineString(raw, content, "foursquare_id"); foursquareID != "" {
		return "foursquare", foursquareID, rawInlineString(raw, content, "foursquare_type")
	}
	return "", "", ""
}

func botAPIInlineMediaURLs(raw map[string]json.RawMessage, resultType string) (content, thumb, mimeType string) {
	switch resultType {
	case "photo":
		content, thumb, mimeType = rawString(raw, "photo_url"), rawString(raw, "thumbnail_url"), "image/jpeg"
	case "gif":
		content, thumb, mimeType = rawString(raw, "gif_url"), rawString(raw, "thumbnail_url"), "image/gif"
	case "mpeg4_gif":
		content, thumb, mimeType = rawString(raw, "mpeg4_url"), rawString(raw, "thumbnail_url"), "video/mp4"
	case "video":
		content, thumb, mimeType = rawString(raw, "video_url"), rawString(raw, "thumbnail_url"), rawString(raw, "mime_type")
	case "audio":
		content, thumb, mimeType = rawString(raw, "audio_url"), "", "audio/mpeg"
	case "voice":
		content, thumb, mimeType = rawString(raw, "voice_url"), "", "audio/ogg"
	case "document":
		content, thumb, mimeType = rawString(raw, "document_url"), rawString(raw, "thumbnail_url"), rawString(raw, "mime_type")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return content, thumb, mimeType
}

func botAPIInputWebDocument(rawURL, mimeType string, width, height int) *mtproto.InputWebDocument {
	attributes := []*mtproto.DocumentAttribute{}
	if width > 0 || height > 0 {
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{W: int32(width), H: int32(height)}).To_DocumentAttribute())
	}
	return mtproto.MakeTLInputWebDocument(&mtproto.InputWebDocument{Url: rawURL, MimeType: mimeType, Attributes: attributes}).To_InputWebDocument()
}

func botAPIInlineMediaDocument(raw map[string]json.RawMessage, resultType, rawURL, mimeType string) *mtproto.InputWebDocument {
	width, height, duration := botAPIInlineMediaDimensions(raw, resultType)
	attributes := make([]*mtproto.DocumentAttribute, 0, 2)
	if width > 0 || height > 0 {
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
			W: int32(width), H: int32(height),
		}).To_DocumentAttribute())
	}
	switch resultType {
	case "gif":
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute())
	case "mpeg4_gif", "video":
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			W: int32(width), H: int32(height), Duration_INT32: int32(duration), SupportsStreaming: true,
		}).To_DocumentAttribute())
	case "audio", "voice":
		attributes = append(attributes, mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{
			Duration_INT32: int32(duration), Voice: resultType == "voice",
		}).To_DocumentAttribute())
	}
	return mtproto.MakeTLInputWebDocument(&mtproto.InputWebDocument{
		Url: rawURL, MimeType: mimeType, Attributes: attributes,
	}).To_InputWebDocument()
}

func botAPIInlineMediaDimensions(raw map[string]json.RawMessage, resultType string) (width, height, duration int) {
	widthKey, heightKey, durationKey := "", "", ""
	switch resultType {
	case "photo":
		widthKey, heightKey = "photo_width", "photo_height"
	case "gif":
		widthKey, heightKey, durationKey = "gif_width", "gif_height", "gif_duration"
	case "mpeg4_gif":
		widthKey, heightKey, durationKey = "mpeg4_width", "mpeg4_height", "mpeg4_duration"
	case "video":
		widthKey, heightKey, durationKey = "video_width", "video_height", "video_duration"
	case "audio":
		durationKey = "audio_duration"
	case "voice":
		durationKey = "voice_duration"
	}
	if widthKey != "" {
		width = rawInt(raw, widthKey)
		if width < 0 {
			width = 0
		}
	}
	if heightKey != "" {
		height = rawInt(raw, heightKey)
		if height < 0 {
			height = 0
		}
	}
	if durationKey != "" {
		duration = rawInt(raw, durationKey)
		if duration < 0 {
			duration = 0
		}
	}
	return width, height, duration
}

func rawInt(values map[string]json.RawMessage, key string) int {
	value, ok := values[key]
	if !ok {
		return 0
	}
	var parsed int
	if json.Unmarshal(value, &parsed) == nil {
		return parsed
	}
	parsed, _ = strconv.Atoi(strings.TrimSpace(string(value)))
	return parsed
}

func rawInlineValue(values, content map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	if content != nil {
		if value, ok := content[key]; ok {
			return value, true
		}
	}
	value, ok := values[key]
	return value, ok
}

func rawInlineString(values, content map[string]json.RawMessage, key string) string {
	value, ok := rawInlineValue(values, content, key)
	if !ok {
		return ""
	}
	var decoded string
	if json.Unmarshal(value, &decoded) == nil {
		return strings.TrimSpace(decoded)
	}
	return ""
}

func rawInlineBool(values, content map[string]json.RawMessage, key string) (bool, bool, error) {
	value, ok := rawInlineValue(values, content, key)
	if !ok || string(value) == "null" {
		return false, false, nil
	}
	var parsed bool
	if err := json.Unmarshal(value, &parsed); err != nil {
		return false, true, err
	}
	return parsed, true, nil
}

func rawInlineFloat(values, content map[string]json.RawMessage, key string) (float64, bool, error) {
	value, ok := rawInlineValue(values, content, key)
	if !ok || string(value) == "null" {
		return 0, false, nil
	}
	var parsed float64
	if err := json.Unmarshal(value, &parsed); err == nil {
		return parsed, true, nil
	}
	var textValue string
	if err := json.Unmarshal(value, &textValue); err != nil {
		return 0, true, err
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(textValue), 64)
	return parsed, true, err
}

func rawInlineInt(values, content map[string]json.RawMessage, key string) (int64, bool, error) {
	value, ok := rawInlineValue(values, content, key)
	if !ok || string(value) == "null" {
		return 0, false, nil
	}
	var parsed int64
	if err := json.Unmarshal(value, &parsed); err == nil {
		return parsed, true, nil
	}
	var textValue string
	if err := json.Unmarshal(value, &textValue); err != nil {
		return 0, true, err
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(textValue), 10, 64)
	return parsed, true, err
}

func inlineInt32Value(value int64, present bool) *wrapperspb.Int32Value {
	if !present {
		return nil
	}
	return wrapperspb.Int32(int32(value))
}

func inlineAccuracyRadius(value float64, present bool) *wrapperspb.Int32Value {
	if !present || value <= 0 {
		return nil
	}
	return wrapperspb.Int32(int32(value))
}

func rawFloat(values map[string]json.RawMessage, key string) float64 {
	value, ok := values[key]
	if !ok {
		return 0
	}
	var parsed float64
	if json.Unmarshal(value, &parsed) == nil {
		return parsed
	}
	parsed, _ = strconv.ParseFloat(strings.TrimSpace(string(value)), 64)
	return parsed
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
		return &botapi.Chat{
			ID: converted.ID, Type: "private", FirstName: converted.FirstName,
			LastName: converted.LastName, Username: converted.Username,
			Photo: botapi.ChatPhotoFromUserProfilePhoto(user.ProfilePhoto()),
		}, http.StatusOK, 0, ""
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
			if full, fullErr := ctx.ChannelsClient.ChannelsGetFullChannel(callCtx, &mtproto.TLChannelsGetFullChannel{
				Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
			}); fullErr == nil && full != nil && full.GetFullChat() != nil {
				fullChat := full.GetFullChat()
				chat.Description = fullChat.GetAbout()
				chat.HasHiddenMembers = fullChat.GetParticipantsHidden()
				chat.HasAggressiveAntiSpam = fullChat.GetAntispam()
				chat.Permissions = botapi.ChatPermissionsFromBannedRights(result.GetChats()[0].GetDefaultBannedRights())
				if fullChat.GetSlowmodeSeconds() != nil {
					chat.SlowModeDelay = int(fullChat.GetSlowmodeSeconds().GetValue())
				}
				if fullChat.GetTtlPeriod() != nil {
					chat.MessageAutoDeleteTime = int(fullChat.GetTtlPeriod().GetValue())
				}
				if fullChat.GetChatPhoto() != nil {
					chat.Photo = botapi.ChatPhotoFromPhoto(fullChat.GetChatPhoto())
				}
				if invite := fullChat.GetExportedInvite(); invite != nil {
					chat.InviteLink = invite.GetLink()
				}
			}
			return chat, http.StatusOK, 0, ""
		}
	}
	return nil, http.StatusBadRequest, 400, "Bad Request: chat is not available"
}

func getChatAdministrators(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	returnBots, err := request.bool("return_bots")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: return_bots is invalid"
	}
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
				if !botAPIShouldReturnAdministrator(member, botID, returnBots) {
					continue
				}
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
		if member != nil && (member.Status == "administrator" || member.Status == "creator") && botAPIShouldReturnAdministrator(member, botID, returnBots) {
			result = append(result, member)
		}
	}
	return result, http.StatusOK, 0, ""
}

func botAPIShouldReturnAdministrator(member *botapi.ChatMember, botID int64, returnBots bool) bool {
	if member == nil || member.User == nil {
		return member != nil
	}
	return !member.User.IsBot || returnBots || member.User.ID == botID
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
	parameter, rightErr := botAPIUnsupportedAdminRight(request)
	if rightErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + rightErr.Error()
	}
	if parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 admin-rights adapter"
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
	if !request.has("can_restrict_members") {
		// Telegram preserves the historical channel-promotion default.
		rights.BanUsers = true
	}
	if chatAdminRightsSet(rights) {
		// can_manage_chat is implied by every other administrator privilege.
		rights.Other = true
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

func botAPIUnsupportedAdminRight(request *botAPIRequest) (string, error) {
	if request == nil {
		return "", nil
	}
	for _, name := range []string{"can_manage_tags", "can_send_welcome_messages"} {
		value, err := request.bool(name)
		if err != nil {
			return "", fmt.Errorf("%s is invalid", name)
		}
		if value {
			return name, nil
		}
	}
	return "", nil
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
	isAdministrator := current.GetPredicateName() == mtproto.Predicate_channelParticipantAdmin || current.GetPredicateName() == mtproto.Predicate_channelParticipantCreator
	// channels.getParticipant returns channelParticipantSelf when the bot edits
	// its own administrator title. Preserve the admin rights carried by that
	// constructor instead of rejecting a valid administrator.
	if current.GetPredicateName() == mtproto.Predicate_channelParticipantSelf && current.GetAdminRights() != nil {
		isAdministrator = true
	}
	if !isAdministrator {
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
