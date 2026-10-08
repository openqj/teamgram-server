package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// DialogFilterTagsDAO stores the per-user dialog-filter tag switch.
// PostgreSQL migrations create the table before the service starts, so this
// DAO does not mutate schema at request time.
type DialogFilterTagsDAO struct{ db DB }

func NewDialogFilterTagsDAO(db DB) *DialogFilterTagsDAO {
	return &DialogFilterTagsDAO{db: db}
}

func (d *DialogFilterTagsDAO) Get(ctx context.Context, userID int64) (bool, error) {
	var enabled bool
	err := d.db.QueryRow(ctx, `SELECT enabled FROM dialog_filter_tags WHERE user_id = $1`, userID).Scan(&enabled)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return enabled, err
}

func (d *DialogFilterTagsDAO) Set(ctx context.Context, userID int64, enabled bool) error {
	_, err := d.db.Exec(ctx, `INSERT INTO dialog_filter_tags (user_id, enabled)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET enabled = EXCLUDED.enabled`, userID, enabled)
	return err
}
