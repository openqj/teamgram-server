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
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatEditChatAdmin
// chat.editChatAdmin chat_id:long operator_id:long edit_chat_admin_id:long is_admin:Bool = MutableChat;
func (c *ChatCore) ChatEditChatAdmin(in *chat.TLChatEditChatAdmin) (*mtproto.MutableChat, error) {
	var (
		now           = time.Now().Unix()
		chat2         *mtproto.MutableChat
		me, editAdmin *mtproto.ImmutableChatParticipant
		err           error
	)

	chat2, err = c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId, in.OperatorId, in.EditChatAdminId)
	if err != nil {
		c.Logger.Errorf("chat.editChatAdmin - error: %v", err)
		err = mtproto.ErrChatIdInvalid
		return nil, err
	}

	me, _ = chat2.GetImmutableChatParticipant(in.OperatorId)
	if me == nil || me.State != mtproto.ChatMemberStateNormal {
		err = mtproto.ErrUserNotParticipant
		c.Logger.Errorf("chat.editChatAdmin - error: %v", err)
		return nil, err
	}

	editAdmin, _ = chat2.GetImmutableChatParticipant(in.EditChatAdminId)
	if editAdmin != nil && editAdmin.State != mtproto.ChatMemberStateNormal {
		err = mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("chat.editChatAdmin - error: %v", err)
		return nil, err
	}

	if !me.CanAdminAddAdmins() {
		err = mtproto.ErrChatAdminRequired
		c.Logger.Errorf("chat.editChatAdmin - error: %v", err)
		return nil, err
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.editChatAdmin: PostgreSQL store is not initialized")
	}
	tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if txErr == nil {
		defer tx.Rollback(c.ctx)
		if mtproto.FromBool(in.IsAdmin) {
			_, txErr = c.svcCtx.Dao.Postgres.Store.Participants.UpdateParticipantTypeOn(c.ctx, tx, mtproto.ChatMemberAdmin, editAdmin.Id)
			if txErr == nil && editAdmin.Link == "" {
				editAdmin.Link = chat.GenChatInviteHash()
				_, txErr = c.svcCtx.Dao.Postgres.Store.Participants.UpdateLinkOn(c.ctx, tx, editAdmin.Link, in.ChatId, in.EditChatAdminId)
				if txErr == nil {
					_, _, txErr = c.svcCtx.Dao.Postgres.Store.Invites.InsertOn(c.ctx, tx, &dataobject.ChatInvitesDO{ChatId: in.ChatId, AdminId: in.EditChatAdminId, Link: editAdmin.Link, Permanent: true, Date2: now})
				}
			}
			editAdmin.AdminRights = mtproto.MakeDefaultChatAdminRights()
			editAdmin.ParticipantType = mtproto.ChatMemberAdmin
		} else {
			_, txErr = c.svcCtx.Dao.Postgres.Store.Participants.UpdateParticipantTypeOn(c.ctx, tx, mtproto.ChatMemberNormal, editAdmin.Id)
			if txErr == nil {
				_, txErr = c.svcCtx.Dao.Postgres.Store.Participants.UpdateLinkOn(c.ctx, tx, "", in.ChatId, in.EditChatAdminId)
			}
			editAdmin.AdminRights = nil
			editAdmin.ParticipantType = mtproto.ChatMemberNormal
			editAdmin.Link = ""
		}
		if txErr == nil {
			_, txErr = c.svcCtx.Dao.Postgres.Store.Chats.UpdateVersionOn(c.ctx, tx, in.ChatId)
		}
		if txErr == nil {
			txErr = tx.Commit(c.ctx)
		}
	}
	if txErr != nil {
		return nil, txErr
	}
	chat2.Chat.Version++
	chat2.Chat.Date = now
	return chat2, nil
}
