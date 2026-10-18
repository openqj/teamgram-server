// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	errBotAPIEphemeralCallbackUnsupported = errors.New("ephemeral callback_query_id is not represented by the Layer 229 adapter")
	errBotAPIEphemeralReplaceUnsupported  = errors.New("replace_callback_query_message is not represented by the Layer 229 adapter")
	errBotAPIEphemeralInvertUnsupported   = errors.New("show_caption_above_media and link_preview_options.invert_media are not represented by the Layer 229 adapter")
	errBotAPIDirectTopicUnsupported       = errors.New("direct_messages_topic_id is not represented by the Layer 229 message adapter")
	errBotAPIDirectTopicOverflow          = errors.New("direct_messages_topic_id exceeds the Layer 229 int32 topic field")
)

type botAPIEphemeralParameters struct {
	ReceiverUserID              int64  `json:"receiver_user_id"`
	CallbackQueryID             string `json:"callback_query_id,omitempty"`
	ReplaceCallbackQueryMessage bool   `json:"replace_callback_query_message,omitempty"`
}

// botAPIEphemeralParametersValue parses the nested Bot API object while
// keeping absent and explicit null values distinct from malformed objects.
func botAPIEphemeralParametersValue(request *botAPIRequest) (*botAPIEphemeralParameters, error) {
	if request == nil || !request.has("ephemeral_message_parameters") {
		return nil, nil
	}
	raw := strings.TrimSpace(string(request.raw("ephemeral_message_parameters")))
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var parameters botAPIEphemeralParameters
	if err := request.json("ephemeral_message_parameters", &parameters); err != nil {
		return nil, errors.New("ephemeral_message_parameters is invalid")
	}
	if parameters.ReceiverUserID <= 0 {
		return nil, errors.New("ephemeral_message_parameters.receiver_user_id is invalid")
	}
	return &parameters, nil
}

// botAPIMessageTopID reads the forum topic field used by the existing message
// adapters. A direct-messages topic is a distinct Bot API concept and must not
// be silently sent as a forum top_msg_id.
func botAPIMessageTopID(request *botAPIRequest) (*wrapperspb.Int32Value, error) {
	messageThreadID, err := botAPIOptionalInt32(request, "message_thread_id", 1, int64(^uint32(0)>>1))
	if err != nil {
		return nil, errors.New("message_thread_id is invalid")
	}
	if request != nil && request.has("direct_messages_topic_id") {
		raw := strings.TrimSpace(string(request.raw("direct_messages_topic_id")))
		if raw != "" && raw != "null" {
			value, parseErr := request.int64("direct_messages_topic_id")
			if parseErr != nil || value <= 0 {
				return nil, errors.New("direct_messages_topic_id is invalid")
			}
			if value > int64(^uint32(0)>>1) {
				return nil, errBotAPIDirectTopicOverflow
			}
			return nil, errBotAPIDirectTopicUnsupported
		}
	}
	return messageThreadID, nil
}

func botAPIMessageTopIDError(err error) (int, int, string) {
	if errors.Is(err, errBotAPIDirectTopicUnsupported) || errors.Is(err, errBotAPIDirectTopicOverflow) {
		return http.StatusNotImplemented, 501, "Not Implemented: " + err.Error()
	}
	return http.StatusBadRequest, 400, "Bad Request: " + err.Error()
}

func botAPIUnsupportedMessageParameterExcept(request *botAPIRequest, allowed ...string) string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	for _, name := range []string{"direct_messages_topic_id", "ephemeral_message_parameters", "suggested_post_parameters"} {
		if _, ok := allowedSet[name]; ok || request == nil || !request.has(name) {
			continue
		}
		raw := strings.TrimSpace(string(request.raw(name)))
		if raw != "" && raw != "null" && raw != `""` {
			return name
		}
	}
	return ""
}

// sendBotAPIEphemeral sends one Bot API message through the existing
// RPCEphemeral adapter. The current TL contract can carry the receiver and
// payload, but has no callback-query or replace flag, so those requests fail
// explicitly instead of silently changing their delivery semantics.
func sendBotAPIEphemeral(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, peerType int32, peerID int64, message string, entities []*mtproto.MessageEntity, media *mtproto.InputMedia, rich *mtproto.InputRichMessage, markup *mtproto.ReplyMarkup, replyTo *mtproto.InputReplyTo) (any, int, int, string) {
	parameters, err := botAPIEphemeralParametersValue(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if parameters == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: ephemeral parameters were not provided"
	}
	if parameters.ReplaceCallbackQueryMessage {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + errBotAPIEphemeralReplaceUnsupported.Error()
	}
	if parameters.CallbackQueryID != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + errBotAPIEphemeralCallbackUnsupported.Error()
	}
	if peerType != mtproto.PEER_CHAT && peerType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: ephemeral messages require a group or supergroup chat"
	}
	if ctx == nil || ctx.EphemeralClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral message service is not configured"
	}
	requestCtx := context.Background()
	if r != nil {
		requestCtx = r.Context()
	}
	receiver, err := botAPIProfilePhotoInputUser(ctx, requestCtx, botID, parameters.ReceiverUserID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	updates, err := ctx.EphemeralClient.EphemeralSendMessage(botRPCContext(requestCtx, botID), &mtproto.TLEphemeralSendMessage{
		Peer:        botAPIInputPeer(botID, peerType, peerID),
		ReceiverId:  receiver,
		Message:     message,
		Entities:    entities,
		Media:       media,
		RichMessage: rich,
		ReplyMarkup: markup,
		ReplyTo:     replyTo,
		RandomId:    randomMessageID(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result := botAPIFirstEphemeralMessage(updates)
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: ephemeral message was not returned"
	}
	return result, http.StatusOK, 0, ""
}

func botAPIFirstEphemeralMessage(updates *mtproto.Updates) *botapi.Message {
	if updates == nil {
		return nil
	}
	var result *botapi.Message
	mtproto.VisitUpdates(0, updates, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewEphemeralMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			converted, ok := botapi.UpdateFromMTProto(update, users, chats)
			if ok && converted != nil {
				result = converted.Message
			}
		},
		mtproto.Predicate_updateEditEphemeralMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			converted, ok := botapi.UpdateFromMTProto(update, users, chats)
			if ok && converted != nil {
				result = converted.EditedMessage
			}
		},
	})
	return result
}
