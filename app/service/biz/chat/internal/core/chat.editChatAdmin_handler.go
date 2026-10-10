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
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	// The public messages.editChatAdmin request always supplies is_admin.  The
	// chats BFF uses an omitted field for the separate creator-transfer flow;
	// keeping that distinction inside the authoritative chat service avoids a
	// second writer for creator_user_id.
	if in.IsAdmin == nil {
		return c.transferChatCreator(in)
	}
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

func (c *ChatCore) transferChatCreator(in *chat.TLChatEditChatAdmin) (*mtproto.MutableChat, error) {
	if in == nil || in.ChatId <= 0 || in.OperatorId <= 0 || in.EditChatAdminId <= 0 || in.OperatorId == in.EditChatAdminId {
		return nil, mtproto.ErrPeerIdInvalid
	}
	// Return the complete roster because the public creator-transfer method
	// emits a participant update to every active member.
	chat2, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId)
	if err != nil || chat2 == nil {
		return nil, mtproto.ErrChatIdInvalid
	}
	me, ok := chat2.GetImmutableChatParticipant(in.OperatorId)
	if !ok || me == nil || !me.IsChatMemberStateNormal() {
		return nil, mtproto.ErrUserNotParticipant
	}
	if !me.IsChatMemberCreator() || chat2.Creator() != in.OperatorId {
		return nil, mtproto.ErrChatAdminRequired
	}
	target, ok := chat2.GetImmutableChatParticipant(in.EditChatAdminId)
	if !ok || target == nil || !target.IsChatMemberStateNormal() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if target.IsChatMemberCreator() || chat2.Creator() == in.EditChatAdminId {
		return nil, mtproto.ErrChatNotModified
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.transferChatCreator: PostgreSQL store is not initialized")
	}
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(c.ctx)
	var storedCreator int64
	if err = tx.QueryRow(c.ctx, `SELECT creator_user_id FROM chats WHERE id = $1 FOR UPDATE`, in.ChatId).Scan(&storedCreator); err != nil {
		return nil, err
	}
	if storedCreator != in.OperatorId {
		return nil, mtproto.ErrChatAdminRequired
	}
	rows, err := tx.Query(c.ctx, `SELECT user_id, participant_type, state FROM chat_participants WHERE chat_id = $1 AND user_id = ANY($2::bigint[]) ORDER BY user_id FOR UPDATE`, in.ChatId, []int64{in.OperatorId, in.EditChatAdminId})
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{}, 2)
	for rows.Next() {
		var userID int64
		var participantType, state int32
		if err = rows.Scan(&userID, &participantType, &state); err != nil {
			rows.Close()
			return nil, err
		}
		seen[userID] = struct{}{}
		if state != mtproto.ChatMemberStateNormal || (userID == in.OperatorId && participantType != mtproto.ChatMemberCreator) {
			rows.Close()
			return nil, mtproto.ErrPeerIdInvalid
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(seen) != 2 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if _, err = c.svcCtx.Dao.Postgres.Store.Participants.UpdateParticipantTypeAndRightsOn(c.ctx, tx, mtproto.ChatMemberNormal, 0, me.Id); err != nil {
		return nil, err
	}
	if _, err = c.svcCtx.Dao.Postgres.Store.Participants.UpdateParticipantTypeAndRightsOn(c.ctx, tx, mtproto.ChatMemberCreator, 0, target.Id); err != nil {
		return nil, err
	}
	result, err := tx.Exec(c.ctx, `UPDATE chats SET creator_user_id = $1, version = version + 1 WHERE id = $2 AND creator_user_id = $3`, in.EditChatAdminId, in.ChatId, in.OperatorId)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() != 1 {
		return nil, mtproto.ErrChatAdminRequired
	}
	if err = tx.Commit(c.ctx); err != nil {
		return nil, err
	}
	chat2.Chat.Creator = in.EditChatAdminId
	chat2.Chat.Version++
	chat2.Chat.Date = time.Now().Unix()
	me.ParticipantType = mtproto.ChatMemberNormal
	me.AdminRights = nil
	target.ParticipantType = mtproto.ChatMemberCreator
	target.AdminRights = nil
	return chat2, nil
}
