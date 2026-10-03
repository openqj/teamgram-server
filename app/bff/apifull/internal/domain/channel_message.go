package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrChannelMissing        = errors.New("channel missing")
	ErrNotCreator            = errors.New("not channel creator")
	ErrMessageMissing        = errors.New("channel message missing")
	ErrInvalidMessageID      = errors.New("invalid channel message id")
	ErrInvalidLocation       = errors.New("invalid channel location")
	ErrChannelWriteForbidden = errors.New("channel write forbidden")
	ErrDuplicateMessageID    = errors.New("duplicate channel message id")
)

type ChannelMessage struct {
	ChannelID    int64
	MessageID    int32
	Pts          int32
	Sender       int64
	Date         int64
	Text         string
	Edited       bool
	EditedAt     int64
	Pinned       bool
	ReplyToMsgID int32
	ReplyToTopID int32
}

const (
	channelEventNew    = "new"
	channelEventEdit   = "edit"
	channelEventDelete = "delete"
	channelEventPin    = "pin"
)

func InsertChannelMessage(channelID, sender, date int64, text string) (ChannelMessage, error) {
	return insertChannelMessage(channelID, sender, date, text, 0, 0)
}

// InsertChannelMessageWithReply persists the reply index used by channel
// discussions. The root is validated inside the same writer transaction.
func InsertChannelMessageWithReply(channelID, sender, date int64, text string, replyToMsgID, replyToTopID int32) (ChannelMessage, error) {
	return insertChannelMessage(channelID, sender, date, text, replyToMsgID, replyToTopID)
}

func insertChannelMessage(channelID, sender, date int64, text string, replyToMsgID, replyToTopID int32) (ChannelMessage, error) {
	var row ChannelMessage
	if db == nil {
		return row, errors.New("domain mysql is not open")
	}
	if date == 0 {
		date = time.Now().Unix()
	}
	tx, err := db.Begin()
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	if replyToMsgID < 0 || replyToTopID < 0 {
		return row, ErrInvalidMessageID
	}
	if replyToTopID > 0 && replyToMsgID == 0 {
		replyToMsgID = replyToTopID
	}
	var creator int64
	var broadcast int
	err = tx.QueryRow(`SELECT creator_user_id, broadcast FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator, &broadcast)
	if err == sql.ErrNoRows {
		return row, ErrChannelMissing
	}
	if err != nil {
		return row, err
	}
	if creator != sender {
		var rawRights, rawBanned string
		err = tx.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member
			WHERE channel_id=? AND user_id=?`, channelID, sender).Scan(&rawRights, &rawBanned)
		if err == sql.ErrNoRows {
			return row, ErrNotChannelMember
		}
		if err != nil {
			return row, err
		}
		if rawBanned != "" {
			var banned ChannelBannedRights
			if err = json.Unmarshal([]byte(rawBanned), &banned); err != nil {
				return row, err
			}
			if banned.Active(time.Now().Unix()) && (banned.ViewMessages || banned.SendMessages || banned.SendPlain) {
				return row, ErrChannelWriteForbidden
			}
		}
		if broadcast != 0 {
			var rights ChannelAdminRights
			if rawRights == "" || json.Unmarshal([]byte(rawRights), &rights) != nil || !rights.PostMessages {
				return row, ErrChannelWriteForbidden
			}
		}
	}
	lastID, pts, err := loadChannelMessageSequence(tx, channelID)
	if err != nil {
		return row, err
	}
	next := lastID + 1
	pts++
	if err = saveChannelMessageSequence(tx, channelID, next, pts); err != nil {
		return row, err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_channel_message
		(channel_id, message_id, sender_user_id, date, message, edited, edited_at, reply_to_msg_id, reply_to_top_id)
		VALUES (?,?,?,?,?,0,0,?,?)`, channelID, next, sender, date, text, replyToMsgID, replyToTopID); err != nil {
		return row, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, 1, channelEventNew, []int32{next}, sender, date, text, 0, false); err != nil {
		return row, err
	}
	if err = tx.Commit(); err != nil {
		return row, err
	}
	return ChannelMessage{ChannelID: channelID, MessageID: next, Pts: pts, Sender: sender, Date: date, Text: text,
		ReplyToMsgID: replyToMsgID, ReplyToTopID: replyToTopID}, nil
}

func UpdateChannelMessage(channelID, sender int64, messageID int32, text string) (ChannelMessage, error) {
	var row ChannelMessage
	if db == nil {
		return row, errors.New("domain mysql is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return row, ErrChannelMissing
	}
	if err != nil {
		return row, err
	}
	if creator != sender {
		return row, ErrNotCreator
	}
	var edited, pinned int
	err = tx.QueryRow(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id
		FROM apifull_channel_message WHERE channel_id=? AND message_id=? FOR UPDATE`, channelID, messageID).
		Scan(&row.MessageID, &row.Sender, &row.Date, &row.Text, &edited, &row.EditedAt, &pinned, &row.ReplyToMsgID, &row.ReplyToTopID)
	if err == sql.ErrNoRows {
		return ChannelMessage{}, ErrMessageMissing
	}
	if err != nil {
		return ChannelMessage{}, err
	}
	pts, err := advanceChannelMessagePTS(tx, channelID, 1)
	if err != nil {
		return ChannelMessage{}, err
	}
	editedAt := time.Now().Unix()
	if _, err = tx.Exec(`UPDATE apifull_channel_message SET message=?, edited=1, edited_at=? WHERE channel_id=? AND message_id=?`,
		text, editedAt, channelID, messageID); err != nil {
		return ChannelMessage{}, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, 1, channelEventEdit, []int32{messageID}, row.Sender, row.Date, text, editedAt, pinned != 0); err != nil {
		return ChannelMessage{}, err
	}
	if err = tx.Commit(); err != nil {
		return ChannelMessage{}, err
	}
	row.ChannelID = channelID
	row.Pts = pts
	row.Text = text
	row.Edited = true
	row.EditedAt = editedAt
	row.Pinned = pinned != 0
	return row, nil
}

