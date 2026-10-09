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

// ChatEditExportedChatInvite
// chat.editExportedChatInvite flags:# self_id:long chat_id:long revoked:flags.2?true link:string expire_date:flags.0?int usage_limit:flags.1?int request_needed:flags.3?Bool title:flags.4?string = ExportedChatInvite;
func (c *ChatCore) ChatEditExportedChatInvite(in *chat.TLChatEditExportedChatInvite) (*chat.Vector_ExportedChatInvite, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Invites == nil || c.svcCtx.Dao.Postgres.Store.Participants == nil {
		return nil, mtproto.ErrInternalServerError
	}
	selfID, err := c.requireInviteSelf(in.SelfId)
	if err != nil {
		return nil, err
	}
	var (
		hash        = chat.GetInviteHashByLink(in.Link)
		chatInvites = make([]*mtproto.ExportedChatInvite, 0, 2)
	)

	var chatInviteDO *dataobject.ChatInvitesDO
	chatInviteDO, err = c.svcCtx.Dao.Postgres.Store.Invites.SelectByLink(c.ctx, hash)
	if err != nil {
		c.Logger.Errorf("chat.editExportedChatInvite - error: %v", err)
		return nil, err
	} else if chatInviteDO == nil {
		err = mtproto.ErrInviteHashInvalid
		c.Logger.Errorf("chat.editExportedChatInvite - error: %v", err)
		return nil, err
	}
	if chatInviteDO.ChatId != in.ChatId {
		return nil, mtproto.ErrInviteHashInvalid
	}
	if _, err = c.requireInvitePermission(in.ChatId, selfID, chatInviteDO.AdminId); err != nil {
		return nil, err
	}
	tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if txErr != nil {
		return nil, txErr
	}
	defer func() { _ = tx.Rollback(c.ctx) }()
	if in.Revoked {
		_, txErr = c.svcCtx.Dao.Postgres.Store.Invites.UpdateOn(c.ctx, tx, map[string]interface{}{"revoked": in.Revoked}, in.ChatId, hash)
		chatInviteDO.Revoked = in.Revoked
		chatInvites = append(chatInvites, c.svcCtx.Dao.MakeChatInviteExported(c.ctx, chatInviteDO))
		if txErr == nil && chatInviteDO.Permanent {
			link := chat.GenChatInviteHash()
			if c.isAPIFullChannel(in.ChatId) {
				link = chat.GenChannelInviteHash()
			}
			chatInviteDO = &dataobject.ChatInvitesDO{
				ChatId:        in.ChatId,
				AdminId:       chatInviteDO.AdminId,
				Link:          link,
				Permanent:     chatInviteDO.Permanent,
				Revoked:       false,
				RequestNeeded: false,
				StartDate:     0,
				ExpireDate:    0,
				UsageLimit:    0,
				Usage2:        0,
				Requested:     0,
				Title:         "",
				Date2:         time.Now().Unix(),
			}
			_, _, txErr = c.svcCtx.Dao.Postgres.Store.Invites.InsertOn(c.ctx, tx, chatInviteDO)
			if txErr != nil {
				return nil, txErr
			}
			chatInvites = append(chatInvites, c.svcCtx.Dao.MakeChatInviteExported(c.ctx, chatInviteDO))
			_, txErr = c.svcCtx.Dao.Postgres.Store.Participants.UpdateLinkOn(c.ctx, tx, chatInviteDO.Link, in.ChatId, chatInviteDO.AdminId)
			if txErr != nil {
				return nil, txErr
			}
		}
	} else {
		cMap := map[string]interface{}{}

		if in.GetExpireDate() != nil {
			cMap["expire_date"] = in.GetExpireDate().GetValue()
			chatInviteDO.ExpireDate = int64(in.GetExpireDate().GetValue())
		}
		if in.GetUsageLimit() != nil {
			cMap["usage_limit"] = in.GetUsageLimit().GetValue()
			chatInviteDO.UsageLimit = in.GetUsageLimit().GetValue()
		}
		if in.GetRequestNeeded() != nil {
			cMap["request_needed"] = mtproto.FromBool(in.GetRequestNeeded())
			chatInviteDO.RequestNeeded = mtproto.FromBool(in.GetRequestNeeded())
		}
		if in.GetTitle() != nil {
			cMap["title"] = in.GetTitle().GetValue()
			chatInviteDO.Title = in.GetTitle().GetValue()
		}

		_, txErr = c.svcCtx.Dao.Postgres.Store.Invites.UpdateOn(c.ctx, tx, cMap, in.ChatId, hash)
		if txErr != nil {
			return nil, txErr
		}
		chatInvites = append(chatInvites, c.svcCtx.Dao.MakeChatInviteExported(c.ctx, chatInviteDO))
	}
	if txErr = tx.Commit(c.ctx); txErr != nil {
		return nil, txErr
	}

	return &chat.Vector_ExportedChatInvite{
		Datas: chatInvites,
	}, nil
}
