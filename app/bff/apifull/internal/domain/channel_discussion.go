package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
)

var (
	ErrDiscussionChannelInvalid = errors.New("invalid discussion channel")
	ErrDiscussionAlreadyLinked  = errors.New("discussion group already linked")
	ErrDiscussionNotAllowed     = errors.New("discussion group change not allowed")
)

// ListDiscussionGroups returns megagroups that the caller owns or administers
// and can therefore use as a channel discussion group.
func ListDiscussionGroups(userID int64) ([]Channel, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if userID <= 0 {
		return nil, ErrInvalidChannelMember
	}
	rows, err := db.Query(`SELECT id FROM apifull_channel
		WHERE megagroup=1 AND (creator_user_id=? OR EXISTS (
			SELECT 1 FROM apifull_channel_member m
			WHERE m.channel_id=apifull_channel.id AND m.user_id=? AND m.admin_rights <> ''
		)) ORDER BY created_at DESC, id DESC`, userID, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	result := make([]Channel, 0, len(ids))
	for _, id := range ids {
		ch, ok, loadErr := LoadChannel(id)
		if loadErr != nil {
			return nil, loadErr
		}
		if ok {
			result = append(result, ch)
		}
	}
	return result, rows.Err()
}

// SetDiscussionGroup links a broadcast channel to a megagroup. Both records
// are locked in one transaction so clients never observe a half-written link.
// groupID == 0 clears an existing link.
func SetDiscussionGroup(actorID, broadcastID, groupID int64) error {
	if db == nil {
		return errors.New("domain mysql is not open")
	}
	if actorID <= 0 || broadcastID <= 0 || broadcastID == groupID || groupID < 0 {
		return ErrDiscussionChannelInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var broadcast int
	var owner int64
	err = tx.QueryRow(`SELECT broadcast, creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, broadcastID).
		Scan(&broadcast, &owner)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if broadcast == 0 || owner != actorID {
		// A delegated administrator must hold the explicit linked-peer right.
		var rawRights string
		err = tx.QueryRow(`SELECT admin_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, broadcastID, actorID).
			Scan(&rawRights)
		if err == sql.ErrNoRows {
			return ErrDiscussionNotAllowed
		}
		if err != nil {
			return err
		}
		var rights ChannelAdminRights
		if json.Unmarshal([]byte(rawRights), &rights) != nil || !rights.ManageLinkedPeers {
			return ErrDiscussionNotAllowed
		}
	}
	if groupID == 0 {
		if _, err = tx.Exec(`UPDATE apifull_channel SET discussion_group_id=NULL WHERE id=?`, broadcastID); err != nil {
			return err
		}
		return tx.Commit()
	}

	var megagroup int
	var groupOwner int64
	err = tx.QueryRow(`SELECT megagroup, creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, groupID).
		Scan(&megagroup, &groupOwner)
	if err == sql.ErrNoRows {
		return ErrDiscussionChannelInvalid
	}
	if err != nil {
		return err
	}
	if megagroup == 0 || (groupOwner != actorID && groupOwner != 0) {
		return ErrDiscussionNotAllowed
	}
	var existing sql.NullInt64
	err = tx.QueryRow(`SELECT discussion_group_id FROM apifull_channel WHERE id=?`, groupID).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if existing.Valid && existing.Int64 != 0 && existing.Int64 != broadcastID {
		return ErrDiscussionAlreadyLinked
	}
	if _, err = tx.Exec(`UPDATE apifull_channel SET discussion_group_id=? WHERE id=?`, groupID, broadcastID); err != nil {
		return err
	}
	return tx.Commit()
}

// DiscussionGroupID returns the linked discussion group for a channel.
func DiscussionGroupID(channelID int64) (int64, error) {
	if db == nil {
		return 0, errors.New("domain mysql is not open")
	}
	var id sql.NullInt64
	err := db.QueryRow(`SELECT discussion_group_id FROM apifull_channel WHERE id=?`, channelID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrChannelMissing
	}
	if err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, nil
	}
	return id.Int64, nil
}

// ChannelMessageReplies returns visible messages replying to rootID. It is
// intentionally channel-local; callers choose a linked discussion group when
// resolving comments for a broadcast channel.
func ChannelMessageReplies(userID, channelID int64, rootID, offsetID, offsetDate, addOffset, minID, maxID, limit int32) ([]ChannelMessage, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain mysql is not open")
	}
	if rootID <= 0 || limit <= 0 {
		return nil, 0, ErrInvalidMessageID
	}
	if limit > 100 {
		limit = 100
	}
	where := `channel_id=? AND (reply_to_msg_id=? OR reply_to_top_id=?) AND NOT EXISTS (
		SELECT 1 FROM apifull_channel_message_hidden h
		WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
		AND h.message_id=apifull_channel_message.message_id)`
	args := []any{channelID, rootID, rootID, userID}
	if offsetID > 0 {
		where += ` AND message_id<?`
		args = append(args, offsetID)
	}
	if offsetDate > 0 {
		where += ` AND date<?`
		args = append(args, offsetDate)
	}
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
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, limit)
	query := `SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned,
		reply_to_msg_id, reply_to_top_id FROM apifull_channel_message WHERE ` + where +
		` ORDER BY message_id DESC LIMIT ?`
	if addOffset > 0 {
		query += ` OFFSET ?`
		queryArgs = append(queryArgs, addOffset)
	}
	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result, err := scanChannelMessages(channelID, rows)
	return result, count, err
}
