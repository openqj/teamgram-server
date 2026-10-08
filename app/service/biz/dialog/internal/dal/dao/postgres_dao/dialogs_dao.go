package postgres_dao

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

const dialogColumns = `id, user_id, peer_type, peer_id, peer_dialog_id, pinned,
top_message, pinned_msg_id, read_inbox_max_id, read_outbox_max_id,
unread_count, unread_mentions_count, unread_reactions_count, unread_mark,
draft_type, draft_message_data, folder_id, folder_pinned, has_scheduled,
ttl_period, theme_emoticon, wallpaper_id, wallpaper_overridden, date2, deleted`

type DialogsDAO struct{ db DB }

func NewDialogsDAO(db DB) *DialogsDAO { return &DialogsDAO{db: db} }

func scanDialog(row interface{ Scan(...any) error }) (*dataobject.DialogsDO, error) {
	do := new(dataobject.DialogsDO)
	err := row.Scan(&do.Id, &do.UserId, &do.PeerType, &do.PeerId, &do.PeerDialogId,
		&do.Pinned, &do.TopMessage, &do.PinnedMsgId, &do.ReadInboxMaxId,
		&do.ReadOutboxMaxId, &do.UnreadCount, &do.UnreadMentionsCount,
		&do.UnreadReactionsCount, &do.UnreadMark, &do.DraftType,
		&do.DraftMessageData, &do.FolderId, &do.FolderPinned, &do.HasScheduled,
		&do.TtlPeriod, &do.ThemeEmoticon, &do.WallpaperId, &do.WallpaperOverridden,
		&do.Date2, &do.Deleted)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanDialogRows(rows pgx.Rows) ([]dataobject.DialogsDO, error) {
	return scanRows(rows, func(rows pgx.Rows, do *dataobject.DialogsDO) error {
		v, err := scanDialog(rows)
		if err != nil {
			return err
		}
		if v != nil {
			*do = *v
		}
		return nil
	})
}

func callbackDialogs(list []dataobject.DialogsDO, cb func(int, int, *dataobject.DialogsDO)) {
	if cb == nil {
		return
	}
	for i := range list {
		cb(len(list), i, &list[i])
	}
}

// InsertIgnore preserves the generated DAO contract while using an explicit
// PostgreSQL conflict policy for the user/peer idempotency key.
func (d *DialogsDAO) InsertIgnore(ctx context.Context, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertIgnore(ctx, d.db, do)
}

func (d *DialogsDAO) InsertIgnoreOn(ctx context.Context, tx DB, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertIgnore(ctx, tx, do)
}

func (d *DialogsDAO) InsertIgnoreTx(ctx context.Context, tx DB, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertIgnore(ctx, tx, do)
}

func (d *DialogsDAO) insertIgnore(ctx context.Context, db DB, do *dataobject.DialogsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO dialogs
 (user_id, peer_type, peer_id, peer_dialog_id, top_message, pinned_msg_id,
  read_inbox_max_id, read_outbox_max_id, unread_count, unread_mentions_count,
  unread_mark, draft_type, draft_message_data, folder_id, date2)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
 ON CONFLICT (user_id, peer_dialog_id) DO NOTHING RETURNING id`,
		do.UserId, do.PeerType, do.PeerId, do.PeerDialogId, do.TopMessage,
		do.PinnedMsgId, do.ReadInboxMaxId, do.ReadOutboxMaxId, do.UnreadCount,
		do.UnreadMentionsCount, do.UnreadMark, do.DraftType, do.DraftMessageData,
		do.FolderId, do.Date2).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *DialogsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, d.db, do)
}

func (d *DialogsDAO) InsertOrUpdateOn(ctx context.Context, tx DB, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, do)
}

func (d *DialogsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, do)
}

func (d *DialogsDAO) insertOrUpdate(ctx context.Context, db DB, do *dataobject.DialogsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO dialogs
 (user_id, peer_type, peer_id, peer_dialog_id, top_message, pinned_msg_id,
  unread_count, unread_mentions_count, draft_message_data, date2)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	 ON CONFLICT (user_id, peer_dialog_id) DO UPDATE SET
	  top_message = EXCLUDED.top_message,
  unread_count = dialogs.unread_count + EXCLUDED.unread_count,
  unread_mentions_count = dialogs.unread_mentions_count + EXCLUDED.unread_mentions_count,
  date2 = EXCLUDED.date2
 RETURNING id`, do.UserId, do.PeerType, do.PeerId, do.PeerDialogId, do.TopMessage,
		do.PinnedMsgId, do.UnreadCount, do.UnreadMentionsCount, do.DraftMessageData, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *DialogsDAO) InsertOrUpdateDialog(ctx context.Context, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertOrUpdateDialog(ctx, d.db, do)
}

func (d *DialogsDAO) InsertOrUpdateDialogOn(ctx context.Context, tx DB, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertOrUpdateDialog(ctx, tx, do)
}

func (d *DialogsDAO) InsertOrUpdateDialogTx(ctx context.Context, tx DB, do *dataobject.DialogsDO) (int64, int64, error) {
	return d.insertOrUpdateDialog(ctx, tx, do)
}

func (d *DialogsDAO) insertOrUpdateDialog(ctx context.Context, db DB, do *dataobject.DialogsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO dialogs
 (user_id, peer_type, peer_id, peer_dialog_id, top_message, pinned_msg_id,
  read_inbox_max_id, draft_message_data, date2)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
 ON CONFLICT (user_id, peer_dialog_id) DO UPDATE SET
  top_message = EXCLUDED.top_message, read_inbox_max_id = EXCLUDED.read_inbox_max_id,
  draft_message_data = EXCLUDED.draft_message_data, date2 = EXCLUDED.date2, deleted = FALSE
 RETURNING id`, do.UserId, do.PeerType, do.PeerId, do.PeerDialogId, do.TopMessage,
		do.PinnedMsgId, do.ReadInboxMaxId, do.DraftMessageData, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *DialogsDAO) UpdateOutboxDialog(ctx context.Context, topMessage int32, date2, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET unread_count = 0, deleted = FALSE,
	 top_message = $1, date2 = $2, unread_mark = FALSE,
	 draft_message_data = 'null'::jsonb WHERE user_id = $3 AND peer_type = $4 AND peer_id = $5`,
		topMessage, date2, userID, peerType, peerID)
}

