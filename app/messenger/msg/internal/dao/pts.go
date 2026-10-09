package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
)

func (d *Dao) loadMessagePtsPostgres(ctx context.Context, db postgres_dao.DB, box *mtproto.MessageBox) error {
	err := db.QueryRow(ctx, `SELECT pts, pts_count FROM user_pts_updates
 WHERE user_id = $1 AND update_data::jsonb->'message_MESSAGE'->>'id' = $2
 AND update_type = $3 ORDER BY pts LIMIT 1`, box.UserId, fmt.Sprint(box.MessageId),
		mtproto.GetUpdateType(mtproto.MakeTLUpdateNewMessage(nil).To_Update())).Scan(&box.Pts, &box.PtsCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("messenger/msg: persisted message has no pts update")
	}
	return err
}

func (d *Dao) RestoreMessagePts(ctx context.Context, box *mtproto.MessageBox) error {
	if d.Postgres == nil || d.Postgres.Pool == nil {
		return errors.New("messenger/msg: postgres store is not configured")
	}
	return d.loadMessagePtsPostgres(ctx, d.Postgres.Pool, box)
}

func (d *Dao) LoadMessageReadHistoryUpdate(ctx context.Context, box *mtproto.MessageBox) (*mtproto.Update, error) {
	if d.Postgres == nil || d.Postgres.Pool == nil || box == nil {
		return nil, errors.New("messenger/msg: postgres store or message is not configured")
	}
	var data string
	err := d.Postgres.Pool.QueryRow(ctx, `SELECT update_data FROM user_pts_updates
 WHERE user_id=$1 AND update_type=$2 AND update_data::jsonb->>'max_id'=$3
 AND update_data::jsonb->'peer_PEER'->>'chat_id'=$4 ORDER BY pts LIMIT 1`,
		box.UserId, mtproto.GetUpdateType(mtproto.MakeTLUpdateReadHistoryInbox(nil).To_Update()), fmt.Sprint(box.MessageId), fmt.Sprint(box.PeerId)).Scan(&data)
	if err != nil {
		return nil, err
	}
	update := new(mtproto.Update)
	if err := jsonx.UnmarshalFromString(data, update); err != nil {
		return nil, err
	}
	return update, nil
}

func (d *Dao) AddToPtsQueueOn(ctx context.Context, tx postgres_dao.DB, userId int64, pts, ptsCount int32, update *mtproto.Update) (int32, error) {
	updateData, err := jsonx.Marshal(update)
	if err != nil {
		return 0, err
	}
	do := &dataobject.UserPtsUpdatesDO{UserId: userId, Pts: pts, PtsCount: ptsCount, UpdateType: mtproto.GetUpdateType(update), UpdateData: string(updateData), Date2: time.Now().Unix()}
	if d.Postgres == nil || d.Postgres.Store == nil || d.Postgres.Store.UserPtsUpdates == nil {
		return 0, errors.New("messenger/msg: postgres pts store is not configured")
	}
	id, _, err := d.Postgres.Store.UserPtsUpdates.InsertOn(ctx, tx, do)
	return int32(id), err
}

func (d *Dao) AddToPtsQueue(ctx context.Context, userId int64, pts, ptsCount int32, update *mtproto.Update) int32 {
	i, err := d.AddToPtsQueueE(ctx, userId, pts, ptsCount, update)
	if err != nil {
		logx.WithContext(ctx).Errorf("AddToPtsQueue - error: %v", err)
	}
	return i
}

func (d *Dao) AddToPtsQueueE(ctx context.Context, userId int64, pts, ptsCount int32, update *mtproto.Update) (int32, error) {
	updateData, err := jsonx.Marshal(update)
	if err != nil {
		return 0, err
	}

	do := &dataobject.UserPtsUpdatesDO{
		UserId:     userId,
		Pts:        pts,
		PtsCount:   ptsCount,
		UpdateType: mtproto.GetUpdateType(update),
		UpdateData: string(updateData),
		Date2:      time.Now().Unix(),
	}

	var i int64
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.UserPtsUpdates != nil {
		i, _, err = d.Postgres.Store.UserPtsUpdates.Insert(ctx, do)
	} else {
		i, _, err = d.UserPtsUpdatesDAO.Insert(ctx, do)
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("AddToPtsQueue - insert into user_pts_updates error: %v, do: %v", err, do)
		return int32(i), err
	}
	return int32(i), nil
}

func (d *Dao) AddToPtsQueueTx(tx *sqlx.Tx, userId int64, pts, ptsCount int32, update *mtproto.Update) (int32, error) {
	updateData, err := jsonx.Marshal(update)
	if err != nil {
		return 0, err
	}

	do := &dataobject.UserPtsUpdatesDO{
		UserId:     userId,
		Pts:        pts,
		PtsCount:   ptsCount,
		UpdateType: mtproto.GetUpdateType(update),
		UpdateData: string(updateData),
		Date2:      time.Now().Unix(),
	}

	i, _, err := d.UserPtsUpdatesDAO.InsertTx(tx, do)
	return int32(i), err
}

// CheckPtsContinuity checks whether a user's pts sequence is continuous
// starting from afterPts. Returns any gaps found as (expectedPts, actualPts) pairs.
func (d *Dao) CheckPtsContinuity(ctx context.Context, userId int64, afterPts int32) (gaps [][2]int32, err error) {
	var rows []dataobject.UserPtsUpdatesDO
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.UserPtsUpdates != nil {
		rows, err = d.Postgres.Store.UserPtsUpdates.SelectByGtPts(ctx, userId, afterPts)
	} else {
		rows, err = d.UserPtsUpdatesDAO.SelectByGtPts(ctx, userId, afterPts)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	expectedPts := afterPts
	for _, row := range rows {
		nextExpected := expectedPts + row.PtsCount
		if row.Pts != nextExpected {
			gaps = append(gaps, [2]int32{nextExpected, row.Pts})
			logx.WithContext(ctx).Errorf("pts gap detected - user_id: %d, expected_pts: %d, actual_pts: %d",
				userId, nextExpected, row.Pts)
		}
		expectedPts = row.Pts
	}

	return gaps, nil
}
