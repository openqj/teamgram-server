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

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatAddChatUser
// chat.addChatUser chat_id:long inviter_id:long user_id:long = MutableChat;
func (c *ChatCore) ChatAddChatUser(in *chat.TLChatAddChatUser) (*mtproto.MutableChat, error) {
	return c.chatAddChatUser(in, nil)
}

func (c *ChatCore) chatAddChatUser(in *chat.TLChatAddChatUser, updateInvite func(pgx.Tx) error) (*mtproto.MutableChat, error) {
	var (
		now                       = time.Now().Unix()
		chat2                     *mtproto.MutableChat
		me, willAdd               *mtproto.ImmutableChatParticipant
		err                       error
		chatId, inviterId, userId = in.ChatId, in.InviterId, in.UserId
	)

	if inviterId != 0 {
		chat2, err = c.svcCtx.Dao.GetMutableChat(c.ctx, chatId, inviterId, userId)
		if err != nil {
			c.Logger.Errorf("chat.addChatUser - error: %v", err)
			return nil, err
		} else {
			if chat2.Deactivated() && chat2.GetChat().GetMigratedTo() != nil {
				err = mtproto.ErrMigratedToChannel
				c.Logger.Errorf("chat.addChatUser - error: %v", err)
				return nil, err
			}
		}

		me, _ = chat2.GetImmutableChatParticipant(inviterId)
		if me == nil || (me.State != mtproto.ChatMemberStateNormal && !me.IsChatMemberCreator()) {
			err = mtproto.ErrInputUserDeactivated
			c.Logger.Errorf("chat.addChatUser - error: %v", err)
			return nil, err
		}
	} else {
		chat2, err = c.svcCtx.Dao.GetMutableChat(c.ctx, chatId, userId)
		if err != nil {
			c.Logger.Errorf("chat.addChatUser - error: %v", err)
			return nil, err
		}
		inviterId = chat2.Creator()
	}

	if chat2.ParticipantsCount() >= 200 {
		err = mtproto.ErrUsersTooFew
		c.Logger.Errorf("chat.addChatUser - error: %v", err)
		return nil, err
	}

	willAdd, _ = chat2.GetImmutableChatParticipant(userId)
	if willAdd != nil && willAdd.State == mtproto.ChatMemberStateNormal {
		err = mtproto.ErrUserAlreadyParticipant
		c.Logger.Errorf("chat.addChatUser - error: %v", err)
		return nil, err
	}

	if me != nil {
		// TODO(@benqi): check
		// 400	CHAT_ADMIN_REQUIRED	You must be an admin in this chat to do this
		if !me.CanInviteUsers() &&
			!chat2.DefaultBannedRights().CanInviteUsers(int32(time.Now().Unix())) {
			err = mtproto.ErrChatAdminRequired
			c.Logger.Errorf("chat.addChatUser - error: %v", err)
			return nil, err
		}
	}

	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.addChatUser: PostgreSQL store is not initialized")
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(c.ctx)
	participantDO := &dataobject.ChatParticipantsDO{
		ChatId: chat2.Chat.Id, UserId: userId, ParticipantType: mtproto.ChatMemberNormal,
		InviterUserId: inviterId, InvitedAt: now, Date2: now, IsBot: in.GetIsBot(),
	}
	if chat2.Chat.Creator == userId {
		participantDO.ParticipantType = mtproto.ChatMemberCreator
	}
	if willAdd == nil {
		participantDO.Id, _, err = c.svcCtx.Dao.Postgres.Store.Participants.InsertOn(c.ctx, tx, participantDO)
		if err == nil {
			willAdd = c.svcCtx.Dao.MakeImmutableChatParticipant(participantDO)
		}
	} else {
		participantDO.Id = willAdd.Id
		_, err = c.svcCtx.Dao.Postgres.Store.Participants.UpdateOn(c.ctx, tx, participantDO.ParticipantType,
			inviterId, now, in.GetIsBot(), participantDO.Id)
	}
	if err == nil {
		chat2.Chat.ParticipantsCount++
		chat2.Chat.Version++
		chat2.Chat.Date = now
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdateParticipantCountOn(c.ctx, tx, chat2.Chat.ParticipantsCount, chatId)
	}
	if err == nil && updateInvite != nil {
		err = updateInvite(tx)
	}
	if err == nil {
		err = tx.Commit(c.ctx)
	}
	if err != nil {
		c.Logger.Errorf("chat.addChatUser - error: %v", err)
		return nil, err
	}
	return chat2, nil
}