func SetChannelMessagePinned(channelID, sender int64, messageID int32, pinned bool) (ChannelMessage, error) {
	var row ChannelMessage
	if db == nil {
		return row, errors.New("domain mysql is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return row, ErrChannelMissing
	}
	if err != nil {
		return row, err
	}
	if creator != sender {
		return row, ErrNotCreator
	}
	var edited, pinFlag int
	err = tx.QueryRow(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id
		FROM apifull_channel_message WHERE channel_id=? AND message_id=? FOR UPDATE`, channelID, messageID).
		Scan(&row.MessageID, &row.Sender, &row.Date, &row.Text, &edited, &row.EditedAt, &pinFlag, &row.ReplyToMsgID, &row.ReplyToTopID)
	if err == sql.ErrNoRows {
		return ChannelMessage{}, ErrMessageMissing
	}
	if err != nil {
		return ChannelMessage{}, err
	}
	flag := 0
	if pinned {
		flag = 1
	}
	if _, err = tx.Exec(`UPDATE apifull_channel_message SET pinned=? WHERE channel_id=? AND message_id=?`,
		flag, channelID, messageID); err != nil {
		return ChannelMessage{}, err
	}
	pts, err := advanceChannelMessagePTS(tx, channelID, 1)
	if err != nil {
		return ChannelMessage{}, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, 1, channelEventPin, []int32{messageID}, row.Sender, row.Date, row.Text, row.EditedAt, pinned); err != nil {
		return ChannelMessage{}, err
	}
	if err = tx.Commit(); err != nil {
		return ChannelMessage{}, err
	}
	row.ChannelID = channelID
	row.Pts = pts
	row.Edited = edited != 0
	row.Pinned = pinned
	return row, nil
}

// ClearChannelMessagePins removes every pinned message in a channel as one
// atomic creator-authorized update and returns the affected message IDs.
func ClearChannelMessagePins(channelID, sender int64) ([]int32, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain mysql is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	_, err = lockChannelMessageWriter(tx, channelID, sender)
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(`SELECT message_id FROM apifull_channel_message WHERE channel_id=? AND pinned=1 ORDER BY message_id FOR UPDATE`, channelID)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()

	pts, err := advanceChannelMessagePTS(tx, channelID, int32(len(ids)))
	if err != nil {
		return nil, 0, err
	}
	if len(ids) > 0 {
		if _, err = tx.Exec(`UPDATE apifull_channel_message SET pinned=0 WHERE channel_id=? AND pinned=1`, channelID); err != nil {
			return nil, 0, err
		}
		if err = appendChannelEventTx(tx, channelID, pts, int32(len(ids)), channelEventPin, ids, 0, 0, "", 0, false); err != nil {
			return nil, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return ids, pts, nil
}

func DeleteChannelMessages(channelID, sender int64, ids []int32) ([]int32, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain mysql is not open")
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, 0, ErrInvalidMessageID
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	pts, err := lockChannelMessageWriter(tx, channelID, sender)
	if err != nil {
		return nil, 0, err
	}
	deleted := []int32{}
	if len(ids) > 0 {
		deleted, err = selectChannelMessageIDs(tx, channelID, ids, 0)
		if err != nil {
			return nil, 0, err
		}
	}
	if err = deleteChannelMessagesTx(tx, channelID, deleted); err != nil {
		return nil, 0, err
	}
	pts, err = advanceChannelMessagePTS(tx, channelID, int32(len(deleted)))
	if err != nil {
		return nil, 0, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, int32(len(deleted)), channelEventDelete, deleted, 0, 0, "", 0, false); err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return deleted, pts, nil
}

func DeleteChannelHistory(channelID, sender int64, maxID int32) ([]int32, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain mysql is not open")
	}
	if maxID < 0 {
		return nil, 0, ErrInvalidMessageID
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	pts, err := lockChannelMessageWriter(tx, channelID, sender)
	if err != nil {
		return nil, 0, err
	}
	deleted, err := selectChannelMessageIDs(tx, channelID, nil, maxID)
	if err != nil {
		return nil, 0, err
	}
	if err = deleteChannelMessagesTx(tx, channelID, deleted); err != nil {
		return nil, 0, err
	}
	pts, err = advanceChannelMessagePTS(tx, channelID, int32(len(deleted)))
	if err != nil {
		return nil, 0, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, int32(len(deleted)), channelEventDelete, deleted, 0, 0, "", 0, false); err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return deleted, pts, nil
}

func DeleteChannelParticipantHistory(channelID, actorID, participantID int64) (int32, int32, error) {
	if db == nil {
		return 0, 0, errors.New("domain mysql is not open")
	}
	if participantID <= 0 {
		return 0, 0, ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if err = lockChannelMessageDeletePermission(tx, channelID, actorID); err != nil {
		return 0, 0, err
	}
	if _, _, err = loadChannelMessageSequence(tx, channelID); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(`DELETE r FROM apifull_channel_message_content_read r
		JOIN apifull_channel_message m ON m.channel_id=r.channel_id AND m.message_id=r.message_id
		WHERE m.channel_id=? AND m.sender_user_id=?`, channelID, participantID); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(`DELETE h FROM apifull_channel_message_hidden h
		JOIN apifull_channel_message m ON m.channel_id=h.channel_id AND m.message_id=h.message_id
		WHERE m.channel_id=? AND m.sender_user_id=?`, channelID, participantID); err != nil {
		return 0, 0, err
	}
	deleted, err := selectChannelMessageIDsBySender(tx, channelID, participantID)
	if err != nil {
		return 0, 0, err
	}
	result, err := tx.Exec(`DELETE FROM apifull_channel_message WHERE channel_id=? AND sender_user_id=?`, channelID, participantID)
	if err != nil {
		return 0, 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	ptsCount := int32(affected)
	pts, err := advanceChannelMessagePTS(tx, channelID, ptsCount)
	if err != nil {
		return 0, 0, err
	}
	if len(deleted) > 0 {
		if err = appendChannelEventTx(tx, channelID, pts, ptsCount, channelEventDelete, deleted, participantID, 0, "", 0, false); err != nil {
			return 0, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return pts, ptsCount, nil
}

func lockChannelMessageDeletePermission(tx *sql.Tx, channelID, actorID int64) error {
	var creatorID int64
	err := tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if actorID == creatorID {
		return nil
	}
	var rawRights string
	err = tx.QueryRow(`SELECT admin_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, actorID).Scan(&rawRights)
	if err == sql.ErrNoRows {
		return ErrNotCreator
	}
	if err != nil {
		return err
	}
	var rights ChannelAdminRights
	if rawRights == "" || json.Unmarshal([]byte(rawRights), &rights) != nil || !rights.DeleteMessages {
		return ErrNotCreator
	}
	return nil
}

func HideChannelHistory(userID, channelID int64, maxID int32) (int32, error) {
	if db == nil {
		return 0, errors.New("domain mysql is not open")
	}
	if maxID < 0 {
		return 0, ErrInvalidMessageID
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var channelExists int64
	err = tx.QueryRow(`SELECT id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&channelExists)
	if err == sql.ErrNoRows {
		return 0, ErrChannelMissing
	}
	if err != nil {
		return 0, err
	}
	query := `INSERT IGNORE INTO apifull_channel_message_hidden (user_id, channel_id, message_id)
		SELECT ?, channel_id, message_id FROM apifull_channel_message WHERE channel_id=?`
	args := []any{userID, channelID}
	if maxID > 0 {
		query += ` AND message_id<=?`
		args = append(args, maxID)
	}
	result, err := tx.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int32(affected), nil
}

func lockChannelMessageWriter(tx *sql.Tx, channelID, sender int64) (int32, error) {
	var creator int64
	err := tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return 0, ErrChannelMissing
	}
	if err != nil {
		return 0, err
	}
	if creator != sender {
		return 0, ErrNotCreator
	}
	_, pts, err := loadChannelMessageSequence(tx, channelID)
	if err != nil {
		return 0, err
	}
	return pts, nil
}

func loadChannelMessageSequence(tx *sql.Tx, channelID int64) (int32, int32, error) {
	var lastID, pts int32
	err := tx.QueryRow(`SELECT last_message_id, pts FROM apifull_channel_message_seq WHERE channel_id=? FOR UPDATE`, channelID).Scan(&lastID, &pts)
	if err == nil {
		return lastID, pts, nil
	}
	if err != sql.ErrNoRows {
		return 0, 0, err
	}
	if err = tx.QueryRow(`SELECT COALESCE(MAX(message_id),0) FROM apifull_channel_message WHERE channel_id=?`, channelID).Scan(&lastID); err != nil {
		return 0, 0, err
	}
	pts = lastID
	if _, err = tx.Exec(`INSERT INTO apifull_channel_message_seq (channel_id, last_message_id, pts) VALUES (?,?,?)`, channelID, lastID, pts); err != nil {
		return 0, 0, err
	}
	return lastID, pts, nil
}

func saveChannelMessageSequence(tx *sql.Tx, channelID int64, lastID, pts int32) error {
	_, err := tx.Exec(`UPDATE apifull_channel_message_seq SET last_message_id=?, pts=? WHERE channel_id=?`, lastID, pts, channelID)
	return err
}

func advanceChannelMessagePTS(tx *sql.Tx, channelID int64, count int32) (int32, error) {
	lastID, pts, err := loadChannelMessageSequence(tx, channelID)
	if err != nil {
		return 0, err
	}
	pts += count
	if err = saveChannelMessageSequence(tx, channelID, lastID, pts); err != nil {
		return 0, err
	}
	return pts, nil
}

func appendChannelEventTx(tx *sql.Tx, channelID int64, pts, ptsCount int32, eventType string, messageIDs []int32, sender, date int64, text string, editedAt int64, pinned bool) error {
	if pts <= 0 || ptsCount <= 0 || eventType == "" {
		return nil
	}
	ids, err := json.Marshal(messageIDs)
	if err != nil {
		return err
	}
	pin := 0
	if pinned {
		pin = 1
	}
	_, err = tx.Exec(`INSERT INTO apifull_channel_event
		(channel_id, pts, pts_count, event_type, message_ids, sender_user_id, date, message, edited_at, pinned)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, channelID, pts, ptsCount, eventType, ids, sender, date, text, editedAt, pin)
	return err
}

func selectChannelMessageIDsBySender(tx *sql.Tx, channelID, sender int64) ([]int32, error) {
	rows, err := tx.Query(`SELECT message_id FROM apifull_channel_message WHERE channel_id=? AND sender_user_id=? ORDER BY message_id`, channelID, sender)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func selectChannelMessageIDs(tx *sql.Tx, channelID int64, ids []int32, maxID int32) ([]int32, error) {
	query := `SELECT message_id FROM apifull_channel_message WHERE channel_id=?`
	args := []any{channelID}
	if len(ids) > 0 {
		holders := make([]string, len(ids))
		for i, id := range ids {
			holders[i] = "?"
			args = append(args, id)
		}
		query += ` AND message_id IN (` + strings.Join(holders, ",") + `)`
	} else if maxID > 0 {
		query += ` AND message_id<=?`
		args = append(args, maxID)
	}
	query += ` ORDER BY message_id`
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deleted := make([]int32, 0, len(ids))
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		deleted = append(deleted, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return deleted, nil
}

func deleteChannelMessagesTx(tx *sql.Tx, channelID int64, ids []int32) error {
	if len(ids) == 0 {
		return nil
	}
	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	in := strings.Join(holders, ",")
	if _, err := tx.Exec(`DELETE FROM apifull_channel_message WHERE channel_id=? AND message_id IN (`+in+`)`, args...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM apifull_channel_message_hidden WHERE channel_id=? AND message_id IN (`+in+`)`, args...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM apifull_channel_message_content_read WHERE channel_id=? AND message_id IN (`+in+`)`, args...); err != nil {
		return err
	}
	return nil
}

func ListChannelMessages(userID, channelID int64, beforeID, limit int32) ([]ChannelMessage, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	q := `SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id
		FROM apifull_channel_message WHERE channel_id=? AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
			AND h.message_id=apifull_channel_message.message_id)`
	args := []any{channelID, userID}
	if beforeID > 0 {
		q += ` AND message_id<?`
		args = append(args, beforeID)
	}
	q += ` ORDER BY message_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelMessages(channelID, rows)
}

func ListChannelMessagesRange(userID, channelID int64, minID, maxID, limit int32) ([]ChannelMessage, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain mysql is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	where := `channel_id=? AND NOT EXISTS (
		SELECT 1 FROM apifull_channel_message_hidden h
		WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
		AND h.message_id=apifull_channel_message.message_id)`
	args := []any{channelID, userID}
	if minID > 0 {
		where += ` AND message_id>?`
		args = append(args, minID)
	}
	if maxID > 0 {
		where += ` AND message_id<?`
		args = append(args, maxID)
	}
	var count int32
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_message WHERE `+where, args...).Scan(&count); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any(nil), args...), limit)
	rows, err := db.Query(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id
		FROM apifull_channel_message WHERE `+where+` ORDER BY message_id DESC LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result, err := scanChannelMessages(channelID, rows)
	return result, count, err
}

func ListPinnedChannelMessages(userID, channelID int64, limit int32) ([]ChannelMessage, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := db.Query(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id
		FROM apifull_channel_message WHERE channel_id=? AND pinned=1 AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
			AND h.message_id=apifull_channel_message.message_id)
		ORDER BY message_id DESC LIMIT ?`, channelID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelMessages(channelID, rows)
}

func ChannelMessagesByID(userID, channelID int64, ids []int32) ([]ChannelMessage, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if len(ids) == 0 {
		return []ChannelMessage{}, nil
	}
	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	args = append(args, userID)
	rows, err := db.Query(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id
		FROM apifull_channel_message WHERE channel_id=? AND message_id IN (`+strings.Join(holders, ",")+`) AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
			AND h.message_id=apifull_channel_message.message_id)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelMessages(channelID, rows)
}

func likeContains(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 2)
	b.WriteByte('%')
	for _, r := range q {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

func SearchChannelMessages(userID, channelID int64, q string, sender int64, beforeID, addOffset, minDate, maxDate, minID, maxID, limit int32) ([]ChannelMessage, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain mysql is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	where := []string{"channel_id=?", `NOT EXISTS (
		SELECT 1 FROM apifull_channel_message_hidden h
		WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
		AND h.message_id=apifull_channel_message.message_id)`}
	args := []any{channelID, userID}
	if q != "" {
		where = append(where, `message LIKE ? ESCAPE '\\'`)
		args = append(args, likeContains(q))
	}
	if sender != 0 {
		where = append(where, "sender_user_id=?")
		args = append(args, sender)
	}
	if minID > 0 {
		where = append(where, "message_id>?")
		args = append(args, minID)
	}
	if maxID > 0 {
		where = append(where, "message_id<?")
		args = append(args, maxID)
	}
	if minDate > 0 {
		where = append(where, "date>=?")
		args = append(args, minDate)
	}
	if maxDate > 0 {
		where = append(where, "date<=?")
		args = append(args, maxDate)
	}
	clause := strings.Join(where, " AND ")
	var total int32
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_message WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	pageArgs := append([]any{}, args...)
	if beforeID > 0 {
		clause += " AND message_id<?"
		pageArgs = append(pageArgs, beforeID)
	}
	pageArgs = append(pageArgs, limit)
	query := `SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id
		FROM apifull_channel_message WHERE ` + clause + ` ORDER BY message_id DESC LIMIT ?`
	if addOffset > 0 {
		query += ` OFFSET ?`
		pageArgs = append(pageArgs, addOffset)
	}
	rows, err := db.Query(query, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out, err := scanChannelMessages(channelID, rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func TopChannelMessage(channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain mysql is not open")
	}
	var id sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(message_id) FROM apifull_channel_message WHERE channel_id=?`, channelID).Scan(&id); err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, nil
	}
	return int32(id.Int64), nil
}

