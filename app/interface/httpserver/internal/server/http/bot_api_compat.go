// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func botAPIGeneralForumTopicRequest(request *botAPIRequest) *botAPIRequest {
	values := make(map[string]json.RawMessage)
	if request != nil {
		for name, value := range request.values {
			values[name] = value
		}
	}
	values["message_thread_id"] = json.RawMessage("1")
	return &botAPIRequest{values: values}
}

func editGeneralForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if request == nil || request.string("name") == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is required"
	}
	return editForumTopic(ctx, r, botID, botAPIGeneralForumTopicRequest(request), false, false, false, false)
}

func closeGeneralForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return closeForumTopic(ctx, r, botID, botAPIGeneralForumTopicRequest(request))
}

func reopenGeneralForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return reopenForumTopic(ctx, r, botID, botAPIGeneralForumTopicRequest(request))
}

func unpinAllGeneralForumTopicMessages(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return unpinAllForumTopicMessages(ctx, r, botID, botAPIGeneralForumTopicRequest(request))
}

func deleteMessages(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	ids, err := botAPIBatchMessageIDs(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_ids is invalid"
	}
	if ctx == nil || ctx.MsgClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	if _, err := ctx.MsgClient.MsgDeleteMessages(botRPCContext(r.Context(), botID), &msgpb.TLMsgDeleteMessages{
		UserId: botID, PeerType: peerType, PeerId: peerID, Revoke: true, Id: ids,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteMessageReaction(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	participant, err := botAPIReactionParticipant(ctx, r, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reaction participant is invalid"
	}
	if ctx == nil || ctx.ReactionsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: reaction service is not configured"
	}
	if _, err := ctx.ReactionsClient.MessagesDeleteParticipantReaction(botRPCContext(r.Context(), botID), &mtproto.TLMessagesDeleteParticipantReaction{
		Peer: botAPIInputPeer(botID, peerType, peerID), MsgId: int32(messageID), Participant: participant,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteAllMessageReactions(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	participant, err := botAPIReactionParticipant(ctx, r, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reaction participant is invalid"
	}
	if ctx == nil || ctx.ReactionsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: reaction service is not configured"
	}
	if _, err := ctx.ReactionsClient.MessagesDeleteParticipantReactions(botRPCContext(r.Context(), botID), &mtproto.TLMessagesDeleteParticipantReactions{
		Peer: botAPIInputPeer(botID, peerType, peerID), Participant: participant,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func botAPIBatchMessageIDs(request *botAPIRequest) ([]int32, error) {
	var rawIDs []int64
	if request == nil || request.json("message_ids", &rawIDs) != nil || len(rawIDs) == 0 || len(rawIDs) > 100 {
		return nil, errors.New("message_ids must contain between 1 and 100 message ids")
	}
	ids := make([]int32, 0, len(rawIDs))
	seen := make(map[int32]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		if rawID <= 0 || rawID > int64(^uint32(0)>>1) {
			return nil, errors.New("message_ids contains an invalid message id")
		}
		id := int32(rawID)
		if _, ok := seen[id]; ok {
			return nil, errors.New("message_ids contains duplicates")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func botAPIReactionParticipant(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (*mtproto.InputPeer, error) {
	userID, userErr := request.int64("user_id")
	actorChatID := strings.TrimSpace(request.string("actor_chat_id"))
	if userErr != nil || (request.has("user_id") && userID <= 0) || (userID != 0 && actorChatID != "") {
		return nil, errors.New("user_id and actor_chat_id are mutually exclusive")
	}
	if userID != 0 {
		return botAPIInputPeer(botID, mtproto.PEER_USER, userID), nil
	}
	if actorChatID == "" {
		return nil, nil
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, actorChatID)
	if err != nil || peerID == 0 || peerType == mtproto.PEER_USER {
		return nil, errors.New("actor_chat_id is invalid")
	}
	return botAPIInputPeer(botID, peerType, peerID), nil
}

// forwardMessages implements Telegram's bulk forwarding and copying methods.
// The MTProto request preserves the order of message_ids and returns one
// update per input message, which maps directly to the Bot API result shape.
func forwardMessages(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest, copyMode bool) (any, int, int, string) {
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	if bot == nil || bot.GetUser() == nil {
		return nil, http.StatusUnauthorized, 401, "Unauthorized"
	}
	if request == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: request is required"
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

	var rawIDs []int64
	if len(request.raw("message_ids")) == 0 || request.json("message_ids", &rawIDs) != nil || len(rawIDs) == 0 || len(rawIDs) > 100 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_ids must contain between 1 and 100 message ids"
	}
	messageIDs := make([]int32, len(rawIDs))
	randomIDs := make([]int64, len(rawIDs))
	for i, id := range rawIDs {
		if id <= 0 || id > int64(^uint32(0)>>1) {
			return nil, http.StatusBadRequest, 400, "Bad Request: message_ids contains an invalid message id"
		}
		messageIDs[i] = int32(id)
		randomIDs[i] = randomMessageID()
	}

	disableNotification, err := request.bool("disable_notification")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: disable_notification is invalid"
	}
	protectContent, err := request.bool("protect_content")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: protect_content is invalid"
	}
	removeCaption, err := request.bool("remove_caption")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: remove_caption is invalid"
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	topMsgID, err := botAPIOptionalInt32(request, "message_thread_id", 1, int64(^uint32(0)>>1))
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}

	updates, err := ctx.MessagesClient.MessagesForwardMessages(botMediaRPCContext(r.Context(), botID), &mtproto.TLMessagesForwardMessages{
		FromPeer:          botAPIInputPeer(botID, fromPeerType, fromPeerID),
		Id:                messageIDs,
		RandomId:          randomIDs,
		ToPeer:            botAPIInputPeer(botID, toPeerType, toPeerID),
		Silent:            disableNotification,
		Noforwards:        protectContent,
		DropAuthor:        copyMode,
		DropMediaCaptions: removeCaption,
		ReplyTo:           replyTo,
		TopMsgId:          topMsgID,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	messages := botapi.MessagesFromUpdates(updates)
	if len(messages) == 0 {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: messages were not returned"
	}
	if copyMode {
		result := make([]botapi.MessageID, 0, len(messages))
		for _, message := range messages {
			if message != nil {
				result = append(result, botapi.MessageID{MessageID: message.MessageID})
			}
		}
		if len(result) == 0 {
			return nil, http.StatusInternalServerError, 500, "Internal Server Error: copied messages were not returned"
		}
		return result, http.StatusOK, 0, ""
	}
	return messages, http.StatusOK, 0, ""
}

func editMessageLiveLocation(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	media, err := botAPILiveLocationMedia(request, false)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessageWithMedia(ctx, r, bot, request, "", false, nil, markup, media)
}

func stopMessageLiveLocation(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	media, err := botAPILiveLocationMedia(request, true)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	return editBotAPIMessageWithMedia(ctx, r, bot, request, "", false, nil, markup, media)
}

func botAPILiveLocationMedia(request *botAPIRequest, stopped bool) (*mtproto.InputMedia, error) {
	if request == nil {
		return nil, errors.New("request is required")
	}
	hasLongitude := len(request.raw("longitude")) > 0 && string(request.raw("longitude")) != "null"
	hasLatitude := len(request.raw("latitude")) > 0 && string(request.raw("latitude")) != "null"
	if !stopped && (!hasLongitude || !hasLatitude) {
		return nil, errors.New("longitude and latitude are required")
	}
	longitude, latitude := 0.0, 0.0
	if hasLongitude || hasLatitude {
		var err error
		longitude, latitude, err = botAPILocationCoordinates(request)
		if err != nil || math.IsNaN(longitude) || math.IsInf(longitude, 0) || math.IsNaN(latitude) || math.IsInf(latitude, 0) {
			return nil, errors.New("longitude or latitude is invalid")
		}
	}
	geo := &mtproto.InputGeoPoint{Long: longitude, Lat: latitude}
	if len(request.raw("horizontal_accuracy")) > 0 && string(request.raw("horizontal_accuracy")) != "null" {
		accuracy, err := request.float64("horizontal_accuracy")
		if err != nil || math.IsNaN(accuracy) || math.IsInf(accuracy, 0) || accuracy < 0 || accuracy > 1500 {
			return nil, errors.New("horizontal_accuracy is invalid")
		}
		geo.AccuracyRadius = wrapperspb.Int32(int32(accuracy))
	}
	heading, err := botAPIOptionalInt32(request, "heading", 0, 360)
	if err != nil {
		return nil, errors.New("heading is invalid")
	}
	proximity, err := botAPIOptionalInt32(request, "proximity_alert_radius", 0, int64(^uint32(0)>>1))
	if err != nil {
		return nil, errors.New("proximity_alert_radius is invalid")
	}
	media := &mtproto.InputMedia{
		GeoPoint:                    mtproto.MakeTLInputGeoPoint(geo).To_InputGeoPoint(),
		Heading:                     heading,
		ProximityNotificationRadius: proximity,
		Stopped:                     stopped,
	}
	return mtproto.MakeTLInputMediaGeoLive(media).To_InputMedia(), nil
}
