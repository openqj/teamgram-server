package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
)

type MessagesDAO struct{ db DB }

func NewMessagesDAO(db DB) *MessagesDAO { return &MessagesDAO{db: db} }

const messageColumns = `id, user_id, user_message_box_id, dialog_id1, dialog_id2,
dialog_message_id, sender_user_id, peer_type, peer_id, random_id,
message_filter_type, message_data, message, mentioned, media_unread, pinned,
has_reaction, reaction, reaction_date, reaction_unread, date2, ttl_period,
saved_peer_type, saved_peer_id, outbox_read_date, deleted`

func scanMessage(row interface{ Scan(...any) error }) (*dataobject.MessagesDO, error) {
	do := new(dataobject.MessagesDO)
	err := row.Scan(&do.Id, &do.UserId, &do.UserMessageBoxId, &do.DialogId1, &do.DialogId2,
		&do.DialogMessageId, &do.SenderUserId, &do.PeerType, &do.PeerId, &do.RandomId,
		&do.MessageFilterType, &do.MessageData, &do.Message, &do.Mentioned, &do.MediaUnread,
		&do.Pinned, &do.HasReaction, &do.Reaction, &do.ReactionDate, &do.ReactionUnread,
		&do.Date2, &do.TtlPeriod, &do.SavedPeerType, &do.SavedPeerId, &do.OutboxReadDate,
		&do.Deleted)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanMessages(rows pgx.Rows) ([]dataobject.MessagesDO, error) {
	defer rows.Close()
	result := make([]dataobject.MessagesDO, 0)
	for rows.Next() {
		var do dataobject.MessagesDO
		if _, err := scanMessageRow(rows, &do); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func scanMessageRow(row interface{ Scan(...any) error }, do *dataobject.MessagesDO) (struct{}, error) {
	err := row.Scan(&do.Id, &do.UserId, &do.UserMessageBoxId, &do.DialogId1, &do.DialogId2,
		&do.DialogMessageId, &do.SenderUserId, &do.PeerType, &do.PeerId, &do.RandomId,
		&do.MessageFilterType, &do.MessageData, &do.Message, &do.Mentioned, &do.MediaUnread,
		&do.Pinned, &do.HasReaction, &do.Reaction, &do.ReactionDate, &do.ReactionUnread,
		&do.Date2, &do.TtlPeriod, &do.SavedPeerType, &do.SavedPeerId, &do.OutboxReadDate,
		&do.Deleted)
	return struct{}{}, err
}

func (d *MessagesDAO) InsertOrReturnId(ctx context.Context, do *dataobject.MessagesDO) (int64, int64, error) {
	return d.insert(ctx, d.db, do)
}

func (d *MessagesDAO) InsertOrReturnIdOn(ctx context.Context, tx DB, do *dataobject.MessagesDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *MessagesDAO) insert(ctx context.Context, db DB, do *dataobject.MessagesDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO messages
 (user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id,
  sender_user_id, peer_type, peer_id, random_id, message_filter_type,
  message_data, message, mentioned, media_unread, pinned, saved_peer_type,
  saved_peer_id, date2, ttl_period, outbox_read_date)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT DO NOTHING
 RETURNING id`, do.UserId, do.UserMessageBoxId, do.DialogId1, do.DialogId2,
		do.DialogMessageId, do.SenderUserId, do.PeerType, do.PeerId, do.RandomId,
		do.MessageFilterType, do.MessageData, do.Message, do.Mentioned, do.MediaUnread,
		do.Pinned, do.SavedPeerType, do.SavedPeerId, do.Date2, do.TtlPeriod,
		do.OutboxReadDate).Scan(&id)
	if err == pgx.ErrNoRows {
		err = db.QueryRow(ctx, `SELECT id FROM messages
 WHERE user_id = $1 AND sender_user_id = $2 AND (
   ($3::bigint <> 0 AND random_id = $3) OR
   ($4::bigint <> 0 AND dialog_message_id = $4) OR
   (user_message_box_id = $5 AND random_id = $3 AND dialog_message_id = $4))
 ORDER BY id LIMIT 1`, do.UserId, do.SenderUserId, do.RandomId, do.DialogMessageId, do.UserMessageBoxId).Scan(&id)
		if err == nil {
			return id, 0, nil
		}
	}
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *MessagesDAO) SelectByStorageIDOn(ctx context.Context, db DB, userID, id int64) (*dataobject.MessagesDO, error) {
	return scanMessage(db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages WHERE user_id = $1 AND id = $2`, userID, id))
}

func (d *MessagesDAO) SelectByDeliveryIDOn(ctx context.Context, db DB, userID, senderUserID, dialogMessageID int64) (*dataobject.MessagesDO, error) {
	return scanMessage(db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages
 WHERE user_id = $1 AND sender_user_id = $2 AND dialog_message_id = $3 AND dialog_message_id <> 0`, userID, senderUserID, dialogMessageID))
}

func (d *MessagesDAO) SelectByRandomId(ctx context.Context, senderUserID, randomID int64) (*dataobject.MessagesDO, error) {
	return d.SelectByRandomIdForUser(ctx, d.db, senderUserID, senderUserID, randomID)
}

func (d *MessagesDAO) SelectByRandomIdOn(ctx context.Context, tx DB, userID, senderUserID, randomID int64) (*dataobject.MessagesDO, error) {
	return d.SelectByRandomIdForUser(ctx, tx, userID, senderUserID, randomID)
}

func (d *MessagesDAO) SelectByRandomIdForUser(ctx context.Context, db DB, userID, senderUserID, randomID int64) (*dataobject.MessagesDO, error) {
	return scanMessage(db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages
 WHERE user_id = $1 AND sender_user_id = $2 AND random_id = $3 LIMIT 1`, userID, senderUserID, randomID))
}

