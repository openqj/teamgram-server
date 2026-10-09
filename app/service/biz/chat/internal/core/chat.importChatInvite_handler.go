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

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatImportChatInvite
// chat.importChatInvite self_id:long hash:string = MutableChat;
func (c *ChatCore) ChatImportChatInvite(in *chat.TLChatImportChatInvite) (*mtproto.MutableChat, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Invites == nil || c.svcCtx.Dao.Postgres.Store.InviteParticipants == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if _, err := c.requireInviteSelf(in.SelfId); err != nil {
		return nil, err
	}
	var err error
	var chatInviteDO *dataobject.ChatInvitesDO
	chatInviteDO, err = c.svcCtx.Dao.Postgres.Store.Invites.SelectByLink(c.ctx, in.Hash)
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
	inviteParticipant := &dataobject.ChatInviteParticipantsDO{
		ChatId: chatInviteDO.ChatId,
		Link:   in.Hash,
		UserId: in.SelfId,
		Date2:  time.Now().Unix(),
	}
	chat2, err := c.chatAddChatUser(&chat.TLChatAddChatUser{
		ChatId:    chatInviteDO.ChatId,
		InviterId: chatInviteDO.AdminId,
		UserId:    in.SelfId,
	}, func(tx pgx.Tx) error {
		lockedInvite, lockErr := c.svcCtx.Dao.Postgres.Store.Invites.SelectByLinkOn(c.ctx, tx, in.Hash)
		if lockErr != nil {
			return lockErr
		}
		if lockedInvite == nil {
			return mtproto.ErrInviteHashInvalid
		}
		if lockedInvite.Revoked || (lockedInvite.ExpireDate != 0 && time.Now().Unix() > lockedInvite.ExpireDate) {
			return mtproto.ErrInviteHashExpired
		}
		if lockedInvite.UsageLimit > 0 {
			count, countErr := c.svcCtx.Dao.Postgres.Store.InviteParticipants.CountByLinkOn(c.ctx, tx, lockedInvite.Link, false)
			if countErr != nil {
				return countErr
			}
			if count >= int64(lockedInvite.UsageLimit) {
				return mtproto.ErrInviteHashExpired
			}
		}
		_, _, insertErr := c.svcCtx.Dao.Postgres.Store.InviteParticipants.InsertOn(c.ctx, tx, inviteParticipant)
		return insertErr
	})
	if err != nil {
		c.Logger.Errorf("chat.importChatInvite - error: %v", err)
		return nil, err
	}

	return chat2, nil
}
