package dao

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

func (d *Dao) deleteMessagesPostgres(ctx context.Context, userID int64, msgIDs []int32) (*mtproto.PeerUtil, []int64, error) {
	if len(msgIDs) == 0 {
		return mtproto.MakePeerUtil(mtproto.PEER_EMPTY, 0), []int64{}, nil
	}
	rows, err := d.Postgres.Store.Messages.SelectByMessageIdList(ctx, userID, msgIDs)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return mtproto.MakePeerUtil(mtproto.PEER_EMPTY, 0), []int64{}, nil
	}
	dialogID := mtproto.DialogID{A: rows[0].DialogId1, B: rows[0].DialogId2}
	for _, row := range rows[1:] {
		if row.DialogId1 != dialogID.A || row.DialogId2 != dialogID.B {
			return mtproto.MakePeerUtil(mtproto.PEER_EMPTY, 0), []int64{}, nil
		}
	}
	lastRows, err := d.Postgres.Store.Messages.SelectDialogLastMessageList(ctx, userID, dialogID.A, dialogID.B, int32(len(msgIDs)+1))
	if err != nil {
		return nil, nil, err
	}
	lastID := int32(0)
	lastDate := time.Now().Unix()
	for _, row := range lastRows {
		found := false
		for _, deletedID := range msgIDs {
			if row.UserMessageBoxId == deletedID {
				found = true
				break
			}
		}
		if !found {
			lastID, lastDate = row.UserMessageBoxId, row.Date2
			break
		}
	}
	if err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, err := d.Postgres.Store.Messages.DeleteMessagesByMessageIdListOn(ctx, tx, userID, msgIDs)
		return err
	}); err != nil {
		return nil, nil, err
	}
	peer := dialogID.ToPeerUtil(userID)
	if _, err := d.Postgres.Store.Dialogs.UpdateOutboxDialog(ctx, lastID, lastDate, userID, peer.PeerType, peer.PeerId); err != nil {
		return nil, nil, err
	}
	deletedDataIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		deletedDataIDs = append(deletedDataIDs, row.DialogMessageId)
	}
	return peer, deletedDataIDs, nil
}

func (d *Dao) deletePhoneCallHistoryPostgres(ctx context.Context, userID int64) ([]int32, []int64, error) {
	rows, err := d.Postgres.Store.Messages.SelectPhoneCallList(ctx, userID, mtproto.MEDIA_PHONE_CALL, math.MaxInt32, math.MaxInt32)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return []int32{}, []int64{}, nil
	}
	byDialog := make(map[mtproto.DialogID][]int32)
	deletedIDs := make([]int32, 0, len(rows))
	dataIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		did := mtproto.DialogID{A: row.DialogId1, B: row.DialogId2}
		byDialog[did] = append(byDialog[did], row.UserMessageBoxId)
		deletedIDs = append(deletedIDs, row.UserMessageBoxId)
		dataIDs = append(dataIDs, row.DialogMessageId)
	}
	if err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		for did, ids := range byDialog {
			if _, err := d.Postgres.Store.Messages.DeleteMessagesByMessageIdListOn(ctx, tx, userID, ids); err != nil {
				return err
			}
			lastID, err := d.Postgres.Store.Messages.SelectDialogLastMessageIdOn(ctx, tx, userID, did.A, did.B)
			if err != nil {
				return err
			}
			peerID := mtproto.GetPeerIdByDialogId(userID, did)
			if _, err = d.Postgres.Store.Dialogs.UpdateCustomMapOn(ctx, tx, map[string]any{"top_message": lastID}, userID, mtproto.PEER_USER, peerID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return deletedIDs, dataIDs, nil
}

func (d *Dao) clearMentionsPostgres(ctx context.Context, userID, peerID int64, topMsgID int32, hasTopMsgID bool) (int32, error) {
	var cleared int32
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		query := `UPDATE messages SET mentioned = FALSE WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND mentioned = TRUE AND deleted = FALSE`
		args := []any{userID, mtproto.PEER_CHAT, peerID}
		if hasTopMsgID {
			query += ` AND user_message_box_id <= $4`
			args = append(args, topMsgID)
		}
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return err
		}
		cleared = int32(tag.RowsAffected())
		var unreadCount int32
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND mentioned = TRUE AND deleted = FALSE`, userID, mtproto.PEER_CHAT, peerID).Scan(&unreadCount); err != nil {
			return err
		}
		_, err = d.Postgres.Store.Dialogs.UpdateCustomMapOn(ctx, tx, map[string]any{"unread_mentions_count": unreadCount}, userID, mtproto.PEER_CHAT, peerID)
		return err
	})
	return cleared, err
}

func (d *Dao) addToPtsQueuePostgresTx(ctx context.Context, tx postgres_dao.DB, userID int64, pts, ptsCount int32, update *mtproto.Update) (int32, error) {
	updateData, err := jsonx.Marshal(update)
	if err != nil {
		return 0, err
	}
	do := &dataobject.UserPtsUpdatesDO{UserId: userID, Pts: pts, PtsCount: ptsCount,
		UpdateType: mtproto.GetUpdateType(update), UpdateData: string(updateData), Date2: time.Now().Unix()}
	id, _, err := d.Postgres.Store.UserPtsUpdates.InsertOn(ctx, tx, do)
	return int32(id), err
}
