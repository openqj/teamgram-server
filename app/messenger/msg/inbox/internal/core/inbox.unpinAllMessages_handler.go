// Copyright 2022 Teamgram Authors
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
	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// InboxUnpinAllMessages
// inbox.unpinAllMessages user_id:long auth_key_id:long peer_type:int peer_id:long = Void;
func (c *InboxCore) InboxUnpinAllMessages(in *inbox.TLInboxUnpinAllMessages) (*mtproto.Void, error) {
	if in == nil || in.UserId <= 0 || in.PeerId <= 0 ||
		(in.PeerType != mtproto.PEER_USER && in.PeerType != mtproto.PEER_CHAT) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx.Dao.Postgres != nil {
		var users []int64
		if in.PeerType == mtproto.PEER_USER {
			users = []int64{in.PeerId}
		} else if in.PeerType == mtproto.PEER_CHAT {
			rows, err := c.svcCtx.Dao.Postgres.Pool.Query(c.ctx, `SELECT user_id FROM chat_participants WHERE chat_id=$1 AND user_id<>$2 AND state=$3 ORDER BY user_id`, in.PeerId, in.UserId, mtproto.ChatMemberStateNormal)
			if err != nil {
				return nil, err
			}
			users, err = pgx.CollectRows(rows, pgx.RowTo[int64])
			if err != nil {
				return nil, err
			}
		} else {
			return nil, mtproto.ErrPeerIdInvalid
		}
		for _, userID := range users {
			peerID := in.PeerId
			if in.PeerType == mtproto.PEER_USER {
				peerID = in.UserId
			}
			if _, _, err := c.svcCtx.Dao.UnpinMessageState(c.ctx, userID, mtproto.MakePeerUtil(in.PeerType, peerID), false); err != nil {
				return nil, err
			}
		}
		return mtproto.EmptyVoid, nil
	}
	var (
		peer     = mtproto.MakePeerUtil(in.PeerType, in.PeerId)
		idList   = make([]int32, 0)
		pts      int32
		ptsCount int32
	)

	switch peer.PeerType {
	case mtproto.PEER_USER:
		dialogId := mtproto.MakeDialogId(peer.PeerId, peer.PeerType, in.UserId)
		_, _ = c.svcCtx.Dao.SelectPinnedMessages(
			c.ctx,
			peer.PeerId,
			dialogId.A,
			dialogId.B,
			func(sz, i int, v *dataobject.MessagesDO) {
				idList = append(idList, v.UserMessageBoxId)
			})
		if len(idList) == 0 {
			break
		}

		pts = c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, peer.PeerId, len(idList))
		ptsCount = int32(len(idList))
		updatePinnedMessages := mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{
			Pinned:    false,
			Peer_PEER: mtproto.MakePeerUser(in.UserId),
			Messages:  idList,
			Pts_INT32: pts,
			PtsCount:  ptsCount,
		}).To_Update()
		c.persistPtsUpdate(c.ctx, peer.PeerId, updatePinnedMessages)

		_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(
			c.ctx,
			&sync.TLSyncPushUpdates{
				UserId:  peer.PeerId,
				Updates: mtproto.MakeUpdatesByUpdates(updatePinnedMessages),
			})
	case mtproto.PEER_CHAT:
		// TODO: 性能优化
		dialogId := mtproto.MakeDialogId(0, peer.PeerType, in.PeerId)
		_, _ = c.svcCtx.Dao.SelectChatParticipants(
			c.ctx,
			peer.PeerId,
			func(sz, i int, v *dataobject.ChatParticipantsDO) {
				if v.UserId == in.UserId {
					return
				}
				if v.State != mtproto.ChatMemberStateNormal {
					return
				}

				_, _ = c.svcCtx.Dao.SelectPinnedMessages(
					c.ctx,
					v.UserId,
					dialogId.A,
					dialogId.B,
					func(sz, i int, v *dataobject.MessagesDO) {
						idList = append(idList, v.UserMessageBoxId)
					})

				if len(idList) == 0 {
					return
				}

				pts = c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, v.UserId, len(idList))
				ptsCount = int32(len(idList))
				updatePinnedMessages := mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{
					Pinned:    false,
					Peer_PEER: mtproto.MakePeerChat(peer.PeerId),
					Messages:  idList,
					Pts_INT32: pts,
					PtsCount:  ptsCount,
				}).To_Update()
				c.persistPtsUpdate(c.ctx, v.UserId, updatePinnedMessages)

				c.svcCtx.Dao.SyncClient.SyncPushUpdates(
					c.ctx,
					&sync.TLSyncPushUpdates{
						UserId:  v.UserId,
						Updates: mtproto.MakeUpdatesByUpdates(updatePinnedMessages),
					})
			},
		)
	case mtproto.PEER_CHANNEL:
	}

	return mtproto.EmptyVoid, nil
}
