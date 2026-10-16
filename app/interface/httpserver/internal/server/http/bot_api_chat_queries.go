// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const botAPIChatSubscriptionPeriod = int64(30 * 24 * 60 * 60)

func setChatMemberTag(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.ChatsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || (peerType != mtproto.PEER_CHAT && peerType != mtproto.PEER_CHANNEL) {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a group or supergroup"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	tag := ""
	if request.has("tag") && string(request.raw("tag")) != "null" {
		tag = request.string("tag")
		if !utf8.ValidString(tag) || utf8.RuneCountInString(tag) > 16 || botAPIContainsEmoji(tag) {
			return nil, http.StatusBadRequest, 400, "Bad Request: tag is invalid"
		}
	}
	_, err = ctx.ChatsClient.MessagesEditChatParticipantRank(botRPCContext(r.Context(), botID), &mtproto.TLMessagesEditChatParticipantRank{
		Peer:        botAPIInputPeer(botID, peerType, peerID),
		Participant: botAPIInputPeer(botID, mtproto.PEER_USER, userID),
		Rank:        tag,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func botAPIContainsEmoji(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.So, r) || (r >= '\U0001f3fb' && r <= '\U0001f3ff') || r == '\u200d' || (r >= '\ufe00' && r <= '\ufe0f') {
			return true
		}
	}
	return false
}

func createChatSubscriptionInviteLink(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	peer, peerType, _, err := botAPIInvitePeer(r, ctx, botID, request)
	if err != nil || peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a channel"
	}
	if ctx == nil || ctx.ChatInvitesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service is not configured"
	}
	name := ""
	if request.has("name") && string(request.raw("name")) != "null" {
		name = request.string("name")
		if utf8.RuneCountInString(name) > 32 {
			return nil, http.StatusBadRequest, 400, "Bad Request: name is too long"
		}
	}
	period, err := request.int64("subscription_period")
	if err != nil || period != botAPIChatSubscriptionPeriod {
		return nil, http.StatusBadRequest, 400, "Bad Request: subscription_period is invalid"
	}
	price, err := request.int64("subscription_price")
	if err != nil || price < 1 || price > 10000 {
		return nil, http.StatusBadRequest, 400, "Bad Request: subscription_price is invalid"
	}
	invite, err := ctx.ChatInvitesClient.MessagesExportChatInvite(botRPCContext(r.Context(), botID), &mtproto.TLMessagesExportChatInvite{
		Peer:  peer,
		Title: optionalString(name),
		SubscriptionPricing: mtproto.MakeTLStarsSubscriptionPricing(&mtproto.StarsSubscriptionPricing{
			Period: int32(period),
			Amount: price,
		}).To_StarsSubscriptionPricing(),
	})
	if err != nil || invite == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if pricing := invite.GetSubscriptionPricing(); pricing == nil || pricing.GetPeriod() != int32(period) || pricing.GetAmount() != price {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service did not return subscription pricing"
	}
	creator := botAPIInviteCreator(ctx, r.Context(), botID, invite.GetAdminId())
	return botAPIInviteLinkFromMTProto(invite, creator), http.StatusOK, 0, ""
}

func editChatSubscriptionInviteLink(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	peer, peerType, _, err := botAPIInvitePeer(r, ctx, botID, request)
	if err != nil || peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a channel"
	}
	if ctx == nil || ctx.ChatInvitesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service is not configured"
	}
	link := request.string("invite_link")
	if link == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: invite_link is required"
	}
	edit := &mtproto.TLMessagesEditExportedChatInvite{Peer: peer, Link: link}
	if request.has("name") && string(request.raw("name")) != "null" {
		name := request.string("name")
		if utf8.RuneCountInString(name) > 32 {
			return nil, http.StatusBadRequest, 400, "Bad Request: name is too long"
		}
		edit.Title = wrapperspb.String(name)
	}
	result, err := ctx.ChatInvitesClient.MessagesEditExportedChatInvite(botRPCContext(r.Context(), botID), edit)
	invite := botAPIExportedChatInvite(result)
	if err != nil || invite == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if pricing := invite.GetSubscriptionPricing(); pricing == nil || int64(pricing.GetPeriod()) != botAPIChatSubscriptionPeriod || pricing.GetAmount() < 1 || pricing.GetAmount() > 10000 {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service did not return subscription pricing"
	}
	creator := botAPIInviteCreator(ctx, r.Context(), botID, invite.GetAdminId())
	return botAPIInviteLinkFromMTProto(invite, creator), http.StatusOK, 0, ""
}

