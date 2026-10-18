package dao

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReceivedMessagesStore is the legacy monotonic cursor projection. The
// ReceivedMessagesProvider below owns the Layer 229 notification rows.
type ReceivedMessagesStore interface {
	Record(ctx context.Context, userID int64, maxID int32) error
}

type ReceivedNotification struct {
	ID    int32
	Flags int32
}

// ReceivedMessagesProvider atomically acknowledges the rows that were
// persisted by sync before session fan-out. Keeping this as a narrower
// interface lets old unit fixtures continue to exercise the fail-closed
// boundary without pretending that a cursor-only store is sufficient.
type ReceivedMessagesProvider interface {
	ReceivedMessagesStore
	RecordNotification(ctx context.Context, userID int64, messageID, flags int32) error
	Acknowledge(ctx context.Context, userID int64, maxID int32) ([]ReceivedNotification, error)
}

type postgresReceivedMessagesStore struct {
	pool *pgxpool.Pool
}

func NewPostgresReceivedMessagesStore(pool *pgxpool.Pool) ReceivedMessagesProvider {
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

func (s *postgresReceivedMessagesStore) RecordNotification(ctx context.Context, userID int64, messageID, flags int32) error {
	if s == nil || s.pool == nil {
		return errors.New("messages: PostgreSQL notification store is unavailable")
	}
	if userID <= 0 || messageID <= 0 {
		return errors.New("messages: invalid notification row")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bff_messages_received_notification (user_id, message_id, flags)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, message_id, flags) DO NOTHING`, userID, messageID, flags)
	return err
}

func (s *postgresReceivedMessagesStore) Acknowledge(ctx context.Context, userID int64, maxID int32) ([]ReceivedNotification, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("messages: PostgreSQL notification store is unavailable")
	}
	if userID <= 0 || maxID < 0 {
		return nil, errors.New("messages: invalid notification acknowledgement")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `
		DELETE FROM bff_messages_received_notification
		WHERE user_id=$1 AND message_id <= $2
		RETURNING message_id, flags`, userID, maxID)
	if err != nil {
		return nil, err
	}
	result := make([]ReceivedNotification, 0)
	for rows.Next() {
		var notification ReceivedNotification
		if err := rows.Scan(&notification.ID, &notification.Flags); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, notification)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == result[j].ID {
			return result[i].Flags < result[j].Flags
		}
		return result[i].ID < result[j].ID
	})
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
