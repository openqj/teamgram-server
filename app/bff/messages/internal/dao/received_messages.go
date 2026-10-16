package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReceivedMessagesStore is a monotonic PostgreSQL projection of a caller's
// max_id. It is not a Layer 229 notification provider: it cannot cancel PUSH
// delivery or supply canonical ReceivedNotifyMessage rows on its own.
type ReceivedMessagesStore interface {
	Record(ctx context.Context, userID int64, maxID int32) error
}

type postgresReceivedMessagesStore struct {
	pool *pgxpool.Pool
}

func NewPostgresReceivedMessagesStore(pool *pgxpool.Pool) ReceivedMessagesStore {
	return &postgresReceivedMessagesStore{pool: pool}
}

func (s *postgresReceivedMessagesStore) Record(ctx context.Context, userID int64, maxID int32) error {
	if s == nil || s.pool == nil {
		return errors.New("messages: PostgreSQL received-message store is unavailable")
	}
	if userID <= 0 || maxID < 0 {
		return errors.New("messages: invalid received-message cursor")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `
		INSERT INTO bff_messages_received_message (user_id, max_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET max_id = GREATEST(bff_messages_received_message.max_id, EXCLUDED.max_id),
		    updated_at = CURRENT_TIMESTAMP`, userID, maxID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
