// Copyright 2026 Teamgram Authors
// All rights reserved.

package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (c *MessagesCore) businessScope() (businessctx.Scope, bool) {
	if c == nil {
		return businessctx.Scope{}, false
	}
	scope, ok := businessctx.ScopeFromContext(c.ctx)
	return scope, ok
}

func (c *MessagesCore) actingUserID() int64 {
	if scope, ok := c.businessScope(); ok {
		return scope.OwnerID
	}
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.UserId
}

// sendIdentity applies the business connection sender semantics to message
// creation. The bot remains the authenticated caller while the connected
// business user owns the resulting message.
func (c *MessagesCore) sendIdentity() (int64, *wrapperspb.Int64Value, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return 0, nil, mtproto.ErrAuthKeyUnregistered
	}
	if scope, ok := c.businessScope(); ok {
		if err := c.requireBusinessRight(scope.CanReply); err != nil {
			return 0, nil, err
		}
		return scope.OwnerID, wrapperspb.Int64(c.MD.UserId), nil
	}
	return c.MD.UserId, nil, nil
}

func (c *MessagesCore) validateBusinessReplyPeer(peer *mtproto.PeerUtil) error {
	scope, business := c.businessScope()
	if !business {
		return nil
	}
	if peer == nil || !peer.IsUser() || peer.IsSelfUser(scope.OwnerID) {
		return mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return mtproto.ErrInternalServerError
	}
	history, err := c.svcCtx.Dao.MessageClient.MessageGetHistoryMessages(c.ctx, &message.TLMessageGetHistoryMessages{
		UserId:   scope.OwnerID,
		PeerType: mtproto.PEER_USER,
		PeerId:   peer.PeerId,
		Limit:    100,
	})
	if err != nil {
		return err
	}
	now := int32(time.Now().Unix())
	for _, box := range history.GetDatas() {
		if box == nil || box.GetSenderUserId() != peer.PeerId || box.GetMessage() == nil {
			continue
		}
		date := box.GetMessage().GetDate()
		if date > 0 && date <= now && now-date <= 24*60*60 {
			return nil
		}
	}
	return mtproto.ErrChatWriteForbidden
}

func (c *MessagesCore) requireBusinessRight(allowed bool) error {
	if !allowed {
		return mtproto.ErrChatAdminRequired
	}
	return nil
}
