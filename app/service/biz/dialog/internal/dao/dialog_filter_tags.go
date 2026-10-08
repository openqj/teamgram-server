package dao

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
)

const dialogFilterTagsDDL = `CREATE TABLE IF NOT EXISTS dialog_filter_tags (
  user_id bigint NOT NULL,
  enabled tinyint NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

var (
	tagsMu    sync.Mutex
	tagsReady bool
)

func (d *Dao) dialogFilterTagsReady(ctx context.Context) error {
	if d == nil || d.Mysql == nil || d.Mysql.DB == nil {
		return errors.New("dialog db is not open")
	}
	tagsMu.Lock()
	defer tagsMu.Unlock()
	if tagsReady {
		return nil
	}
	if schemaReadOnly() {
		var row struct {
			TableName string `db:"table_name"`
		}
		if err := d.Mysql.DB.QueryRow(ctx, &row, `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_name = 'dialog_filter_tags'
		`); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("dialog_filter_tags table is missing from the provisioned schema")
			}
			return err
		}
		tagsReady = true
		return nil
	}
	if _, err := d.Mysql.DB.Exec(ctx, dialogFilterTagsDDL); err != nil {
		return err
	}
	tagsReady = true
	return nil
}

func schemaReadOnly() bool {
	value := strings.TrimSpace(os.Getenv("TEAMGRAM_APIFULL_SCHEMA_READONLY"))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

type dialogFilterTagsRow struct {
	Enabled int `db:"enabled"`
}

// GetDialogFilterTags reports whether this user has dialog filter tags on.
// A missing row is off, not an error.
func (d *Dao) GetDialogFilterTags(ctx context.Context, userID int64) (bool, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.DialogFilterTags != nil {
		return d.Postgres.Store.DialogFilterTags.Get(ctx, userID)
	}
	if err := d.dialogFilterTagsReady(ctx); err != nil {
		return false, err
	}
	var row dialogFilterTagsRow
	err := d.Mysql.DB.QueryRow(ctx, &row, "SELECT enabled FROM dialog_filter_tags WHERE user_id = ? LIMIT 1", userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return row.Enabled != 0, nil
}

// SetDialogFilterTags upserts the tag switch for one user.
func (d *Dao) SetDialogFilterTags(ctx context.Context, userID int64, enabled bool) error {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.DialogFilterTags != nil {
		return d.Postgres.Store.DialogFilterTags.Set(ctx, userID, enabled)
	}
	if err := d.dialogFilterTagsReady(ctx); err != nil {
		return err
	}
	on := 0
	if enabled {
		on = 1
	}
	_, err := d.Mysql.DB.Exec(ctx, "INSERT INTO dialog_filter_tags (user_id, enabled) VALUES (?, ?) ON DUPLICATE KEY UPDATE enabled = VALUES(enabled)", userID, on)
	return err
}