func (d *DialogsDAO) UpdateInboxDialog(ctx context.Context, values map[string]any, userID int64, peerType int32, peerID int64) (int64, error) {
	values = cloneAllowed(values, map[string]bool{"top_message": true, "date2": true, "read_inbox_max_id": true, "unread_mark": true, "deleted": true})
	values["deleted"] = false
	keys := make([]string, 0, len(values))
	args := make([]any, 0, len(values)+3)
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sets := []string{"unread_count = unread_count + 1"}
	for i, key := range keys {
		sets = append(sets, fmt.Sprintf("%s = $%d", key, i+1))
		args = append(args, normalizeDialogValue(key, values[key]))
	}
	args = append(args, userID, peerType, peerID)
	query := fmt.Sprintf("UPDATE dialogs SET %s WHERE user_id = $%d AND peer_type = $%d AND peer_id = $%d", stringsJoin(sets, ", "), len(keys)+1, len(keys)+2, len(keys)+3)
	return d.update(ctx, d.db, query, args...)
}

func (d *DialogsDAO) update(ctx context.Context, db DB, query string, args ...any) (int64, error) {
	tag, err := db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

var allowedDialogColumns = map[string]bool{
	"top_message": true, "date2": true, "read_outbox_max_id": true,
	"read_inbox_max_id": true, "unread_count": true, "unread_mark": true,
	"pinned_msg_id": true, "deleted": true, "wallpaper_id": true,
	"wallpaper_overridden": true, "theme_emoticon": true, "ttl_period": true,
}

func cloneAllowed(input map[string]any, allowed map[string]bool) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		if allowed[key] {
			out[key] = value
		}
	}
	return out
}

func (d *DialogsDAO) updateMap(ctx context.Context, db DB, values map[string]any, userID int64, peerType int32, peerID int64) (int64, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if allowedDialogColumns[key] {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return 0, nil
	}
	sort.Strings(keys)
	args := make([]any, 0, len(keys)+3)
	sets := make([]string, 0, len(keys))
	for i, key := range keys {
		sets = append(sets, fmt.Sprintf("%s = $%d", key, i+1))
		args = append(args, normalizeDialogValue(key, values[key]))
	}
	args = append(args, userID, peerType, peerID)
	query := fmt.Sprintf("UPDATE dialogs SET %s WHERE user_id = $%d AND peer_type = $%d AND peer_id = $%d", stringsJoin(sets, ", "), len(keys)+1, len(keys)+2, len(keys)+3)
	return d.update(ctx, db, query, args...)
}

func normalizeDialogValue(column string, value any) any {
	if column != "deleted" && column != "unread_mark" && column != "wallpaper_overridden" {
		return value
	}
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	default:
		return value
	}
}

func stringsJoin(values []string, sep string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += sep + value
	}
	return out
}

func (d *DialogsDAO) SelectPinnedDialogs(ctx context.Context, userID int64) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND folder_id = 0 AND pinned > 0 AND deleted = FALSE ORDER BY pinned DESC`, userID)
}
func (d *DialogsDAO) SelectPinnedDialogsWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectPinnedDialogs(ctx, userID)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) SelectFolderPinnedDialogs(ctx context.Context, userID int64) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND folder_id = 1 AND folder_pinned > 0 AND deleted = FALSE ORDER BY folder_pinned DESC`, userID)
}

