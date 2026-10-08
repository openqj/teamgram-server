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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatImportChatInvite
// chat.importChatInvite self_id:long hash:string = MutableChat;
func (c *ChatCore) ChatImportChatInvite(in *chat.TLChatImportChatInvite) (*mtproto.MutableChat, error) {
	if _, err := c.requireInviteSelf(in.SelfId); err != nil {
		return nil, err
	}
	var err error
	var chatInviteDO *dataobject.ChatInvitesDO
	if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
		chatInviteDO, err = c.svcCtx.Dao.Postgres.Store.Invites.SelectByLink(c.ctx, in.Hash)
	} else {
		chatInviteDO, err = c.svcCtx.Dao.ChatInvitesDAO.SelectByLink(c.ctx, in.Hash)
	}
	if err != nil {
		c.Logger.Errorf("chat.importChatInvite - error: %v", err)
		return nil, err
	} else if chatInviteDO == nil {
		err = mtproto.ErrInviteHashInvalid
		c.Logger.Errorf("chat.importChatInvite - error: %v", err)
		return nil, err
	}
	if chatInviteDO.Revoked {
		err = mtproto.ErrInviteHashExpired
		c.Logger.Errorf("chat.importChatInvite - error: %v", err)
		return nil, err
	}

	if chatInviteDO.ExpireDate != 0 && time.Now().Unix() > chatInviteDO.ExpireDate {
		err = mtproto.ErrInviteHashExpired
		c.Logger.Errorf("chat.importChatInvite - error: %v", err)
		return nil, err
	}
	if chatInviteDO.UsageLimit > 0 {
		var sz int
		if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
			count, countErr := c.svcCtx.Dao.Postgres.Store.InviteParticipants.CountByLink(c.ctx, chatInviteDO.Link, false)
			if countErr != nil {
				return nil, countErr
			}
			sz = int(count)
		} else {
			sz = c.svcCtx.Dao.CommonDAO.CalcSize(c.ctx, "chat_invite_participants", map[string]interface{}{"link": chatInviteDO.Link})
		}

		if sz >= int(chatInviteDO.UsageLimit) {
			err = mtproto.ErrInviteHashExpired
			c.Logger.Errorf("chat.importChatInvite - error: %v", err)
			return nil, err
		}
	}

	chat2, err := c.ChatAddChatUser(&chat.TLChatAddChatUser{
		ChatId:    chatInviteDO.ChatId,
		InviterId: chatInviteDO.AdminId,
		UserId:    in.SelfId,
	})
	if err != nil {
		c.Logger.Errorf("chat.importChatInvite - error: %v", err)
		return nil, err
	}

	inviteParticipant := &dataobject.ChatInviteParticipantsDO{
		ChatId: chatInviteDO.ChatId,
		Link:   in.Hash,
		UserId: in.SelfId,
		Date2:  time.Now().Unix(),
	}
	if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
		tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
		if txErr == nil {
			defer tx.Rollback(c.ctx)
			_, _, txErr = c.svcCtx.Dao.Postgres.Store.InviteParticipants.InsertOn(c.ctx, tx, inviteParticipant)
			if txErr == nil {
				txErr = tx.Commit(c.ctx)
			}
		}
		err = txErr
	} else {
		_, _, err = c.svcCtx.Dao.ChatInviteParticipantsDAO.Insert(c.ctx, inviteParticipant)
	}
	if err != nil {
		c.Logger.Errorf("chat.importChatInvite - error: %v", err)
		return nil, err
	}

	return chat2, nil
}
