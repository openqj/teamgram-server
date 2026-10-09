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

package dao

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
)

// ClearMentions clears only the mentioned marker for a basic-group dialog and
// refreshes the dialog's persisted unread mention count in the same transaction.
func (d *Dao) ClearMentions(ctx context.Context, userId, peerId int64, topMsgId int32, hasTopMsgId bool) (int32, error) {
	if d.Postgres != nil && d.Postgres.Store != nil {
		var cleared int32
		err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('messenger-user:' || $1::bigint::text, 0))`, userId); err != nil {
				return err
			}
			query := `UPDATE messages SET mentioned = FALSE WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND mentioned = TRUE AND deleted = FALSE`
			args := []any{userId, mtproto.PEER_CHAT, peerId}
			if hasTopMsgId {
				query += " AND user_message_box_id <= $4"
				args = append(args, topMsgId)
			}
			tag, err := tx.Exec(ctx, query, args...)
			if err != nil {
				return err
			}
			cleared = int32(tag.RowsAffected())
			var unread int32
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM messages WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND mentioned = TRUE AND deleted = FALSE`, userId, mtproto.PEER_CHAT, peerId).Scan(&unread); err != nil {
				return err
			}
			_, err = d.Postgres.Store.Dialogs.UpdateCustomMapOn(ctx, tx, map[string]any{"unread_mentions_count": unread}, userId, mtproto.PEER_CHAT, peerId)
			return err
		})
		if err != nil {
			return 0, err
		}
		return cleared, nil
	}
	var cleared int32
	table := d.MessagesDAO.CalcTableName(userId)
	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, storeResult *sqlx.StoreResult) {
		query := fmt.Sprintf("UPDATE %s SET mentioned = 0 WHERE user_id = ? AND peer_type = ? AND peer_id = ? AND mentioned = 1 AND deleted = 0", table)
		args := []interface{}{userId, mtproto.PEER_CHAT, peerId}
		if hasTopMsgId {
			query += " AND user_message_box_id <= ?"
			args = append(args, topMsgId)
		}
		rows, err := tx.Exec(query, args...)
		if err != nil {
			storeResult.Err = err
			return
		}
		rowsAffected, err := rows.RowsAffected()
		if err != nil {
			storeResult.Err = err
			return
		}
		cleared = int32(rowsAffected)

		var unreadCount int32
		countQuery := fmt.Sprintf("SELECT COUNT(id) FROM %s WHERE user_id = ? AND peer_type = ? AND peer_id = ? AND mentioned = 1 AND deleted = 0", table)
		if err = tx.QueryRowPartial(&unreadCount, countQuery, userId, mtproto.PEER_CHAT, peerId); err != nil {
			storeResult.Err = err
			return
		}
		_, storeResult.Err = d.DialogsDAO.UpdateCustomMapTx(tx, map[string]interface{}{
			"unread_mentions_count": unreadCount,
		}, userId, mtproto.PEER_CHAT, peerId)
	})
	if result.Err != nil {
		return 0, result.Err
	}
	if err := d.CachedConn.DelCache(ctx, dialog.GetDialogCacheKeyByPeer(userId, mtproto.PEER_CHAT, peerId)); err != nil {
		return 0, err
	}
	return cleared, nil
}

func (d *Dao) DeleteByMessageIdList(ctx context.Context, userId int64, idList []int32) (rowsAffected int64, err error) {
	if len(idList) == 0 {
		return 0, nil
	}
	return d.DeleteMessageByIDList(ctx, userId, idList)
}

func (d *Dao) GetLastMessageAndIdListByDialog(ctx context.Context, userId int64, peer *mtproto.PeerUtil) (lastMessage *mtproto.Message, idList []int32) {
	dialogId := mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
	d.SelectDialogMessageListWithCB(
		ctx,
		userId,
		dialogId.A,
		dialogId.B,
		func(sz, i int, v *dataobject.MessagesDO) {
			if i == 0 {
				var (
					m = new(mtproto.Message)
				)
				err := jsonx.UnmarshalFromString(v.MessageData, m)
				if err != nil {
					logx.WithContext(ctx).Errorf("error: %v, do: %v", err, v)
				} else {
					lastMessage = m.FixData()
				}
			}

			idList = append(idList, v.UserMessageBoxId)
		})
	return
}

//func (d *Dao) GetPeerMessageId(ctx context.Context, userId, messageId, peerId int32) int32 {
//	//do, _ := d.MessagesDAO.SelectPeerMessageId(ctx, peerId, userId, messageId)
//	//if do == nil {
//	//	return 0
//	//} else {
//	//	return do.UserMessageBoxId
//	//}
//}
//
//func (d *Dao) GetPeerDialogMessageIdList(ctx context.Context, userId int64, idList []int32) map[int64][]int32 {
//	doList, _ := d.MessagesDAO.SelectPeerDialogMessageIdList(ctx, userId, idList)
//	peerMessageIdListMap := make(map[int64][]int32)
//
//	for _, do := range doList {
//		if messageIdList, ok := peerMessageIdListMap[do.UserId]; !ok {
//			peerMessageIdListMap[do.UserId] = []int32{do.UserMessageBoxId}
//		} else {
//			peerMessageIdListMap[do.UserId] = append(messageIdList, do.UserMessageBoxId)
//		}
//	}
//
//	return peerMessageIdListMap
//}

func (d *Dao) GetMessageIdListByDialog(ctx context.Context, userId int64, peer *mtproto.PeerUtil) []int32 {
	var (
		dialogId = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		idList   []int32
	)

	d.SelectDialogMessageListWithCB(
		ctx,
		userId,
		dialogId.A,
		dialogId.B,
		func(sz, i int, v *dataobject.MessagesDO) {
			idList = append(idList, v.UserMessageBoxId)
		})

	return idList
}