func parseBotAPIQueryID(request *botAPIRequest, name string) (int64, error) {
	value := strings.TrimSpace(request.string(name))
	if value == "" {
		return 0, errors.New("query id is required")
	}
	queryID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || queryID <= 0 {
		return 0, errors.New("query id is invalid")
	}
	return queryID, nil
}

func joinChatBotResult(result string, webAppURL string) (*mtproto.JoinChatBotResult, error) {
	base := &mtproto.JoinChatBotResult{Url: webAppURL}
	switch result {
	case "approve":
		return mtproto.MakeTLJoinChatBotResultApproved(base).To_JoinChatBotResult(), nil
	case "decline":
		return mtproto.MakeTLJoinChatBotResultDeclined(base).To_JoinChatBotResult(), nil
	case "queue":
		return mtproto.MakeTLJoinChatBotResultQueued(base).To_JoinChatBotResult(), nil
	case "web_view":
		if webAppURL == "" {
			return nil, errors.New("web app url is required")
		}
		return mtproto.MakeTLJoinChatBotResultWebView(base).To_JoinChatBotResult(), nil
	default:
		return nil, errors.New("result is invalid")
	}
}

func answerChatJoinRequestQuery(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BotsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: bot service is not configured"
	}
	queryID, err := parseBotAPIQueryID(request, "chat_join_request_query_id")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_join_request_query_id is invalid"
	}
	resultName := strings.TrimSpace(request.string("result"))
	result, err := joinChatBotResult(resultName, "")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: result must be approve, decline, or queue"
	}
	ok, err := ctx.BotsClient.BotsSetJoinChatResults(botRPCContext(r.Context(), botID), &mtproto.TLBotsSetJoinChatResults{QueryId: queryID, Result: result})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat join request query is invalid or expired"
	}
	return true, http.StatusOK, 0, ""
}

func sendChatJoinRequestWebApp(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BotsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: bot service is not configured"
	}
	queryID, err := parseBotAPIQueryID(request, "chat_join_request_query_id")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_join_request_query_id is invalid"
	}
	webAppURL := strings.TrimSpace(request.string("web_app_url"))
	if !validBotAPIWebAppURL(webAppURL) {
		return nil, http.StatusBadRequest, 400, "Bad Request: web_app_url must be an HTTPS URL"
	}
	result, _ := joinChatBotResult("web_view", webAppURL)
	ok, err := ctx.BotsClient.BotsSetJoinChatResults(botRPCContext(r.Context(), botID), &mtproto.TLBotsSetJoinChatResults{QueryId: queryID, Result: result})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat join request query is invalid or expired"
	}
	return true, http.StatusOK, 0, ""
}

func answerGuestQuery(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.InlineBotClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: inline query service is not configured"
	}
	queryID, err := parseBotAPIQueryID(request, "guest_query_id")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: guest_query_id is invalid"
	}
	rawResult := request.raw("result")
	if len(rawResult) > 0 && rawResult[0] == '"' {
		var encoded string
		if jsonErr := json.Unmarshal(rawResult, &encoded); jsonErr == nil {
			rawResult = json.RawMessage(encoded)
		}
	}
	var raw map[string]json.RawMessage
	if len(rawResult) == 0 || json.Unmarshal(rawResult, &raw) != nil || len(raw) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: result is invalid"
	}
	result, err := botAPIWebAppInlineResult(raw)
	if err != nil {
		if errors.Is(err, errBotAPIWebAppResultUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: this inline result type is not supported"
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: result is invalid"
	}
	sent, err := ctx.InlineBotClient.MessagesSetBotGuestChatResult(botRPCContext(r.Context(), botID), &mtproto.TLMessagesSetBotGuestChatResult{QueryId: queryID, Result: result})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	inlineID := botAPIInlineMessageIDString(sent)
	if inlineID == "" {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: guest message id was not returned"
	}
	return &botapi.SentGuestMessage{InlineMessageID: inlineID}, http.StatusOK, 0, ""
}

func botAPIInlineMessageIDString(in *mtproto.InputBotInlineMessageID) string {
	if in == nil || in.GetDcId() <= 0 || in.GetAccessHash() == 0 {
		return ""
	}
	if in.GetPredicateName() == mtproto.Predicate_inputBotInlineMessageID64 || in.GetOwnerId() != 0 {
		return strconv.FormatInt(int64(in.GetDcId()), 10) + ":" + strconv.FormatInt(in.GetOwnerId(), 10) + ":" + strconv.FormatInt(int64(in.GetId_INT32()), 10) + ":" + strconv.FormatInt(in.GetAccessHash(), 10)
	}
	return strconv.FormatInt(int64(in.GetDcId()), 10) + ":" + strconv.FormatInt(in.GetId_INT64(), 10) + ":" + strconv.FormatInt(in.GetAccessHash(), 10)
}
