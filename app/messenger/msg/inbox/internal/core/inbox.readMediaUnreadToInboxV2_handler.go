// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// InboxReadMediaUnreadToInboxV2
// inbox.readMediaUnreadToInboxV2 user_id:long peer_type:int peer_id:long dialog_message_id:long = Void;
func (c *InboxCore) InboxReadMediaUnreadToInboxV2(in *inbox.TLInboxReadMediaUnreadToInboxV2) (*mtproto.Void, error) {
	if in == nil || in.UserId <= 0 || in.PeerId <= 0 || in.DialogMessageId <= 0 || (in.PeerType != mtproto.PEER_USER && in.PeerType != mtproto.PEER_CHAT) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx.Dao.Postgres != nil {
		_, _, err := c.svcCtx.Dao.MutateMessageStateOnce(c.ctx, in.UserId, fmt.Sprintf("media:%d", in.DialogMessageId), func(tx pgx.Tx) ([]*mtproto.Update, error) {
			var id int32
			err := tx.QueryRow(c.ctx, `UPDATE messages SET media_unread=FALSE WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND dialog_message_id=$4 AND media_unread AND NOT deleted RETURNING user_message_box_id`, in.UserId, in.PeerType, in.PeerId, in.DialogMessageId).Scan(&id)
			if err == pgx.ErrNoRows {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			return []*mtproto.Update{mtproto.MakeTLUpdateReadMessagesContents(&mtproto.Update{Messages: []int32{id}, PtsCount: 1}).To_Update()}, nil
		})
		if err != nil {
			return nil, err
		}
		return mtproto.EmptyVoid, nil
	}
	unreadDO, err := c.svcCtx.Dao.SelectMessageByDataID(c.ctx, in.UserId, in.DialogMessageId)
	if err != nil {
		c.Logger.Errorf("inbox.readMediaUnreadToInboxV2 - error: %v", err)
		return nil, err
	} else if unreadDO == nil {
		err = mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("inbox.readMediaUnreadToInboxV2 - error: %v", err)
		return nil, err
	}

	if !unreadDO.MediaUnread {
		return mtproto.EmptyVoid, nil
	}
	_, err = c.svcCtx.Dao.UpdateMessageMediaUnread(c.ctx, unreadDO.UserId, unreadDO.UserMessageBoxId)
	if err != nil {
		c.Logger.Errorf("inbox.readMediaUnreadToInboxV2 - error: %v", err)
		return nil, err
	}

	pts := c.svcCtx.Dao.IDGenClient2.NextPtsId(c.ctx, in.UserId)
	if pts == 0 {
		c.Logger.Errorf("inbox.readMediaUnreadToInboxV2 - error: nextPtsId(%d) is 0", in.UserId)
		return nil, mtproto.ErrInternalServerError
	}

	updateReadMessagesContents := mtproto.MakeTLUpdateReadMessagesContents(&mtproto.Update{
		Messages:  []int32{unreadDO.UserMessageBoxId},
		Pts_INT32: pts,
		PtsCount:  1,
	}).To_Update()
	c.persistPtsUpdate(c.ctx, in.UserId, updateReadMessagesContents)

	_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId:  in.UserId,
		Updates: mtproto.MakeUpdatesByUpdates(updateReadMessagesContents),
	})

	return mtproto.EmptyVoid, nil
}
