/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatDeleteRevokedExportedChatInvites
// chat.deleteRevokedExportedChatInvites self_id:long chat_id:long admin_id:long = Bool;
func (c *ChatCore) ChatDeleteRevokedExportedChatInvites(in *chat.TLChatDeleteRevokedExportedChatInvites) (*mtproto.Bool, error) {
	selfID, err := c.requireInviteSelf(in.SelfId)
	if err != nil {
		return nil, err
	}
	if _, err = c.requireInvitePermission(in.ChatId, selfID, in.AdminId); err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Invites == nil {
		return nil, mtproto.ErrInternalServerError
	}
	tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if txErr == nil {
		defer tx.Rollback(c.ctx)
		_, txErr = c.svcCtx.Dao.Postgres.Store.Invites.DeleteByRevokedOn(c.ctx, tx, in.ChatId, in.AdminId)
		if txErr == nil {
			txErr = tx.Commit(c.ctx)
		}
	}
	err = txErr
	if err != nil {
		c.Logger.Errorf("chat.deleteRevokedExportedChatInvites - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
