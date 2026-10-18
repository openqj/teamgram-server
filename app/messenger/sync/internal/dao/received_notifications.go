package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// RecordReceivedNotifications persists the incoming message notifications
// before sync starts network fan-out. The primary key makes retries of the
// same update idempotent while keeping acknowledgement ownership in the
// messages BFF.
func (d *Dao) RecordReceivedNotifications(ctx context.Context, userID int64, updates *mtproto.Updates) error {
	if d == nil || d.Postgres == nil || d.Pool == nil {
		return errors.New("sync: received notification store is unavailable")
	}
	if userID <= 0 || updates == nil {
		return mtproto.ErrInputRequestInvalid
	}
	events, err := updateEvents(userID, updates)
	if err != nil {
		return err
	}
	type notification struct {
		id    int32
		flags int32
	}
	notifications := make([]notification, 0, len(events))
	seen := make(map[[2]int32]struct{}, len(events))
	for _, update := range events {
		if update == nil || update.GetPredicateName() != mtproto.Predicate_updateNewMessage {
			continue
		}
		message := update.GetMessage_MESSAGE()
		if message == nil || message.GetId() <= 0 || message.GetOut() {
			continue
		}
		// The sync update model does not carry a separate notification-flags
		// source. Zero is the canonical value for an ordinary, non-silent
		// notification; clients still receive the full update separately.
		row := notification{id: message.GetId()}
		key := [2]int32{row.id, row.flags}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		notifications = append(notifications, row)
	}
	if len(notifications) == 0 {
		return nil
	}
	return postgres.WithTx(ctx, d.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range notifications {
			if _, err := tx.Exec(ctx, `
				INSERT INTO bff_messages_received_notification (user_id, message_id, flags)
				VALUES ($1, $2, $3)
				ON CONFLICT (user_id, message_id, flags) DO NOTHING`, userID, row.id, row.flags); err != nil {
				return err
			}
		}
		return nil
	})
}