func (d *MessagesDAO) SelectByMessageId(ctx context.Context, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	return scanMessage(d.db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages WHERE user_id = $1 AND user_message_box_id = $2 AND deleted = FALSE LIMIT 1`, userID, messageID))
}

// SelectByMessageDataId locates a message by the dialog-wide message id.
func (d *MessagesDAO) SelectByMessageDataId(ctx context.Context, userID, dialogMessageID int64) (*dataobject.MessagesDO, error) {
	return scanMessage(d.db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages WHERE user_id = $1 AND dialog_message_id = $2 AND deleted = FALSE LIMIT 1`, userID, dialogMessageID))
}

// SelectByMessageDataIdUserIdList returns a dialog message for each requested user.
func (d *MessagesDAO) SelectByMessageDataIdUserIdList(ctx context.Context, dialogMessageID int64, userIDs []int64) ([]dataobject.MessagesDO, error) {
	if len(userIDs) == 0 {
		rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages WHERE dialog_message_id = $1 AND deleted = FALSE`, dialogMessageID)
		if err != nil {
			return nil, err
		}
		return scanMessages(rows)
	}
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages WHERE dialog_message_id = $1 AND user_id = ANY($2::bigint[]) AND deleted = FALSE`, dialogMessageID, userIDs)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectByMessageIdList(ctx context.Context, userID int64, ids []int32) ([]dataobject.MessagesDO, error) {
	if len(ids) == 0 {
		return []dataobject.MessagesDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages WHERE user_id = $1 AND user_message_box_id = ANY($2::integer[]) AND deleted = FALSE ORDER BY user_message_box_id DESC`, userID, ids)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// SelectByMessageDataIdList returns all user views for the requested
// dialog-wide message ids. The generated MySQL DAO accepted a physical table
// name; PostgreSQL stores all message views in one relation, so the user
// scope is the authoritative partition key instead.
func (d *MessagesDAO) SelectByMessageDataIdList(ctx context.Context, _ int64, ids []int64) ([]dataobject.MessagesDO, error) {
	if len(ids) == 0 {
		return []dataobject.MessagesDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE dialog_message_id = ANY($1::bigint[]) AND deleted = FALSE
ORDER BY user_message_box_id DESC`, ids)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectPeerUserMessageId(ctx context.Context, peerID, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	return d.SelectPeerUserMessage(ctx, peerID, userID, messageID)
}

func (d *MessagesDAO) SelectPeerUserMessage(ctx context.Context, peerID, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	return scanMessage(d.db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages
	WHERE user_id = $1 AND dialog_message_id = (SELECT dialog_message_id FROM messages WHERE user_id = $2 AND user_message_box_id = $3 AND deleted = FALSE LIMIT 1) AND deleted = FALSE LIMIT 1`, peerID, userID, messageID))
}

func (d *MessagesDAO) SelectDialogLastMessageList(ctx context.Context, userID, dialogID1, dialogID2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND deleted = FALSE ORDER BY user_message_box_id DESC LIMIT $4`, userID, dialogID1, dialogID2, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectDialogMessageIdList(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND deleted = FALSE ORDER BY user_message_box_id DESC`, userID, dialogID1, dialogID2)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) DeleteMessagesByMessageIdList(ctx context.Context, userID int64, ids []int32) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := d.db.Exec(ctx, `UPDATE messages SET deleted = TRUE WHERE user_id = $1 AND user_message_box_id = ANY($2::integer[]) AND deleted = FALSE`, userID, ids)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

func (d *MessagesDAO) DeleteMessagesByMessageIdListOn(ctx context.Context, db DB, userID int64, ids []int32) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := db.Exec(ctx, `UPDATE messages SET deleted = TRUE
WHERE user_id = $1 AND user_message_box_id = ANY($2::integer[]) AND deleted = FALSE`, userID, ids)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

func (d *MessagesDAO) UpdatePinned(ctx context.Context, pinned bool, userID int64, messageID int32) (int64, error) {
	return d.UpdatePinnedOn(ctx, d.db, pinned, userID, messageID)
}

func (d *MessagesDAO) UpdatePinnedOn(ctx context.Context, db DB, pinned bool, userID int64, messageID int32) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE messages SET pinned = $1 WHERE user_id = $2 AND user_message_box_id = $3`, pinned, userID, messageID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

func (d *MessagesDAO) UpdateMediaUnread(ctx context.Context, userID int64, messageID int32) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE messages SET media_unread = FALSE WHERE user_id = $1 AND user_message_box_id = $2`, userID, messageID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

func (d *MessagesDAO) UpdateMentionedAndMediaUnread(ctx context.Context, userID int64, messageID int32) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE messages SET mentioned = FALSE, media_unread = FALSE WHERE user_id = $1 AND user_message_box_id = $2`, userID, messageID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

func (d *MessagesDAO) CountMentioned(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND mentioned = TRUE AND deleted = FALSE`, userID, peerType, peerID).Scan(&count)
	return count, err
}

func (d *MessagesDAO) CountUnreadIncoming(ctx context.Context, userID, dialogID1, dialogID2, senderID int64, fromID, toID int32) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND sender_user_id <> $4 AND user_message_box_id > $5 AND user_message_box_id <= $6 AND deleted = FALSE`, userID, dialogID1, dialogID2, senderID, fromID, toID).Scan(&count)
	return count, err
}

func (d *MessagesDAO) SelectLastTwoPinnedList(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]int32, error) {
	rows, err := d.db.Query(ctx, `SELECT user_message_box_id FROM messages WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND pinned = TRUE AND deleted = FALSE ORDER BY user_message_box_id DESC LIMIT 2`, userID, dialogID1, dialogID2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]int32, 0, 2)
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (d *MessagesDAO) SelectPinnedList(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND pinned = TRUE AND deleted = FALSE ORDER BY user_message_box_id DESC`, userID, dialogID1, dialogID2)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) UpdateUnPinnedByIdList(ctx context.Context, userID int64, ids []int32) (int64, error) {
	return d.UpdateUnPinnedByIdListOn(ctx, d.db, userID, ids)
}

func (d *MessagesDAO) UpdateUnPinnedByIdListOn(ctx context.Context, db DB, userID int64, ids []int32) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := db.Exec(ctx, `UPDATE messages SET pinned = FALSE WHERE user_id = $1 AND user_message_box_id = ANY($2::integer[])`, userID, ids)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

func (d *MessagesDAO) SelectDialogLastMessageId(ctx context.Context, userID, dialogID1, dialogID2 int64) (int32, error) {
	return d.SelectDialogLastMessageIdOn(ctx, d.db, userID, dialogID1, dialogID2)
}

func (d *MessagesDAO) SelectDialogLastMessageIdOn(ctx context.Context, db DB, userID, dialogID1, dialogID2 int64) (int32, error) {
	var id int32
	err := db.QueryRow(ctx, `SELECT user_message_box_id FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT 1`, userID, dialogID1, dialogID2).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (d *MessagesDAO) SelectDialogLastMessageIdNotIdList(ctx context.Context, userID, dialogID1, dialogID2 int64, ids []int32) (int32, error) {
	var id int32
	if len(ids) == 0 {
		return d.SelectDialogLastMessageId(ctx, userID, dialogID1, dialogID2)
	}
	err := d.db.QueryRow(ctx, `SELECT user_message_box_id FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND user_message_box_id <> ALL($4::integer[]) AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT 1`, userID, dialogID1, dialogID2, ids).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (d *MessagesDAO) CountUnreadInDialog(ctx context.Context, userID, dialogID1, dialogID2, senderUserID int64, minID, maxID int32) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND sender_user_id <> $4 AND user_message_box_id > $5
  AND user_message_box_id <= $6 AND deleted = FALSE`, userID, dialogID1, dialogID2, senderUserID, minID, maxID).Scan(&count)
	return count, err
}

func (d *MessagesDAO) UpdateEditMessage(ctx context.Context, messageData, message string, userID int64, messageID int32) (int64, error) {
	return d.UpdateEditMessageOn(ctx, d.db, messageData, message, userID, messageID)
}

func (d *MessagesDAO) UpdateEditMessageOn(ctx context.Context, db DB, messageData, message string, userID int64, messageID int32) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE messages SET message_data = $1, message = $2 WHERE user_id = $3 AND user_message_box_id = $4 AND deleted = FALSE`, messageData, message, userID, messageID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

// Search returns messages in one dialog whose text matches q2. q2 is passed
// through as a SQL LIKE pattern to preserve the generated DAO contract (the
// caller supplies the surrounding wildcards).
func (d *MessagesDAO) Search(ctx context.Context, userID, dialogID1, dialogID2 int64, userMessageBoxID int32, q2 string, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND user_message_box_id < $4 AND deleted = FALSE
  AND message <> '' AND message ILIKE $5
ORDER BY user_message_box_id DESC LIMIT $6`, userID, dialogID1, dialogID2, userMessageBoxID, q2, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// SearchWithCB is Search with the callback shape used by generated service
// handlers.
func (d *MessagesDAO) SearchWithCB(ctx context.Context, userID, dialogID1, dialogID2 int64, userMessageBoxID int32, q2 string, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	result, err := d.Search(ctx, userID, dialogID1, dialogID2, userMessageBoxID, q2, limit)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		sz := len(result)
		for i := range result {
			cb(sz, i, &result[i])
		}
	}
	return result, nil
}

// SearchGlobal returns matching messages across all dialogs owned by a user.
func (d *MessagesDAO) SearchGlobal(ctx context.Context, userID int64, userMessageBoxID int32, q2 string, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND user_message_box_id < $2 AND deleted = FALSE
  AND message <> '' AND message ILIKE $3
ORDER BY user_message_box_id DESC LIMIT $4`, userID, userMessageBoxID, q2, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// SearchGlobalWithCB is SearchGlobal with the callback shape used by
// generated service handlers.
func (d *MessagesDAO) SearchGlobalWithCB(ctx context.Context, userID int64, userMessageBoxID int32, q2 string, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	result, err := d.SearchGlobal(ctx, userID, userMessageBoxID, q2, limit)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		sz := len(result)
		for i := range result {
			cb(sz, i, &result[i])
		}
	}
	return result, nil
}

// SelectByMediaType returns media rows in one dialog ordered newest first.
func (d *MessagesDAO) SelectByMediaType(ctx context.Context, userID, dialogID1, dialogID2 int64, mediaType int32, userMessageBoxID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND message_filter_type = $4 AND user_message_box_id < $5 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $6`, userID, dialogID1, dialogID2, mediaType, userMessageBoxID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// SelectByPhotoVideoMediaType covers the combined photo/video filter and the
// two individual filters used by Layer 229 clients.
func (d *MessagesDAO) SelectByPhotoVideoMediaType(ctx context.Context, userID, dialogID1, dialogID2 int64, userMessageBoxID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND message_filter_type IN (0, 7, 8) AND user_message_box_id < $4 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $5`, userID, dialogID1, dialogID2, userMessageBoxID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// SelectSentByMediaType returns media authored by userID across dialogs.
func (d *MessagesDAO) SelectSentByMediaType(ctx context.Context, userID int64, mediaType int32, userMessageBoxID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND sender_user_id = $1 AND message_filter_type = $2
  AND user_message_box_id < $3 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $4`, userID, mediaType, userMessageBoxID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// SelectPhoneCallList returns phone-call media rows in the user's view.
func (d *MessagesDAO) SelectPhoneCallList(ctx context.Context, userID int64, mediaType int32, userMessageBoxID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND message_filter_type = $2 AND user_message_box_id < $3 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $4`, userID, mediaType, userMessageBoxID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectBackwardSavedByOffsetIdLimit(ctx context.Context, userID int64, savedPeerType int32, savedPeerID int64, messageID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND saved_peer_type = $2 AND saved_peer_id = $3
  AND user_message_box_id < $4 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $5`, userID, savedPeerType, savedPeerID, messageID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectForwardSavedByOffsetIdLimit(ctx context.Context, userID int64, savedPeerType int32, savedPeerID int64, messageID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND saved_peer_type = $2 AND saved_peer_id = $3
  AND user_message_box_id >= $4 AND deleted = FALSE
ORDER BY user_message_box_id ASC LIMIT $5`, userID, savedPeerType, savedPeerID, messageID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectBackwardSavedByOffsetDateLimit(ctx context.Context, userID int64, savedPeerType int32, savedPeerID int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND saved_peer_type = $2 AND saved_peer_id = $3
  AND date2 <= $4 AND deleted = FALSE
ORDER BY date2 DESC, user_message_box_id DESC LIMIT $5`, userID, savedPeerType, savedPeerID, date2, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectForwardSavedByOffsetDateLimit(ctx context.Context, userID int64, savedPeerType int32, savedPeerID int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND saved_peer_type = $2 AND saved_peer_id = $3
  AND date2 >= $4 AND deleted = FALSE
ORDER BY date2 ASC, user_message_box_id ASC LIMIT $5`, userID, savedPeerType, savedPeerID, date2, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) CountSaved(ctx context.Context, userID int64, savedPeerType int32, savedPeerID int64) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages
WHERE user_id = $1 AND saved_peer_type = $2 AND saved_peer_id = $3 AND deleted = FALSE`, userID, savedPeerType, savedPeerID).Scan(&count)
	return count, err
}

// SelectBackwardByOffsetIdLimit returns history before offsetId.
func (d *MessagesDAO) SelectBackwardByOffsetIdLimit(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND user_message_box_id < $4 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $5`, userID, dialogID1, dialogID2, offsetID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// SelectForwardByOffsetIdLimit returns history at or after offsetId.
func (d *MessagesDAO) SelectForwardByOffsetIdLimit(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND user_message_box_id >= $4 AND deleted = FALSE
ORDER BY user_message_box_id ASC LIMIT $5`, userID, dialogID1, dialogID2, offsetID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectBackwardByOffsetDateLimit(ctx context.Context, userID, dialogID1, dialogID2 int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND date2 <= $4 AND deleted = FALSE
ORDER BY date2 DESC, user_message_box_id DESC LIMIT $5`, userID, dialogID1, dialogID2, date2, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectForwardByOffsetDateLimit(ctx context.Context, userID, dialogID1, dialogID2 int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND date2 >= $4 AND deleted = FALSE
ORDER BY date2 ASC, user_message_box_id ASC LIMIT $5`, userID, dialogID1, dialogID2, date2, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) CountByMediaType(ctx context.Context, userID, dialogID1, dialogID2 int64, mediaType int32) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND message_filter_type = $4 AND deleted = FALSE`, userID, dialogID1, dialogID2, mediaType).Scan(&count)
	return count, err
}

func (d *MessagesDAO) CountHistory(ctx context.Context, userID, dialogID1, dialogID2 int64) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND deleted = FALSE`, userID, dialogID1, dialogID2).Scan(&count)
	return count, err
}

func (d *MessagesDAO) CountHistoryBySender(ctx context.Context, userID, dialogID1, dialogID2, senderUserID int64) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND sender_user_id = $4 AND deleted = FALSE`, userID, dialogID1, dialogID2, senderUserID).Scan(&count)
	return count, err
}

func (d *MessagesDAO) CountUnreadMentions(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT count(*) FROM messages
WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3
  AND mentioned = TRUE AND media_unread = TRUE AND deleted = FALSE`, userID, peerType, peerID).Scan(&count)
	return count, err
}

func (d *MessagesDAO) SelectBackwardUnreadMentionsByOffsetIdLimit(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, minID, maxID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND user_message_box_id < $4 AND user_message_box_id >= $5
  AND ($6 = 0 OR user_message_box_id <= $6)
  AND mentioned = TRUE AND media_unread = TRUE AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $7`, userID, dialogID1, dialogID2, offsetID, minID, maxID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectForwardUnreadMentionsByOffsetIdLimit(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, minID, maxID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND user_message_box_id >= $4 AND user_message_box_id >= $5
  AND ($6 = 0 OR user_message_box_id <= $6)
  AND mentioned = TRUE AND media_unread = TRUE AND deleted = FALSE
ORDER BY user_message_box_id ASC LIMIT $7`, userID, dialogID1, dialogID2, offsetID, minID, maxID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectBackwardBySendUserIdOffsetIdLimit(ctx context.Context, userID, dialogID1, dialogID2, senderUserID int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+messageColumns+` FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3
  AND sender_user_id = $4 AND user_message_box_id < $5 AND deleted = FALSE
ORDER BY user_message_box_id DESC LIMIT $6`, userID, dialogID1, dialogID2, senderUserID, offsetID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (d *MessagesDAO) SelectPinnedMessageIdList(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]int32, error) {
	rows, err := d.db.Query(ctx, `SELECT user_message_box_id FROM messages
WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3 AND pinned = TRUE AND deleted = FALSE
ORDER BY user_message_box_id DESC`, userID, dialogID1, dialogID2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
