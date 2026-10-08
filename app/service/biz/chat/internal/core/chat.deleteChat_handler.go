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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatDeleteChat
// chat.deleteChat chat_id:long operator_id:long = MutableChat;
func (c *ChatCore) ChatDeleteChat(in *chat.TLChatDeleteChat) (*mtproto.MutableChat, error) {
	mChat, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId)
	if err != nil {
		c.Logger.Errorf("chat.deleteChat - error: %v", err)
		return nil, err
	}

	if in.OperatorId == 0 {
		in.OperatorId = mChat.Creator()
	}

	if mChat.Creator() != in.OperatorId {
		err = mtproto.ErrChatAdminRequired
		c.Logger.Errorf("chat.deleteChat - error: %v", err)
		return nil, err
	}

	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.deleteChat: PostgreSQL store is not initialized")
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(c.ctx)
	_, err = c.svcCtx.Dao.Postgres.Store.Participants.UpdateStateByChatIdOn(c.ctx, tx, mtproto.ChatMemberStateKicked, in.ChatId)
	if err == nil {
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdateParticipantCountOn(c.ctx, tx, 0, in.ChatId)
	}
	if err == nil {
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdateDeactivatedOn(c.ctx, tx, true, in.ChatId)
	}
	if err == nil {
		err = tx.Commit(c.ctx)
	}
	if err != nil {
		c.Logger.Errorf("chat.deleteChat - error: %v", err)
		return nil, err
	}
	return mChat, nil
}