func ChannelMessagePTS(channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain mysql is not open")
	}
	var pts int32
	err := db.QueryRow(`SELECT pts FROM apifull_channel_message_seq WHERE channel_id=?`, channelID).Scan(&pts)
	if err == sql.ErrNoRows {
		return TopChannelMessage(channelID)
	}
	return pts, err
}

func MarkChannelReadHistory(userID, channelID int64, maxID int32) (int32, error) {
	if db == nil {
		return 0, errors.New("domain mysql is not open")
	}
	if maxID <= 0 {
		return 0, ErrInvalidMessageID
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var channelExists int64
	err = tx.QueryRow(`SELECT id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&channelExists)
	if err == sql.ErrNoRows {
		return 0, ErrChannelMissing
	}
	if err != nil {
		return 0, err
	}
	var top int32
	if err = tx.QueryRow(`SELECT COALESCE(MAX(message_id),0) FROM apifull_channel_message WHERE channel_id=?`, channelID).Scan(&top); err != nil {
		return 0, err
	}
	if maxID > top {
		maxID = top
	}
	if _, err = tx.Exec(`INSERT INTO apifull_channel_read_state (user_id, channel_id, read_max_id)
		VALUES (?,?,?) ON DUPLICATE KEY UPDATE read_max_id=GREATEST(read_max_id, VALUES(read_max_id))`,
		userID, channelID, maxID); err != nil {
		return 0, err
	}
	var readMax int32
	if err = tx.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		userID, channelID).Scan(&readMax); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return readMax, nil
}

// MarkChannelMessageContentsRead stores durable, per-user content-read
// receipts. Validation and all inserts happen in one transaction so an
// unknown message cannot result in a partial receipt batch.
func MarkChannelMessageContentsRead(userID, channelID, accessHash int64, ids []int32) error {
	if db == nil {
		return errors.New("domain mysql is not open")
	}
	if userID <= 0 || channelID <= 0 || accessHash <= 0 {
		return ErrInvalidChannelAccessHash
	}
	if len(ids) == 0 {
		return ErrInvalidMessageID
	}
	seen := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return ErrInvalidMessageID
		}
		if _, ok := seen[id]; ok {
			return ErrDuplicateMessageID
		}
		seen[id] = struct{}{}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var storedAccessHash, creatorID int64
	err = tx.QueryRow(`SELECT access_hash, creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).
		Scan(&storedAccessHash, &creatorID)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if storedAccessHash != accessHash {
		return ErrInvalidChannelAccessHash
	}
	if userID != creatorID {
		var rawBanned string
		err = tx.QueryRow(`SELECT banned_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, userID).
			Scan(&rawBanned)
		if err == sql.ErrNoRows {
			return ErrNotChannelMember
		}
		if err != nil {
			return err
		}
		if rawBanned != "" {
			var banned ChannelBannedRights
			if err = json.Unmarshal([]byte(rawBanned), &banned); err != nil {
				return err
			}
			if banned.Kicks(time.Now().Unix()) {
				return ErrNotChannelMember
			}
		}
	}

	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	rows, err := tx.Query(`SELECT message_id FROM apifull_channel_message WHERE channel_id=? AND message_id IN (`+strings.Join(holders, ",")+`) FOR UPDATE`, args...)
	if err != nil {
		return err
	}
	found := make(map[int32]struct{}, len(ids))
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		found[id] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(found) != len(ids) {
		return ErrMessageMissing
	}

	readAt := time.Now().Unix()
	for _, id := range ids {
		if _, err = tx.Exec(`INSERT IGNORE INTO apifull_channel_message_content_read
			(user_id, channel_id, message_id, read_at) VALUES (?,?,?,?)`, userID, channelID, id, readAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ChannelReadMaxID(userID, channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain mysql is not open")
	}
	var readMax int32
	err := db.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		userID, channelID).Scan(&readMax)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return readMax, err
}

// ChannelMessageViewCounts reports how many persisted channel read cursors
// include each requested message.
func ChannelMessageViewCounts(channelID int64, ids []int32) (map[int32]int32, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	counts := make(map[int32]int32, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}
	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	rows, err := db.Query(`SELECT m.message_id, COUNT(r.user_id)
		FROM apifull_channel_message m
		LEFT JOIN apifull_channel_read_state r
			ON r.channel_id=m.channel_id AND r.read_max_id>=m.message_id
		WHERE m.channel_id=? AND m.message_id IN (`+strings.Join(holders, ",")+`)
		GROUP BY m.message_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, count int32
		if err = rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func ChannelOutboxMaxID(userID, channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain mysql is not open")
	}
	var maxID int32
	err := db.QueryRow(`SELECT COALESCE(MAX(message_id),0) FROM apifull_channel_message
		WHERE channel_id=? AND sender_user_id=?`, channelID, userID).Scan(&maxID)
	return maxID, err
}

func scanChannelMessages(channelID int64, rows *sql.Rows) ([]ChannelMessage, error) {
	out := []ChannelMessage{}
	for rows.Next() {
		var row ChannelMessage
		var edited, pinned int
		if err := rows.Scan(&row.MessageID, &row.Sender, &row.Date, &row.Text, &edited, &row.EditedAt, &pinned, &row.ReplyToMsgID, &row.ReplyToTopID); err != nil {
			return nil, err
		}
		row.ChannelID = channelID
		row.Edited = edited != 0
		row.Pinned = pinned != 0
		out = append(out, row)
	}
	return out, rows.Err()
}
