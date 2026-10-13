// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
	"google.golang.org/grpc/status"
)

func resolveBotAPIBusinessScope(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, connectionID string) (businessctx.Scope, error) {
	if ctx == nil || ctx.BusinessBotsClient == nil || botID <= 0 || strings.TrimSpace(connectionID) == "" {
		return businessctx.Scope{}, mtproto.ErrInputRequestInvalid
	}
	updates, err := ctx.BusinessBotsClient.AccountGetBotBusinessConnection(
		botRPCContext(requestCtx, botID),
		&mtproto.TLAccountGetBotBusinessConnection{ConnectionId: strings.TrimSpace(connectionID)},
	)
	if err != nil {
		return businessctx.Scope{}, err
	}
	connection := businessConnectionFromUpdates(updates, strings.TrimSpace(connectionID))
	if connection == nil || connection.GetUserId() <= 0 || connection.GetDisabled() {
		return businessctx.Scope{}, status.Error(mtproto.ErrBadRequest, "CONNECTION_ID_INVALID")
	}
	rights := connection.GetRights()
	return businessctx.Scope{
		ConnectionID:    connection.GetConnectionId(),
		OwnerID:         connection.GetUserId(),
		CanReply:        connection.GetCanReply() || (rights != nil && rights.GetReply()),
		CanReadMessages: rights != nil && rights.GetReadMessages(),
		CanDeleteSent:   rights != nil && rights.GetDeleteSentMessages(),
		CanDeleteAll:    rights != nil && rights.GetDeleteReceivedMessages(),
	}, nil
}

func businessConnectionFromUpdates(updates *mtproto.Updates, connectionID string) *mtproto.BotBusinessConnection {
	if updates == nil {
		return nil
	}
	var result *mtproto.BotBusinessConnection
	visit := func(update *mtproto.Update) {
		if update == nil || update.GetPredicateName() != mtproto.Predicate_updateBotBusinessConnect {
			return
		}
		connection := update.GetConnection()
		if connection != nil && connection.GetConnectionId() == connectionID {
			result = connection
		}
	}
	visit(updates.GetUpdate())
	for _, update := range updates.GetUpdates() {
		visit(update)
	}
	return result
}

func businessBotCallContext(parent context.Context, botID int64, scope businessctx.Scope) context.Context {
	return businessctx.WithOutgoingScope(botRPCContext(parent, botID), scope)
}

func businessBotMediaCallContext(parent context.Context, botID int64, scope businessctx.Scope) context.Context {
	return businessctx.WithOutgoingScope(botMediaRPCContext(parent, botID), scope)
}

func resolveBotAPISendScope(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (businessctx.Scope, bool, error) {
	connectionID := strings.TrimSpace(request.string("business_connection_id"))
	if connectionID == "" {
		return businessctx.Scope{}, false, nil
	}
	if len(connectionID) > 128 {
		return businessctx.Scope{}, false, mtproto.ErrInputRequestInvalid
	}
	scope, err := resolveBotAPIBusinessScope(ctx, requestCtx, botID, connectionID)
	if err != nil {
		return businessctx.Scope{}, false, err
	}
	if !scope.CanReply {
		return businessctx.Scope{}, false, status.Error(mtproto.ErrBadRequest, "BUSINESS_CONNECTION_CAN_REPLY_REQUIRED")
	}
	return scope, true, nil
}

func readBusinessMessage(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	connectionID := strings.TrimSpace(request.string("business_connection_id"))
	if connectionID == "" || len(connectionID) > 128 {
		return nil, http.StatusBadRequest, 400, "Bad Request: business_connection_id is invalid"
	}
	scope, err := resolveBotAPIBusinessScope(ctx, r.Context(), botID, connectionID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !scope.CanReadMessages {
		return nil, http.StatusBadRequest, 400, "Bad Request: business connection does not allow reading messages"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, scope.OwnerID, request.string("chat_id"))
	if err != nil || peerID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int64("message_id")
	if err != nil || messageID <= 0 || messageID > int64(^uint32(0)>>1) {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	_, err = ctx.MessagesClient.MessagesReadHistory(businessBotCallContext(r.Context(), botID, scope), &mtproto.TLMessagesReadHistory{
		Peer:  botAPIInputPeer(scope.OwnerID, peerType, peerID),
		MaxId: int32(messageID),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteBusinessMessages(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	connectionID := strings.TrimSpace(request.string("business_connection_id"))
	if connectionID == "" || len(connectionID) > 128 {
		return nil, http.StatusBadRequest, 400, "Bad Request: business_connection_id is invalid"
	}
	scope, err := resolveBotAPIBusinessScope(ctx, r.Context(), botID, connectionID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	var rawIDs []int
	if err = request.json("message_ids", &rawIDs); err != nil || len(rawIDs) == 0 || len(rawIDs) > 100 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_ids is invalid"
	}
	ids := make([]int32, 0, len(rawIDs))
	seen := make(map[int32]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		if rawID <= 0 || int64(rawID) > int64(^uint32(0)>>1) {
			return nil, http.StatusBadRequest, 400, "Bad Request: message_ids is invalid"
		}
		id := int32(rawID)
		if _, exists := seen[id]; exists {
			return nil, http.StatusBadRequest, 400, "Bad Request: message_ids contains duplicates"
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	if _, err = ctx.MessagesClient.MessagesDeleteMessages(businessBotCallContext(r.Context(), botID, scope), &mtproto.TLMessagesDeleteMessages{
		Revoke: true,
		Id:     ids,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}
