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
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatGetMutableChatByLink
// chat.getMutableChatByLink link:string = MutableChat;
func (c *ChatCore) ChatGetMutableChatByLink(in *chat.TLChatGetMutableChatByLink) (*mtproto.MutableChat, error) {
	if in == nil || strings.TrimSpace(in.GetLink()) == "" {
		return nil, mtproto.ErrInviteHashInvalid
	}

	link := chat.GetInviteHashByLink(strings.TrimSpace(in.GetLink()))
	var invite *dataobject.ChatInvitesDO
	var err error
	if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
		invite, err = c.svcCtx.Dao.Postgres.Store.Invites.SelectByLink(c.ctx, link)
	} else {
		invite, err = c.svcCtx.Dao.ChatInvitesDAO.SelectByLink(c.ctx, link)
	}
	if err != nil {
		c.Logger.Errorf("chat.getMutableChatByLink - error: %v", err)
		return nil, err
	}
	if invite == nil || invite.ChatId <= 0 {
		return nil, mtproto.ErrInviteHashInvalid
	}
	if invite.Revoked || (invite.ExpireDate != 0 && invite.ExpireDate <= time.Now().Unix()) {
		return nil, mtproto.ErrInviteHashExpired
	}

	mChat, err := c.svcCtx.Dao.GetMutableChat(c.ctx, invite.ChatId)
	if err != nil {
		c.Logger.Errorf("chat.getMutableChatByLink - error: %v", err)
		return nil, err
	}
	if mChat == nil {
		return nil, mtproto.ErrChatIdInvalid
	}
	return mChat, nil
}
