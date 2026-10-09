// Package counter owns PostgreSQL counters shared by idgen RPCs and mutations.
package counter

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

var (
	ErrInvalidCounter  = errors.New("idgen: invalid counter key or value")
	ErrCounterOverflow = errors.New("idgen: counter exceeds its positive integer range")
)

type CounterStore struct{ Pool *pgxpool.Pool }

func NewCounterStore(pool *pgxpool.Pool) *CounterStore { return &CounterStore{Pool: pool} }

func PtsKey(userID int64) string        { return "pts_updates_ngen_" + strconv.FormatInt(userID, 10) }
func MessageBoxKey(userID int64) string { return "message_box_ngen_" + strconv.FormatInt(userID, 10) }
func SeqKey(authID int64) string        { return "seq_updates_ngen_" + strconv.FormatInt(authID, 10) }

func counterLimit(key string) int64 {
	for _, prefix := range []string{
		"pts_updates_ngen_", "qts_updates_ngen_", "seq_updates_ngen_",
		"message_box_ngen_", "message_data_ngen_", "channel_message_box_ngen_",
		"channel_pts_updates_ngen_", "scheduled_ngen_", "bot_updates_ngen_",
		"story_ngen_", "channel_story_ngen_",
	} {
		if strings.HasPrefix(key, prefix) {
			return math.MaxInt32
		}
	}
	return math.MaxInt64
}

// NextOn holds the counter row until the caller commits its mutation. Its
// increment is rolled back with message, update, and delivery-intent writes.
func NextOn(ctx context.Context, tx pgx.Tx, key string, delta int64) (int64, error) {
	if tx == nil || strings.TrimSpace(key) == "" || delta <= 0 {
		return 0, ErrInvalidCounter
	}
	limit := counterLimit(key)
	if delta > limit {
		return 0, ErrCounterOverflow
	}
	var value int64
	err := tx.QueryRow(ctx, `INSERT INTO idgen_counters (key, value)
	 VALUES ($1,$2)
	 ON CONFLICT (key) DO UPDATE SET value=idgen_counters.value+EXCLUDED.value, updated_at=now()
	 WHERE idgen_counters.value<=$3
	 RETURNING value`, key, delta, limit-delta).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrCounterOverflow
	}
	return value, err
}

func (s *CounterStore) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if s == nil || s.Pool == nil {
		return errors.New("idgen: PostgreSQL counter store is not configured")
	}
	return postgres.WithTx(ctx, s.Pool, pgx.TxOptions{}, fn)
}

func (s *CounterStore) Next(ctx context.Context, key string, delta int64) (int64, error) {
	var value int64
	err := s.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		value, err = NextOn(ctx, tx, key, delta)
		return err
	})
	if err != nil {
		return 0, err
	}
	return value, nil
}

// Current does not initialize a row or advance an unused sequence.
func (s *CounterStore) Current(ctx context.Context, key string) (int64, error) {
	if s == nil || s.Pool == nil {
		return 0, errors.New("idgen: PostgreSQL counter store is not configured")
	}
	if strings.TrimSpace(key) == "" {
		return 0, ErrInvalidCounter
	}
	var value int64
	err := s.Pool.QueryRow(ctx, `SELECT value FROM idgen_counters WHERE key=$1`, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return value, err
}

func (s *CounterStore) CurrentList(ctx context.Context, keys []string) ([]int64, error) {
	if s == nil || s.Pool == nil {
		return nil, errors.New("idgen: PostgreSQL counter store is not configured")
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return nil, ErrInvalidCounter
		}
	}
	rows, err := s.Pool.Query(ctx, `SELECT key, value FROM idgen_counters WHERE key=ANY($1::text[])`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make(map[string]int64, len(keys))
	for rows.Next() {
		var key string
		var value int64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]int64, len(keys))
	for i, key := range keys {
		result[i] = values[key]
	}
	return result, nil
}

func (s *CounterStore) Set(ctx context.Context, key string, value int64) error {
	if strings.TrimSpace(key) == "" || value < 0 {
		return ErrInvalidCounter
	}
	if value > counterLimit(key) {
		return ErrCounterOverflow
	}
	return s.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO idgen_counters (key, value) VALUES ($1,$2)
	 ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, key, value)
		return err
	})
}
