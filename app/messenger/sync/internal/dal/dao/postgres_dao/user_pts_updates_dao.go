package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dal/dataobject"
)

type UserPtsUpdatesDAO struct{ db DB }

func NewUserPtsUpdatesDAO(db DB) *UserPtsUpdatesDAO { return &UserPtsUpdatesDAO{db: db} }

const userPtsColumns = `id, user_id, pts, pts_count, update_type, update_data, date2`

func (d *UserPtsUpdatesDAO) Insert(ctx context.Context, do *dataobject.UserPtsUpdatesDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO user_pts_updates
 (user_id, pts, pts_count, update_type, update_data, date2)
 VALUES ($1,$2,$3,$4,$5,$6)
 ON CONFLICT (user_id, pts) DO NOTHING RETURNING id`,
		do.UserId, do.Pts, do.PtsCount, do.UpdateType, do.UpdateData, do.Date2).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *UserPtsUpdatesDAO) SelectLastPts(ctx context.Context, userID int64) (*dataobject.UserPtsUpdatesDO, error) {
	var do dataobject.UserPtsUpdatesDO
	err := d.db.QueryRow(ctx, `SELECT `+userPtsColumns+` FROM user_pts_updates
 WHERE user_id = $1 ORDER BY pts DESC LIMIT 1`, userID).Scan(
		&do.Id, &do.UserId, &do.Pts, &do.PtsCount, &do.UpdateType, &do.UpdateData, &do.Date2)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &do, nil
}

func (d *UserPtsUpdatesDAO) SelectByGtPts(ctx context.Context, userID int64, pts int32) ([]dataobject.UserPtsUpdatesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+userPtsColumns+` FROM user_pts_updates
 WHERE user_id = $1 AND pts > $2 ORDER BY pts ASC`, userID, pts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.UserPtsUpdatesDO, 0)
	for rows.Next() {
		var do dataobject.UserPtsUpdatesDO
		if err := rows.Scan(&do.Id, &do.UserId, &do.Pts, &do.PtsCount, &do.UpdateType, &do.UpdateData, &do.Date2); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *UserPtsUpdatesDAO) SelectByGtPtsWithCB(ctx context.Context, userID int64, pts int32, cb func(int, int, *dataobject.UserPtsUpdatesDO)) ([]dataobject.UserPtsUpdatesDO, error) {
	result, err := d.SelectByGtPts(ctx, userID, pts)
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, err
}
