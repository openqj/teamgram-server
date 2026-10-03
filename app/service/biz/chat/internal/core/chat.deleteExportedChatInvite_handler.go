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
)

// ChatDeleteExportedChatInvite
// chat.deleteExportedChatInvite self_id:long chat_id:long link:string = Bool;
func (c *ChatCore) ChatDeleteExportedChatInvite(in *chat.TLChatDeleteExportedChatInvite) (*mtproto.Bool, error) {
	selfID, err := c.requireInviteSelf(in.SelfId)
	if err != nil {
		return nil, err
	}
	link := chat.GetInviteHashByLink(in.Link)
	if _, err = c.requireInviteLinkPermission(in.ChatId, selfID, in.Link); err != nil {
		return nil, err
	}

	rows, err := c.svcCtx.Dao.ChatInvitesDAO.DeleteByLink(c.ctx, in.ChatId, link)
	if err != nil {
		c.Logger.Errorf("chat.deleteExportedChatInvite - error: %v", err)
		return nil, err
	}
	if rows == 0 {
		return nil, mtproto.ErrInviteHashInvalid
	}
	return mtproto.BoolTrue, nil
}
