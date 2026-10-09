package domain

import (
	"context"
	"errors"
	"time"
)

// RecordMessagesDelivery records every notification message acknowledged by a
// user. The primary key makes retries idempotent while retaining one receipt
// per user/message pair.
func RecordMessagesDelivery(ctx context.Context, userID int64, messageIDs []int32) error {
	if userID <= 0 {
		return errors.New("messages delivery: invalid user id")
	}
	if len(messageIDs) == 0 {
		return errors.New("messages delivery: empty message id list")
	}
	if db == nil {
		return errors.New("messages delivery: PostgreSQL store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	for _, messageID := range messageIDs {
		if messageID <= 0 {
			return errors.New("messages delivery: invalid message id")
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO apifull_message_delivery_report (user_id, message_id, received_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, message_id) DO UPDATE SET received_at = GREATEST(apifull_message_delivery_report.received_at, EXCLUDED.received_at)
		`, userID, messageID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
