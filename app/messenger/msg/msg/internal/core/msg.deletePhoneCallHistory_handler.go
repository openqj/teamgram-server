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
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// MsgDeletePhoneCallHistory
// msg.deletePhoneCallHistory user_id:long auth_key_id:long revoke:Bool = messages.AffectedFoundMessages;
func (c *MsgCore) MsgDeletePhoneCallHistory(in *msg.TLMsgDeletePhoneCallHistory) (*mtproto.Messages_AffectedFoundMessages, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx.Dao.Postgres != nil {
		updates, pts, rows, err := c.svcCtx.Dao.DeletePhoneCallState(c.ctx, in.UserId, in.Revoke)
		if err != nil {
			return nil, err
		}
		ids := make([]int32, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UserMessageBoxId)
		}
		var count int32
		for _, update := range updates {
			count += update.PtsCount
		}
		return mtproto.MakeTLMessagesAffectedFoundMessages(&mtproto.Messages_AffectedFoundMessages{Pts: pts, PtsCount: count, Messages: ids}).To_Messages_AffectedFoundMessages(), nil
	}
	var (
		pts, ptsCount int32
		msgIdList     []int32
		msgDataIdList []int64
		err           error
	)

	if msgIdList, msgDataIdList, err = c.svcCtx.Dao.DeletePhoneCallHistory(c.ctx, in.UserId); err != nil {
		c.Logger.Errorf("DeleteMessages - %v", err)
		return nil, err
	}

	pts = c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, in.UserId, len(msgDataIdList))
	ptsCount = int32(len(msgDataIdList))

	// Write user_pts_updates before RPC return to guarantee getDifference completeness
	updateDelete := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
		Messages:  msgIdList,
		Pts_INT32: pts,
		PtsCount:  ptsCount,
	}).To_Update()
	c.svcCtx.Dao.AddToPtsQueue(c.ctx, in.UserId, pts, ptsCount, updateDelete)

	c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(
		c.ctx,
		&sync.TLSyncUpdatesNotMe{
			UserId:        in.UserId,
			PermAuthKeyId: in.AuthKeyId,
			Updates:       mtproto.MakeUpdatesByUpdates(updateDelete),
		})

	if in.Revoke {
		c.svcCtx.Dao.InboxClient.InboxDeleteMessagesToInbox(
			c.ctx,
			&inbox.TLInboxDeleteMessagesToInbox{
				FromId: in.UserId,
				Id:     msgDataIdList,
			})
	}

	return mtproto.MakeTLMessagesAffectedFoundMessages(&mtproto.Messages_AffectedFoundMessages{
		Pts:      pts,
		PtsCount: ptsCount,
		Offset:   0,
		Messages: msgIdList,
	}).To_Messages_AffectedFoundMessages(), nil
}
