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

// ChatExportChatInvite
// chat.exportChatInvite flags:# chat_id:long admin_id:long legacy_revoke_permanent:flags.2?true request_needed:flags.3?true expire_date:flags.0?int usage_limit:flags.1?int title:flags.4?string = ExportedChatInvite;
func (c *ChatCore) ChatExportChatInvite(in *chat.TLChatExportChatInvite) (*mtproto.ExportedChatInvite, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Invites == nil {
		return nil, mtproto.ErrInternalServerError
	}
	callerId, err := c.requireInviteCaller()
	if err != nil {
		return nil, err
	}
	if in.AdminId != callerId {
		return nil, mtproto.ErrUserIdInvalid
	}
	if _, err := c.requireInvitePermission(in.ChatId, callerId, 0); err != nil {
		return nil, err
	}
	link := chat.GenChatInviteHash()
	if c.isAPIFullChannel(in.ChatId) {
		link = chat.GenChannelInviteHash()
	}
	chatInviteDO := &dataobject.ChatInvitesDO{
		ChatId:        in.ChatId,
		AdminId:       in.AdminId,
		Link:          link,
		Permanent:     false,
		Revoked:       false,
		RequestNeeded: in.RequestNeeded,
		StartDate:     0,
		ExpireDate:    int64(in.GetExpireDate().GetValue()),
		UsageLimit:    in.GetUsageLimit().GetValue(),
		Usage2:        0,
		Requested:     0,
		Title:         in.GetTitle().GetValue(),
		Date2:         time.Now().Unix(),
	}

	tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if txErr != nil {
		return nil, txErr
	}
	defer func() { _ = tx.Rollback(c.ctx) }()
	_, _, txErr = c.svcCtx.Dao.Postgres.Store.Invites.InsertOn(c.ctx, tx, chatInviteDO)
	if txErr == nil {
		txErr = tx.Commit(c.ctx)
	}
	if txErr != nil {
		err = txErr
	}
	if err != nil {
		c.Logger.Errorf("chat.exportChatInvite - error: %v", err)
		return nil, err
	}

	return c.svcCtx.Dao.MakeChatInviteExported(c.ctx, chatInviteDO), nil
}
