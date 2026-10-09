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

// ChatDeleteExportedChatInvite
// chat.deleteExportedChatInvite self_id:long chat_id:long link:string = Bool;
func (c *ChatCore) ChatDeleteExportedChatInvite(in *chat.TLChatDeleteExportedChatInvite) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Invites == nil {
		return nil, mtproto.ErrInternalServerError
	}
	selfID, err := c.requireInviteSelf(in.SelfId)
	if err != nil {
		return nil, err
	}
	link := chat.GetInviteHashByLink(in.Link)
	if _, err = c.requireInviteLinkPermission(in.ChatId, selfID, in.Link); err != nil {
		return nil, err
	}

	var rows int64
	tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if txErr != nil {
		return nil, txErr
	}
	defer func() { _ = tx.Rollback(c.ctx) }()
	rows, txErr = c.svcCtx.Dao.Postgres.Store.Invites.DeleteByLinkOn(c.ctx, tx, in.ChatId, link)
	if txErr == nil {
		txErr = tx.Commit(c.ctx)
	}
	err = txErr
	if err != nil {
		c.Logger.Errorf("chat.deleteExportedChatInvite - error: %v", err)
		return nil, err
	}
	if rows == 0 {
		return nil, mtproto.ErrInviteHashInvalid
	}
	return mtproto.BoolTrue, nil
}
