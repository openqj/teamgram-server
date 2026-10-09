package dao

import (
	"context"
	"errors"
)

// GetDialogFilterTags reports whether this user has dialog filter tags on.
// A missing row is off, not an error.
func (d *Dao) GetDialogFilterTags(ctx context.Context, userID int64) (bool, error) {
	if d == nil || d.Postgres == nil || d.Postgres.Store == nil || d.Postgres.Store.DialogFilterTags == nil {
		return false, errors.New("dialog: postgres dialog-filter tags store is not configured")
	}
	return d.Postgres.Store.DialogFilterTags.Get(ctx, userID)
}

// SetDialogFilterTags upserts the tag switch for one user.
func (d *Dao) SetDialogFilterTags(ctx context.Context, userID int64, enabled bool) error {
	if d == nil || d.Postgres == nil || d.Postgres.Store == nil || d.Postgres.Store.DialogFilterTags == nil {
		return errors.New("dialog: postgres dialog-filter tags store is not configured")
	}
	return d.Postgres.Store.DialogFilterTags.Set(ctx, userID, enabled)
}
