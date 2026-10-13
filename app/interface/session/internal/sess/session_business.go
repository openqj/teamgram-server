// Copyright 2026 Teamgram Authors
// All rights reserved.

package sess

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto"
	rpcmetadata "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
	"google.golang.org/grpc/status"
)

func (c *session) resolveBusinessConnection(ctx context.Context, clientIP, connectionID string) (businessctx.Scope, error) {
	if c == nil || c.sessList == nil || c.sessList.cb == nil || c.sessList.cb.cb == nil || c.sessList.cb.cb.Dao == nil || c.sessList.cb.AuthUserId <= 0 {
		return businessctx.Scope{}, status.Error(mtproto.ErrBadRequest, "CONNECTION_ID_INVALID")
	}
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return businessctx.Scope{}, status.Error(mtproto.ErrBadRequest, "CONNECTION_ID_INVALID")
	}
	wrapper := c.sessList.cb
	metadata := &rpcmetadata.RpcMetadata{
		ServerId:      wrapper.cb.Dao.MyServerId,
		ClientAddr:    clientIP,
		AuthId:        wrapper.authKeyId,
		SessionId:     c.sessionId,
		ReceiveTime:   0,
		UserId:        wrapper.AuthUserId,
		IsBot:         true,
		Layer:         wrapper.Layer(),
		Client:        wrapper.ClientName(),
		PermAuthKeyId: wrapper.authKeyId,
	}
	result, err := wrapper.cb.Dao.InvokeContext(ctx, metadata, &mtproto.TLAccountGetBotBusinessConnection{ConnectionId: connectionID})
	if err != nil {
		return businessctx.Scope{}, err
	}
	updates, ok := result.(*mtproto.Updates)
	if !ok || updates == nil {
		return businessctx.Scope{}, status.Error(mtproto.ErrBadRequest, "CONNECTION_ID_INVALID")
	}

	var connection *mtproto.BotBusinessConnection
	visit := func(update *mtproto.Update) {
		if update != nil && update.GetPredicateName() == mtproto.Predicate_updateBotBusinessConnect && update.GetConnection() != nil {
			candidate := update.GetConnection()
			if candidate.GetConnectionId() == connectionID {
				connection = candidate
			}
		}
	}
	visit(updates.GetUpdate())
	for _, update := range updates.GetUpdates() {
		visit(update)
	}
	if connection == nil || connection.GetUserId() <= 0 || connection.GetDisabled() {
		return businessctx.Scope{}, status.Error(mtproto.ErrBadRequest, "CONNECTION_ID_INVALID")
	}
	rights := connection.GetRights()
	return businessctx.Scope{
		ConnectionID:    connectionID,
		OwnerID:         connection.GetUserId(),
		CanReply:        connection.GetCanReply() || (rights != nil && rights.GetReply()),
		CanReadMessages: rights != nil && rights.GetReadMessages(),
		CanDeleteSent:   rights != nil && rights.GetDeleteSentMessages(),
		CanDeleteAll:    rights != nil && rights.GetDeleteReceivedMessages(),
	}, nil
}
