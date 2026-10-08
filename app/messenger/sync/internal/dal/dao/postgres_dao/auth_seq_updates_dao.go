package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dal/dataobject"
)

type AuthSeqUpdatesDAO struct{ db DB }

func NewAuthSeqUpdatesDAO(db DB) *AuthSeqUpdatesDAO { return &AuthSeqUpdatesDAO{db: db} }

const authSeqColumns = `id, auth_id, user_id, seq, update_type, update_data, date2`

func (d *AuthSeqUpdatesDAO) Insert(ctx context.Context, do *dataobject.AuthSeqUpdatesDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO auth_seq_updates
 (auth_id, user_id, seq, update_type, update_data, date2)
 VALUES ($1,$2,$3,$4,$5,$6)
 ON CONFLICT (auth_id, user_id, seq) DO NOTHING RETURNING id`,
		do.AuthId, do.UserId, do.Seq, do.UpdateType, do.UpdateData, do.Date2).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *AuthSeqUpdatesDAO) SelectLastSeq(ctx context.Context, authID, userID int64) (*dataobject.AuthSeqUpdatesDO, error) {
	var do dataobject.AuthSeqUpdatesDO
	err := d.db.QueryRow(ctx, `SELECT `+authSeqColumns+` FROM auth_seq_updates
 WHERE auth_id = $1 AND user_id = $2 ORDER BY seq DESC LIMIT 1`, authID, userID).Scan(
		&do.Id, &do.AuthId, &do.UserId, &do.Seq, &do.UpdateType, &do.UpdateData, &do.Date2)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &do, nil
}

func (d *AuthSeqUpdatesDAO) SelectByGtSeq(ctx context.Context, authID, userID int64, seq int32) ([]dataobject.AuthSeqUpdatesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+authSeqColumns+` FROM auth_seq_updates
 WHERE auth_id = $1 AND user_id = $2 AND seq > $3 ORDER BY seq ASC`, authID, userID, seq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.AuthSeqUpdatesDO, 0)
	for rows.Next() {
		var do dataobject.AuthSeqUpdatesDO
		if err := rows.Scan(&do.Id, &do.AuthId, &do.UserId, &do.Seq, &do.UpdateType, &do.UpdateData, &do.Date2); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *AuthSeqUpdatesDAO) SelectByGtSeqWithCB(ctx context.Context, authID, userID int64, seq int32, cb func(int, int, *dataobject.AuthSeqUpdatesDO)) ([]dataobject.AuthSeqUpdatesDO, error) {
	result, err := d.SelectByGtSeq(ctx, authID, userID, seq)
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, err
}

func (d *AuthSeqUpdatesDAO) SelectByGtDate(ctx context.Context, authID, userID, date2 int64) ([]dataobject.AuthSeqUpdatesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+authSeqColumns+` FROM auth_seq_updates
 WHERE auth_id = $1 AND user_id = $2 AND date2 > $3 ORDER BY seq ASC`, authID, userID, date2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.AuthSeqUpdatesDO, 0)
	for rows.Next() {
		var do dataobject.AuthSeqUpdatesDO
		if err := rows.Scan(&do.Id, &do.AuthId, &do.UserId, &do.Seq, &do.UpdateType, &do.UpdateData, &do.Date2); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *AuthSeqUpdatesDAO) SelectByGtDateWithCB(ctx context.Context, authID, userID, date2 int64, cb func(int, int, *dataobject.AuthSeqUpdatesDO)) ([]dataobject.AuthSeqUpdatesDO, error) {
	result, err := d.SelectByGtDate(ctx, authID, userID, date2)
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, err
}
