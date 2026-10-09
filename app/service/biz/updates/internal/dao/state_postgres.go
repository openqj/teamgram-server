package dao

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
)

type UpdateSnapshot struct {
	State      *mtproto.Updates_State
	PtsUpdates []dataobject.UserPtsUpdatesDO
	SeqUpdates []dataobject.AuthSeqUpdatesDO
	HasMore    bool
	TooLong    bool
}

func committedUpdateState(ctx context.Context, db postgres_dao.DB, userID, authID int64) (*mtproto.Updates_State, error) {
	state := &mtproto.Updates_State{}
	err := db.QueryRow(ctx, `SELECT
	 COALESCE((SELECT pts FROM user_pts_updates WHERE user_id=$1 ORDER BY pts DESC LIMIT 1),0),
	 COALESCE((SELECT seq FROM auth_seq_updates WHERE user_id=$1 AND auth_id=$2 ORDER BY seq DESC LIMIT 1),0),
	 COALESCE((SELECT last_qts FROM apifull_secret_user_state WHERE user_id=$1),0),
	 GREATEST(COALESCE((SELECT date2::integer FROM auth_seq_updates WHERE user_id=$1 AND auth_id=$2 ORDER BY seq DESC LIMIT 1),1),
	 floor(extract(epoch FROM clock_timestamp()))::integer)`, userID, authID).Scan(&state.Pts, &state.Seq, &state.Qts, &state.Date)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLUpdatesState(state).To_Updates_State(), nil
}

func (d *Dao) withUpdateSnapshot(ctx context.Context, authID int64, fn func(pgx.Tx) error) error {
	if d == nil || d.Postgres == nil || d.Postgres.Pool == nil {
		return errors.New("updates: PostgreSQL state store is not configured")
	}
	conn, err := d.Postgres.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, `SELECT pg_advisory_unlock_shared(hashtextextended($1,0))`, counter.SeqKey(authID)); err != nil {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	// Take the lock before BEGIN: a snapshot taken while waiting for a writer
	// could hide its committed event while advancing the date past that event.
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1,0))`, counter.SeqKey(authID)); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CurrentUpdateState observes committed records without reserving a pts value.
func (d *Dao) CurrentUpdateState(ctx context.Context, userID, authID int64) (*mtproto.Updates_State, error) {
	var state *mtproto.Updates_State
	err := d.withUpdateSnapshot(ctx, authID, func(tx pgx.Tx) error {
		var err error
		state, err = committedUpdateState(ctx, tx, userID, authID)
		return err
	})
	return state, err
}

// LoadUpdateSnapshot keeps rows and their terminal state in one database
// snapshot, so a concurrent commit cannot advance the reply past unseen data.
func (d *Dao) LoadUpdateSnapshot(ctx context.Context, userID, authID int64, pts int32, date int64, limit int32, totalLimit *int32) (*UpdateSnapshot, error) {
	var snapshot *UpdateSnapshot
	err := d.withUpdateSnapshot(ctx, authID, func(tx pgx.Tx) error {
		var err error
		snapshot, err = loadUpdateSnapshotOn(ctx, tx, userID, authID, pts, date, limit, totalLimit)
		return err
	})
	return snapshot, err
}

func loadUpdateSnapshotOn(ctx context.Context, tx pgx.Tx, userID, authID int64, pts int32, date int64, limit int32, totalLimit *int32) (*UpdateSnapshot, error) {
	snapshot := &UpdateSnapshot{}
	var err error
	snapshot.State, err = committedUpdateState(ctx, tx, userID, authID)
	if err != nil {
		return nil, err
	}
	if pts < 0 || pts > snapshot.State.Pts {
		return nil, mtproto.ErrPersistentTimestampInvalid
	}
	if totalLimit == nil && snapshot.State.Pts > 4000000 {
		defaultLimit := int32(1000000)
		totalLimit = &defaultLimit
	}
	if totalLimit != nil && int64(pts)+int64(*totalLimit) < int64(snapshot.State.Pts) {
		snapshot.TooLong = true
		return snapshot, nil
	}
	snapshot.PtsUpdates, err = postgres_dao.NewUserPtsUpdatesDAO(tx).SelectByGtPts(ctx, userID, pts, limit+1)
	if err != nil {
		return nil, err
	}
	if len(snapshot.PtsUpdates) > int(limit) {
		snapshot.PtsUpdates = snapshot.PtsUpdates[:limit]
		snapshot.HasMore = true
	}
	rows, err := tx.Query(ctx, `SELECT id,auth_id,user_id,seq,update_type,update_data,date2 FROM (
	 SELECT id,auth_id,user_id,seq,update_type,update_data,date2 FROM auth_seq_updates
	 WHERE auth_id=$1 AND user_id=$2 AND date2=$3
	 UNION ALL
	 SELECT id,auth_id,user_id,seq,update_type,update_data,date2 FROM (
	  SELECT id,auth_id,user_id,seq,update_type,update_data,date2 FROM auth_seq_updates
	  WHERE auth_id=$1 AND user_id=$2 AND date2>$3
	  ORDER BY date2 FETCH FIRST $4 ROWS WITH TIES
	 ) newer
	) page ORDER BY date2,seq`, authID, userID, date, limit+1)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var row dataobject.AuthSeqUpdatesDO
		if err := rows.Scan(&row.Id, &row.AuthId, &row.UserId, &row.Seq, &row.UpdateType, &row.UpdateData, &row.Date2); err != nil {
			rows.Close()
			return nil, err
		}
		snapshot.SeqUpdates = append(snapshot.SeqUpdates, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Replay the boundary second to recover commits after the prior snapshot.
	// It does not consume this page's budget, so even a large group progresses.
	boundary := 0
	for boundary < len(snapshot.SeqUpdates) && snapshot.SeqUpdates[boundary].Date2 == date {
		boundary++
	}
	if len(snapshot.SeqUpdates)-boundary > int(limit) {
		end := boundary + int(limit)
		for end < len(snapshot.SeqUpdates) && snapshot.SeqUpdates[end].Date2 == snapshot.SeqUpdates[end-1].Date2 {
			end++
		}
		if end < len(snapshot.SeqUpdates) {
			snapshot.SeqUpdates = snapshot.SeqUpdates[:end]
			snapshot.HasMore = true
		}
	}
	if len(snapshot.SeqUpdates) > 0 {
		var more bool
		lastDate := snapshot.SeqUpdates[len(snapshot.SeqUpdates)-1].Date2
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth_seq_updates
	 WHERE auth_id=$1 AND user_id=$2 AND date2>$3)`, authID, userID, lastDate).Scan(&more); err != nil {
			return nil, err
		}
		snapshot.HasMore = snapshot.HasMore || more
	}
	return snapshot, nil
}
