package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

type SavedDialogsDAO struct{ db DB }

func NewSavedDialogsDAO(db DB) *SavedDialogsDAO { return &SavedDialogsDAO{db: db} }

const savedDialogColumns = `user_id, peer_type, peer_id, pinned, top_message, deleted`

func scanSavedDialog(row interface{ Scan(...any) error }) (*dataobject.SavedDialogsDO, error) {
	do := new(dataobject.SavedDialogsDO)
	err := row.Scan(&do.UserId, &do.PeerType, &do.PeerId, &do.Pinned, &do.TopMessage, &do.Deleted)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}
func scanSavedDialogs(rows pgx.Rows) ([]dataobject.SavedDialogsDO, error) {
	return scanRows(rows, func(rows pgx.Rows, do *dataobject.SavedDialogsDO) error {
		v, err := scanSavedDialog(rows)
		if err != nil {
			return err
		}
		if v != nil {
			*do = *v
		}
		return nil
	})
}

func (d *SavedDialogsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.SavedDialogsDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO saved_dialogs (user_id, peer_type, peer_id, pinned, top_message)
 VALUES ($1,$2,$3,0,$4) ON CONFLICT (user_id, peer_type, peer_id) DO UPDATE SET top_message = EXCLUDED.top_message, deleted = FALSE RETURNING id`, do.UserId, do.PeerType, do.PeerId, do.TopMessage).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}
func (d *SavedDialogsDAO) Select(ctx context.Context, userID int64, peerType int32, peerID int64) (*dataobject.SavedDialogsDO, error) {
	return d.SelectOn(ctx, d.db, userID, peerType, peerID)
}
func (d *SavedDialogsDAO) SelectOn(ctx context.Context, db DB, userID int64, peerType int32, peerID int64) (*dataobject.SavedDialogsDO, error) {
	return scanSavedDialog(db.QueryRow(ctx, `SELECT `+savedDialogColumns+` FROM saved_dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND deleted = FALSE`, userID, peerType, peerID))
}
func (d *SavedDialogsDAO) SelectPinnedDialogs(ctx context.Context, userID int64) ([]dataobject.SavedDialogsDO, error) {
	return d.selectList(ctx, `user_id = $1 AND pinned > 0 AND deleted = FALSE ORDER BY pinned DESC`, userID)
}
func (d *SavedDialogsDAO) SelectPinnedDialogsWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.SavedDialogsDO)) ([]dataobject.SavedDialogsDO, error) {
	list, err := d.SelectPinnedDialogs(ctx, userID)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}
func (d *SavedDialogsDAO) SelectExcludePinnedDialogs(ctx context.Context, userID int64, topMessage, limit int32) ([]dataobject.SavedDialogsDO, error) {
	return d.selectList(ctx, `user_id = $1 AND pinned = 0 AND top_message < $2 AND deleted = FALSE ORDER BY top_message DESC LIMIT $3`, userID, topMessage, limit)
}
func (d *SavedDialogsDAO) SelectExcludePinnedDialogsWithCB(ctx context.Context, userID int64, topMessage, limit int32, cb func(int, int, *dataobject.SavedDialogsDO)) ([]dataobject.SavedDialogsDO, error) {
	list, err := d.SelectExcludePinnedDialogs(ctx, userID, topMessage, limit)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}
func (d *SavedDialogsDAO) SelectDialogs(ctx context.Context, userID int64, topMessage, limit int32) ([]dataobject.SavedDialogsDO, error) {
	return d.selectList(ctx, `user_id = $1 AND top_message < $2 AND deleted = FALSE ORDER BY top_message DESC LIMIT $3`, userID, topMessage, limit)
}
func (d *SavedDialogsDAO) SelectDialogsWithCB(ctx context.Context, userID int64, topMessage, limit int32, cb func(int, int, *dataobject.SavedDialogsDO)) ([]dataobject.SavedDialogsDO, error) {
	list, err := d.SelectDialogs(ctx, userID, topMessage, limit)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *SavedDialogsDAO) Count(ctx context.Context, userID int64, excludePinned bool) (int64, error) {
	query := `SELECT count(*) FROM saved_dialogs WHERE user_id = $1 AND deleted = FALSE`
	if excludePinned {
		query += ` AND pinned = 0`
	}
	var count int64
	if err := d.db.QueryRow(ctx, query, userID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
func (d *SavedDialogsDAO) selectList(ctx context.Context, where string, args ...any) ([]dataobject.SavedDialogsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+savedDialogColumns+` FROM saved_dialogs WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	return scanSavedDialogs(rows)
}
func (d *SavedDialogsDAO) UpdateUserUnPinned(ctx context.Context, userID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE saved_dialogs SET pinned = 0 WHERE user_id = $1 AND pinned > 0 AND deleted = FALSE`, userID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
func (d *SavedDialogsDAO) UpdateUserPeerPinned(ctx context.Context, pinned int64, userID int64, peerType int32, peerID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE saved_dialogs SET pinned = $1 WHERE user_id = $2 AND peer_type = $3 AND peer_id = $4`, pinned, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
func (d *SavedDialogsDAO) UpdateReadMaxId(ctx context.Context, readMaxID int32, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.UpdateReadMaxIdOn(ctx, d.db, readMaxID, userID, peerType, peerID)
}
func (d *SavedDialogsDAO) UpdateReadMaxIdOn(ctx context.Context, db DB, readMaxID int32, userID int64, peerType int32, peerID int64) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE saved_dialogs SET read_max_id = GREATEST(read_max_id, $1) WHERE user_id = $2 AND peer_type = $3 AND peer_id = $4 AND deleted = FALSE`, readMaxID, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
