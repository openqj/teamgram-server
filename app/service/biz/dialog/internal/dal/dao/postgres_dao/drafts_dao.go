package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

type DraftsDAO struct{ db DB }

func NewDraftsDAO(db DB) *DraftsDAO { return &DraftsDAO{db: db} }

const draftColumns = `id, user_id, peer_dialog_id, draft_type, draft_message_data, date2`

func scanDraft(row interface{ Scan(...any) error }) (*dataobject.DraftsDO, error) {
	do := new(dataobject.DraftsDO)
	err := row.Scan(&do.Id, &do.UserId, &do.PeerDialogId, &do.DraftType, &do.DraftMessageData, &do.Date2)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func (d *DraftsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.DraftsDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO drafts (user_id, peer_dialog_id, draft_type, draft_message_data, date2)
 VALUES ($1,$2,$3,$4::jsonb,$5)
 ON CONFLICT (user_id, peer_dialog_id) DO UPDATE SET draft_type = EXCLUDED.draft_type,
 draft_message_data = EXCLUDED.draft_message_data, date2 = EXCLUDED.date2 RETURNING id`,
		do.UserId, do.PeerDialogId, do.DraftType, do.DraftMessageData, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}
func (d *DraftsDAO) Select(ctx context.Context, userID int32, peerDialogID int64) (*dataobject.DraftsDO, error) {
	return scanDraft(d.db.QueryRow(ctx, `SELECT `+draftColumns+` FROM drafts WHERE user_id = $1 AND peer_dialog_id = $2`, userID, peerDialogID))
}
func (d *DraftsDAO) SelectIdList(ctx context.Context, userID int32) ([]int64, error) {
	rows, err := d.db.Query(ctx, `SELECT peer_dialog_id FROM drafts WHERE user_id = $1 ORDER BY peer_dialog_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
func (d *DraftsDAO) SelectIdListWithCB(ctx context.Context, userID int32, cb func(int, int, int64)) ([]int64, error) {
	list, err := d.SelectIdList(ctx, userID)
	if cb != nil {
		for i, id := range list {
			cb(len(list), i, id)
		}
	}
	return list, err
}
func (d *DraftsDAO) SelectByIdList(ctx context.Context, userID int32, ids []int64) ([]dataobject.DraftsDO, error) {
	if len(ids) == 0 {
		return []dataobject.DraftsDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+draftColumns+` FROM drafts WHERE user_id = $1 AND peer_dialog_id = ANY($2::bigint[]) ORDER BY peer_dialog_id`, userID, ids)
	if err != nil {
		return nil, err
	}
	return scanRows(rows, func(rows pgx.Rows, do *dataobject.DraftsDO) error {
		v, err := scanDraft(rows)
		if err != nil {
			return err
		}
		if v != nil {
			*do = *v
		}
		return nil
	})
}
func (d *DraftsDAO) SelectByIdListWithCB(ctx context.Context, userID int32, ids []int64, cb func(int, int, *dataobject.DraftsDO)) ([]dataobject.DraftsDO, error) {
	list, err := d.SelectByIdList(ctx, userID, ids)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}
func (d *DraftsDAO) ClearByIdList(ctx context.Context, userID int32, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := d.db.Exec(ctx, `UPDATE drafts SET draft_type = 0, draft_message_data = 'null'::jsonb WHERE user_id = $1 AND peer_dialog_id = ANY($2::bigint[])`, userID, ids)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