func (d *DialogsDAO) SelectFolderPinnedDialogsWithCB(ctx context.Context, userID int64, folderID int32, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.selectList(ctx, `WHERE user_id = $1 AND folder_id = $2 AND folder_pinned > 0 AND deleted = FALSE ORDER BY folder_pinned DESC`, userID, folderID)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) SelectPeerDialogList(ctx context.Context, userID int64, ids []int64) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND peer_dialog_id = ANY($2::bigint[]) AND deleted = FALSE ORDER BY date2 DESC`, userID, ids)
}
func (d *DialogsDAO) SelectPeerDialogListWithCB(ctx context.Context, userID int64, ids []int64, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectPeerDialogList(ctx, userID, ids)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) SelectDialog(ctx context.Context, userID int64, peerType int32, peerID int64) (*dataobject.DialogsDO, error) {
	return scanDialog(d.db.QueryRow(ctx, `SELECT `+dialogColumns+` FROM dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND deleted = FALSE`, userID, peerType, peerID))
}
func (d *DialogsDAO) SelectByPeerDialogId(ctx context.Context, userID, peerDialogID int64) (*dataobject.DialogsDO, error) {
	return scanDialog(d.db.QueryRow(ctx, `SELECT `+dialogColumns+` FROM dialogs WHERE user_id = $1 AND peer_dialog_id = $2 AND deleted = FALSE`, userID, peerDialogID))
}
func (d *DialogsDAO) SelectDialogs(ctx context.Context, userID int64, folderID int32) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND folder_id = $2 AND deleted = FALSE ORDER BY pinned DESC, folder_pinned DESC, date2 DESC`, userID, folderID)
}
func (d *DialogsDAO) SelectDialogsWithCB(ctx context.Context, userID int64, folderID int32, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectDialogs(ctx, userID, folderID)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) SelectExcludePinnedDialogs(ctx context.Context, userID int64) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND folder_id = 0 AND pinned = 0 AND deleted = FALSE ORDER BY date2 DESC`, userID)
}
func (d *DialogsDAO) SelectExcludePinnedDialogsWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectExcludePinnedDialogs(ctx, userID)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) SelectExcludeFolderPinnedDialogs(ctx context.Context, userID int64) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND folder_id = 1 AND folder_pinned = 0 AND deleted = FALSE ORDER BY date2 DESC`, userID)
}
func (d *DialogsDAO) SelectExcludeFolderPinnedDialogsWithCB(ctx context.Context, userID int64, folderID int32, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.selectList(ctx, `WHERE user_id = $1 AND folder_id = $2 AND folder_pinned = 0 AND deleted = FALSE ORDER BY date2 DESC`, userID, folderID)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) selectList(ctx context.Context, where string, args ...any) ([]dataobject.DialogsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+dialogColumns+` FROM dialogs `+where, args...)
	if err != nil {
		return nil, err
	}
	return scanDialogRows(rows)
}

