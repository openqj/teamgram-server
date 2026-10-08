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
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatEditChatTitle
// chat.editChatTitle chat_id:long edit_user_id:long title:string = MutableChat;
func (c *ChatCore) ChatEditChatTitle(in *chat.TLChatEditChatTitle) (*mtproto.MutableChat, error) {
	var (
		now                = time.Now().Unix()
		chat2              *mtproto.MutableChat
		me                 *mtproto.ImmutableChatParticipant
		err                error
		chatId, editUserId = in.ChatId, in.EditUserId
	)

	if in.Title == "" {
		err = mtproto.ErrChatTitleEmpty
		c.Logger.Errorf("chat.editChatTitle - error: %v", err)
		return nil, err
	}

	chat2, err = c.svcCtx.Dao.GetMutableChat(c.ctx, chatId, editUserId)
	if err != nil {
		c.Logger.Errorf("chat.editChatTitle - error: %v", err)
		return nil, err
	}
	if chat2.Chat.Title == in.Title {
		err = mtproto.ErrChatNotModified
		c.Logger.Errorf("chat.editChatTitle - error: %v", err)
		return nil, err
	}

	me, _ = chat2.GetImmutableChatParticipant(editUserId)
	if me == nil || me.State != mtproto.ChatMemberStateNormal {
		err = mtproto.ErrInputUserDeactivated
		c.Logger.Errorf("chat.editChatTitle - error: %v", err)
		return nil, err
	}

	// TODO(@benqi): check
	// 400	CHAT_ADMIN_REQUIRED	You must be an admin in this chat to do this
	if !me.CanChangeInfo() {
		err = mtproto.ErrChatAdminRequired
		c.Logger.Errorf("chat.editChatTitle - error: %v", err)
		return nil, err
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.editChatTitle: PostgreSQL store is not initialized")
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer tx.Rollback(c.ctx)
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdateTitleOn(c.ctx, tx, in.Title, chatId)
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		c.Logger.Errorf("chat.editChatTitle - error: %v", err)
		return nil, err
	}

	chat2.Chat.Title = in.Title
	chat2.Chat.Version += 1
	chat2.Chat.Date = now
	return chat2, nil
}
