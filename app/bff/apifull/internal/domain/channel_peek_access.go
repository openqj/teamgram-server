package domain

import (
	"database/sql"
	"errors"
	"time"
)

var ErrInvalidChannelPeekAccess = errors.New("invalid channel peek access")

// GrantChannelPeekAccess records the temporary read grant created by a valid
// private channel invite preview. A repeated check can extend an existing
// grant, but never shorten one that is still active.
func GrantChannelPeekAccess(channelID, userID, expiresAt int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if channelID <= 0 || userID <= 0 || expiresAt <= time.Now().Unix() {
		return ErrInvalidChannelPeekAccess
	}
	grantedAt := time.Now().Unix()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO apifull_channel_peek_access
		(channel_id, user_id, expires_at, granted_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (channel_id, user_id) DO UPDATE
		SET expires_at=GREATEST(apifull_channel_peek_access.expires_at, EXCLUDED.expires_at),
			granted_at=EXCLUDED.granted_at`, channelID, userID, expiresAt, grantedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ChannelPeekAccessUntil returns the active preview expiry for one user. An
// expired row is intentionally left for a later housekeeping/delete path so
// read checks remain side-effect free and cheap.
func ChannelPeekAccessUntil(channelID, userID, now int64) (int64, bool, error) {
	if db == nil {
		return 0, false, errors.New("domain PostgreSQL is not open")
	}
	if channelID <= 0 || userID <= 0 {
		return 0, false, ErrInvalidChannelPeekAccess
	}
	var expiresAt int64
	err := db.QueryRow(`SELECT expires_at FROM apifull_channel_peek_access
		WHERE channel_id=$1 AND user_id=$2 AND expires_at>$3`, channelID, userID, now).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return expiresAt, true, nil
}

// ChannelPeekAccessActive reports whether a user may still read a private
// channel after messages.checkChatInvite returned chatInvitePeek.
func ChannelPeekAccessActive(channelID, userID, now int64) (bool, error) {
	_, active, err := ChannelPeekAccessUntil(channelID, userID, now)
	return active, err
}
