// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"net/http"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

// stopPoll uses messages.editMessage with inputMediaPoll(stopped=true). The
// underlying message service preserves the existing poll and voter results,
// so the Bot API response remains the canonical closed Poll object.
func stopPoll(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	botID := bot.GetUser().GetId()
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 || messageID > int(^uint32(0)>>1) {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	updates, err := ctx.MessagesClient.MessagesEditMessage(botRPCContext(r.Context(), botID), &mtproto.TLMessagesEditMessage{
		Peer:        botAPIInputPeer(botID, peerType, peerID),
		Id:          int32(messageID),
		Media:       mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{Stopped: true}).To_InputMedia(),
		ReplyMarkup: markup,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	message := botapi.FirstMessageFromUpdates(updates)
	if message == nil || message.Poll == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: poll was not returned"
	}
	return message.Poll, http.StatusOK, 0, ""
}
