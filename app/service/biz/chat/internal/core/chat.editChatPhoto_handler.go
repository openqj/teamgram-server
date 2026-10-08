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

// ChatEditChatPhoto
// chat.editChatPhoto chat_id:long edit_user_id:long chat_photo:Photo = MutableChat;
func (c *ChatCore) ChatEditChatPhoto(in *chat.TLChatEditChatPhoto) (*mtproto.MutableChat, error) {
	var (
		err        error
		now        = time.Now().Unix()
		chatId     = in.ChatId
		editUserId = in.EditUserId
		chat2      *mtproto.MutableChat
		me         *mtproto.ImmutableChatParticipant
	)

	chat2, err = c.svcCtx.Dao.GetMutableChat(c.ctx, chatId, editUserId)
	if err != nil {
		return nil, err
	}

	me, _ = chat2.GetImmutableChatParticipant(editUserId)
	if me == nil || me.State != mtproto.ChatMemberStateNormal {
		err = mtproto.ErrInputUserDeactivated
		return nil, err
	}

	// TODO(@benqi): check
	// 400	CHAT_ADMIN_REQUIRED	You must be an admin in this chat to do this
	if !me.CanChangeInfo() {
		err = mtproto.ErrChatAdminRequired
		return nil, err
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.editChatPhoto: PostgreSQL store is not initialized")
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer tx.Rollback(c.ctx)
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdatePhotoIDOn(c.ctx, tx, in.GetChatPhoto().GetId(), chatId)
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		c.Logger.Errorf("chat.editChatPhoto - error: %v", err)
		return nil, err
	}

	chat2.Chat.Version += 1
	chat2.Chat.Date = now
	return chat2, nil
}
