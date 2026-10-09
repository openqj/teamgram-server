package domain

import "errors"

// ListLeftChannels returns channels for which the caller's latest persisted
// membership event is a leave.  The live member table intentionally removes a
// row on leave, so the immutable admin log is the source of truth for this
// takeout query.  A later join or invite event removes the channel from the
// result, and a current member row is an additional guard against stale logs.
func ListLeftChannels(userID int64, offset, limit int32) ([]Channel, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return nil, ErrInvalidChannelMember
	}
	if offset < 0 {
		return nil, errors.New("invalid left-channel offset")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := db.Query(`
		WITH latest AS (
			SELECT DISTINCT ON (channel_id) channel_id, action
			FROM apifull_channel_admin_log
			WHERE target_user_id=?
			ORDER BY channel_id, id DESC
		)
		SELECT c.id
		FROM apifull_channel c
		JOIN latest l ON l.channel_id=c.id AND l.action=?
		LEFT JOIN apifull_channel_member m
			ON m.channel_id=c.id AND m.user_id=?
		WHERE c.creator_user_id<>? AND m.channel_id IS NULL
		ORDER BY c.id DESC
		LIMIT ? OFFSET ?`, userID, ChannelAdminLogParticipantLeave, userID, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := make([]Channel, 0, limit)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		channel, ok, loadErr := LoadChannel(id)
		if loadErr != nil {
			return nil, loadErr
		}
		if ok {
			channels = append(channels, channel)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}
