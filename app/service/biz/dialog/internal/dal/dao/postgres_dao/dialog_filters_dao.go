package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

type DialogFiltersDAO struct{ db DB }

func NewDialogFiltersDAO(db DB) *DialogFiltersDAO { return &DialogFiltersDAO{db: db} }

const filterColumns = `id, user_id, dialog_filter_id, is_chatlist, joined_by_slug,
slug, has_my_invites, dialog_filter, order_value, from_suggested, deleted`

func scanFilter(row interface{ Scan(...any) error }) (*dataobject.DialogFiltersDO, error) {
	do := new(dataobject.DialogFiltersDO)
	err := row.Scan(&do.Id, &do.UserId, &do.DialogFilterId, &do.IsChatlist,
		&do.JoinedBySlug, &do.Slug, &do.HasMyInvites, &do.DialogFilter,
		&do.OrderValue, &do.FromSuggested, &do.Deleted)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanFilters(rows pgx.Rows) ([]dataobject.DialogFiltersDO, error) {
	return scanRows(rows, func(rows pgx.Rows, do *dataobject.DialogFiltersDO) error {
		v, err := scanFilter(rows)
		if err != nil {
			return err
		}
		if v != nil {
			*do = *v
		}
		return nil
	})
}

func (d *DialogFiltersDAO) InsertOrUpdate(ctx context.Context, do *dataobject.DialogFiltersDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO dialog_filters
 (user_id, dialog_filter_id, is_chatlist, joined_by_slug, slug, has_my_invites,
  dialog_filter, order_value, from_suggested)
 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)
 ON CONFLICT (user_id, dialog_filter_id) DO UPDATE SET
  is_chatlist = EXCLUDED.is_chatlist, joined_by_slug = EXCLUDED.joined_by_slug,
  slug = EXCLUDED.slug, has_my_invites = EXCLUDED.has_my_invites,
  dialog_filter = EXCLUDED.dialog_filter, order_value = EXCLUDED.order_value,
  from_suggested = EXCLUDED.from_suggested, deleted = FALSE RETURNING id`,
		do.UserId, do.DialogFilterId, do.IsChatlist, do.JoinedBySlug, do.Slug,
		do.HasMyInvites, do.DialogFilter, do.OrderValue, do.FromSuggested).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *DialogFiltersDAO) SelectBySlug(ctx context.Context, userID int64, slug string) (*dataobject.DialogFiltersDO, error) {
	return scanFilter(d.db.QueryRow(ctx, `SELECT `+filterColumns+` FROM dialog_filters WHERE user_id = $1 AND slug = $2 AND deleted = FALSE ORDER BY order_value DESC LIMIT 1`, userID, slug))
}
func (d *DialogFiltersDAO) Select(ctx context.Context, userID int64, filterID int32) (*dataobject.DialogFiltersDO, error) {
	return scanFilter(d.db.QueryRow(ctx, `SELECT `+filterColumns+` FROM dialog_filters WHERE user_id = $1 AND dialog_filter_id = $2 AND deleted = FALSE ORDER BY order_value DESC LIMIT 1`, userID, filterID))
}
func (d *DialogFiltersDAO) SelectList(ctx context.Context, userID int64) ([]dataobject.DialogFiltersDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+filterColumns+` FROM dialog_filters WHERE user_id = $1 AND deleted = FALSE ORDER BY order_value DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanFilters(rows)
}
func (d *DialogFiltersDAO) SelectListWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.DialogFiltersDO)) ([]dataobject.DialogFiltersDO, error) {
	list, err := d.SelectList(ctx, userID)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}
func (d *DialogFiltersDAO) UpdateOrder(ctx context.Context, orderValue, userID int64, filterID int32) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE dialog_filters SET order_value = $1 WHERE user_id = $2 AND dialog_filter_id = $3`, orderValue, userID, filterID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
func (d *DialogFiltersDAO) Clear(ctx context.Context, userID int64, filterID int32) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE dialog_filters SET deleted = TRUE, dialog_filter = 'null'::jsonb, order_value = 0 WHERE user_id = $1 AND dialog_filter_id = $2`, userID, filterID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
