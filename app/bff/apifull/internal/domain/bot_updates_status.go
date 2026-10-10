package domain

import (
	"database/sql"
	"errors"
	"unicode/utf8"
)

// BotUpdatesStatus is the durable status reported by a bot update consumer.
// The status is scoped to the bot user and replaced atomically on retry.
type BotUpdatesStatus struct {
	BotUserID           int64
	PendingUpdatesCount int32
	Message             string
}

// UpsertBotUpdatesStatus stores one canonical status snapshot. The operation
// is idempotent for retries of the same bot and uses PostgreSQL's upsert so a
// concurrent status report cannot leave duplicate snapshots.
func UpsertBotUpdatesStatus(botUserID int64, pendingUpdatesCount int32, message string) error {
	if db == nil {
		return errors.New("apifull: PostgreSQL domain is not open")
	}
	if botUserID <= 0 || pendingUpdatesCount < 0 || !utf8.ValidString(message) || utf8.RuneCountInString(message) > 4096 {
		return errors.New("apifull: invalid bot updates status")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`INSERT INTO apifull_bot_updates_status
		(bot_user_id, pending_updates_count, message, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (bot_user_id) DO UPDATE SET
		pending_updates_count = EXCLUDED.pending_updates_count,
		message = EXCLUDED.message,
		updated_at = CURRENT_TIMESTAMP`, botUserID, pendingUpdatesCount, message); err != nil {
		return err
	}
	return tx.Commit()
}

// LoadBotUpdatesStatus reads the last durable snapshot for focused provider
// tests and operational inspection. A missing row is reported as found=false.
func LoadBotUpdatesStatus(botUserID int64) (status BotUpdatesStatus, found bool, err error) {
	if db == nil {
		return status, false, errors.New("apifull: PostgreSQL domain is not open")
	}
	err = db.QueryRow(`SELECT bot_user_id, pending_updates_count, message
		FROM apifull_bot_updates_status WHERE bot_user_id = $1`, botUserID).
		Scan(&status.BotUserID, &status.PendingUpdatesCount, &status.Message)
	if errors.Is(err, sql.ErrNoRows) {
		return BotUpdatesStatus{}, false, nil
	}
	if err != nil {
		return BotUpdatesStatus{}, false, err
	}
	return status, true, nil
}
