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

// ChatMigratedToChannel
// chat.migratedToChannel chat:MutableChat id:long access_hash:long = Bool;
func (c *ChatCore) ChatMigratedToChannel(in *chat.TLChatMigratedToChannel) (*mtproto.Bool, error) {
	tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if txErr == nil {
		defer tx.Rollback(c.ctx)
		_, txErr = c.svcCtx.Dao.Postgres.Store.Chats.UpdateMigratedToOn(c.ctx, tx, in.Id, in.AccessHash, in.Chat.Id())
		if txErr == nil {
			_, txErr = c.svcCtx.Dao.Postgres.Store.Participants.UpdateStateByChatIdOn(c.ctx, tx, mtproto.ChatMemberStateMigrated, in.Chat.Id())
		}
		if txErr == nil {
			txErr = tx.Commit(c.ctx)
		}
	}
	if txErr != nil {
		c.Logger.Errorf("chat.migratedToChannel - error: %v", txErr)
		return nil, txErr
	}

	return mtproto.BoolTrue, nil
}