func (d *DialogsDAO) UpdateReadInboxMaxId(ctx context.Context, unreadCount, readInboxMaxID int32, userID, peerDialogID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET unread_count = $1, unread_mark = FALSE, read_inbox_max_id = $2 WHERE user_id = $3 AND peer_dialog_id = $4`, unreadCount, readInboxMaxID, userID, peerDialogID)
}
func (d *DialogsDAO) UpdateReadOutboxMaxId(ctx context.Context, readOutboxMaxID int32, userID, peerDialogID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET read_outbox_max_id = $1 WHERE user_id = $2 AND peer_dialog_id = $3`, readOutboxMaxID, userID, peerDialogID)
}
func (d *DialogsDAO) UpdateTopMessage(ctx context.Context, topMessage int32, userID, peerDialogID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET top_message = $1 WHERE user_id = $2 AND peer_dialog_id = $3`, topMessage, userID, peerDialogID)
}
func (d *DialogsDAO) UpdatePinnedMsgId(ctx context.Context, pinnedMsgID int32, userID, peerDialogID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET pinned_msg_id = $1 WHERE user_id = $2 AND peer_dialog_id = $3`, pinnedMsgID, userID, peerDialogID)
}
func (d *DialogsDAO) Delete(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.update(ctx, d.db, `DELETE FROM dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, userID, peerType, peerID)
}
func (d *DialogsDAO) SelectDialogsByGTReadInboxMaxId(ctx context.Context, peerType int32, peerID int64, readInboxMaxID int32, userID int64) ([]int64, error) {
	rows, err := d.db.Query(ctx, `SELECT user_id FROM dialogs WHERE peer_type = $1 AND peer_id = $2 AND read_inbox_max_id >= $3 AND user_id <> $4`, peerType, peerID, readInboxMaxID, userID)
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

func (d *DialogsDAO) SelectDialogsByGTReadInboxMaxIdWithCB(ctx context.Context, peerType int32, peerID int64, readInboxMaxID int32, userID int64, cb func(int, int, int64)) ([]int64, error) {
	list, err := d.SelectDialogsByGTReadInboxMaxId(ctx, peerType, peerID, readInboxMaxID, userID)
	if cb != nil {
		for i, id := range list {
			cb(len(list), i, id)
		}
	}
	return list, err
}

func (d *DialogsDAO) UpdateCustomMap(ctx context.Context, values map[string]any, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.updateMap(ctx, d.db, values, userID, peerType, peerID)
}

func (d *DialogsDAO) UpdateCustomMapTx(ctx context.Context, tx DB, values map[string]any, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.updateMap(ctx, tx, values, userID, peerType, peerID)
}
func (d *DialogsDAO) SaveDraft(ctx context.Context, draftType int32, draftMessageData string, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET draft_type = $1, draft_message_data = $2::jsonb WHERE user_id = $3 AND peer_type = $4 AND peer_id = $5`, draftType, draftMessageData, userID, peerType, peerID)
}
func (d *DialogsDAO) SelectAllDrafts(ctx context.Context, userID int64) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND draft_type > 0 ORDER BY peer_dialog_id`, userID)
}
func (d *DialogsDAO) SelectAllDraftsWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectAllDrafts(ctx, userID)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) ClearAllDrafts(ctx context.Context, userID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET draft_type = 0, draft_message_data = 'null'::jsonb WHERE user_id = $1 AND draft_type = 2`, userID)
}
func (d *DialogsDAO) UpdatePeerFolderId(ctx context.Context, folderID int32, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET folder_id = $1 WHERE user_id = $2 AND peer_type = $3 AND peer_id = $4`, folderID, userID, peerType, peerID)
}
func (d *DialogsDAO) UpdatePeerDialogListFolderId(ctx context.Context, folderID int32, userID int64, ids []int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET folder_id = $1 WHERE user_id = $2 AND peer_dialog_id = ANY($3::bigint[])`, folderID, userID, ids)
}
func (d *DialogsDAO) UpdatePeerDialogListPinned(ctx context.Context, pinned int64, userID int64, ids []int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET pinned = $1 WHERE user_id = $2 AND folder_id = 0 AND peer_dialog_id = ANY($3::bigint[])`, pinned, userID, ids)
}
func (d *DialogsDAO) UpdateFolderPeerDialogListPinned(ctx context.Context, pinned int64, userID int64, ids []int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET folder_pinned = $1 WHERE user_id = $2 AND folder_id = 1 AND peer_dialog_id = ANY($3::bigint[])`, pinned, userID, ids)
}
func (d *DialogsDAO) UpdateUnPinnedNotIdList(ctx context.Context, userID int64, ids []int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET pinned = 0 WHERE user_id = $1 AND folder_id = 0 AND pinned > 0 AND NOT (peer_dialog_id = ANY($2::bigint[]))`, userID, ids)
}
func (d *DialogsDAO) UpdateFolderUnPinnedNotIdList(ctx context.Context, userID int64, ids []int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET folder_pinned = 0 WHERE user_id = $1 AND folder_id = 1 AND folder_pinned > 0 AND NOT (peer_dialog_id = ANY($2::bigint[]))`, userID, ids)
}
func (d *DialogsDAO) SelectAllDialogs(ctx context.Context, userID int64) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND deleted = FALSE ORDER BY date2 DESC`, userID)
}
func (d *DialogsDAO) SelectAllDialogsWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectAllDialogs(ctx, userID)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) SelectDialogsByPeerType(ctx context.Context, userID int64, peerTypes []int32) ([]dataobject.DialogsDO, error) {
	return d.selectList(ctx, `WHERE user_id = $1 AND peer_type = ANY($2::int[]) AND deleted = FALSE ORDER BY date2 DESC`, userID, peerTypes)
}
func (d *DialogsDAO) SelectDialogsByPeerTypeWithCB(ctx context.Context, userID int64, peerTypes []int32, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectDialogsByPeerType(ctx, userID, peerTypes)
	callbackDialogs(list, cb)
	return list, err
}
func (d *DialogsDAO) UpdateUnreadCount(ctx context.Context, unreadCount, unreadMentionsCount, unreadReactionsCount int32, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE dialogs SET unread_count = unread_count + $1, unread_mentions_count = unread_mentions_count + $2, unread_reactions_count = unread_reactions_count + $3 WHERE user_id = $4 AND peer_type = $5 AND peer_id = $6`, unreadCount, unreadMentionsCount, unreadReactionsCount, userID, peerType, peerID)
}
