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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatGetExportedChatInvite
// chat.getExportedChatInvite chat_id:long link:string = ExportedChatInvite;
func (c *ChatCore) ChatGetExportedChatInvite(in *chat.TLChatGetExportedChatInvite) (*mtproto.ExportedChatInvite, error) {
	callerID, err := c.requireInviteCaller()
	if err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Invites == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		link = chat.GetInviteHashByLink(in.Link)
	)

	var chatInviteDO *dataobject.ChatInvitesDO
	chatInviteDO, err = c.svcCtx.Dao.Postgres.Store.Invites.SelectByLink(c.ctx, link)
	if err != nil {
		c.Logger.Errorf("chat.getExportedChatInvite - error: %v", err)
		return nil, err
	} else if chatInviteDO == nil {
		err = mtproto.ErrInviteHashInvalid
		c.Logger.Errorf("chat.getExportedChatInvite - error: %v", err)
		return nil, err
	}
	if chatInviteDO.ChatId != in.ChatId {
		return nil, mtproto.ErrInviteHashInvalid
	}
	if _, err = c.requireInvitePermission(in.ChatId, callerID, chatInviteDO.AdminId); err != nil {
		return nil, err
	}

	return c.svcCtx.Dao.MakeChatInviteExported(c.ctx, chatInviteDO), nil
}
