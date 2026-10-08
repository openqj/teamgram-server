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

// ChatDeleteChatUser
// chat.deleteChatUser chat_id:long operator_id:long delete_user_id:long = MutableChat;
func (c *ChatCore) ChatDeleteChatUser(in *chat.TLChatDeleteChatUser) (*mtproto.MutableChat, error) {
	var (
		now             = time.Now().Unix()
		chat2           *mtproto.MutableChat
		me, deletedUser *mtproto.ImmutableChatParticipant
		err             error
		chatId          = in.ChatId
		operatorId      = in.OperatorId
		deleteUserId    = in.DeleteUserId
		kicked          = operatorId != deleteUserId
	)

	chat2, err = c.svcCtx.Dao.GetMutableChat(c.ctx, chatId)
	if err != nil {
		c.Logger.Errorf("chat.deleteChatUser - error: %v", err)
		return nil, err
	}

	if operatorId == 0 {
		operatorId = chat2.Creator()
	}

	me, _ = chat2.GetImmutableChatParticipant(operatorId)
	if me == nil {
		err = mtproto.ErrInputUserDeactivated
		c.Logger.Errorf("chat.deleteChatUser - error: %v", err)
		return nil, err
	}

	if kicked {
		if me.State != mtproto.ChatMemberStateNormal {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("chat.deleteChatUser - error: %v", err)
			return nil, err
		}

		if !me.CanAdminBanUsers() {
			err = mtproto.ErrChatAdminRequired
			c.Logger.Errorf("chat.deleteChatUser - error: %v", err)
			return nil, err
		}
		//switch me.ChatParticipant.PredicateName {
		//case mtproto.Predicate_chatParticipantCreator:
		//default:
		//	err = mtproto.ErrChatAdminRequired
		//	return nil, err
		//}

		deletedUser, _ = chat2.GetImmutableChatParticipant(deleteUserId)
		if deletedUser == nil {
			// USER_NOT_PARTICIPANT
			err = mtproto.ErrUserNotParticipant
			c.Logger.Errorf("chat.deleteChatUser - error: %v", err)
			return nil, err
		} else if deletedUser.State != mtproto.ChatMemberStateNormal {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("chat.deleteChatUser - error: %v", err)
			return nil, err
		}
	} else {
		// left
		deletedUser = me
		if me.State != mtproto.ChatMemberStateNormal {
			err = mtproto.ErrPeerIdInvalid
			return nil, err
		}
	}

	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.deleteChatUser: PostgreSQL store is not initialized")
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(c.ctx)
	if kicked {
		_, err = c.svcCtx.Dao.Postgres.Store.Participants.UpdateKickedOn(c.ctx, tx, now, deletedUser.Id)
		deletedUser.State = mtproto.ChatMemberStateKicked
	} else {
		_, err = c.svcCtx.Dao.Postgres.Store.Participants.UpdateLeftOn(c.ctx, tx, now, deletedUser.Id)
		deletedUser.State = mtproto.ChatMemberStateLeft
	}
	if err == nil {
		chat2.Chat.ParticipantsCount--
		chat2.Chat.Date = now
		chat2.Chat.Version++
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdateParticipantCountOn(c.ctx, tx, chat2.Chat.ParticipantsCount, chat2.Chat.Id)
	}
	if err == nil {
		_, err = c.svcCtx.Dao.Postgres.Store.InviteParticipants.DeleteOn(c.ctx, tx, chat2.Chat.Id, deleteUserId)
	}
	if err == nil {
		err = tx.Commit(c.ctx)
	}
	if err != nil {
		c.Logger.Errorf("chat.deleteChatUser - error: %v", err)
		return nil, err
	}
	return chat2, nil
}
