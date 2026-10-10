package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var (
	ErrSponsoredEventConflict       = errors.New("sponsored event payload conflict")
	ErrSponsoredChannelMissing      = errors.New("sponsored channel missing")
	ErrSponsoredChannelAccessHash   = errors.New("sponsored channel access hash invalid")
	ErrSponsoredChannelUnauthorized = errors.New("sponsored channel admin required")
)

func sponsoredJSONEqual(left, right []byte) bool {
	var leftValue, rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func sponsoredContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func sponsoredPostgres() (*postgresStore, bool) {
	store, ok := Default.(*postgresStore)
	return store, ok && store != nil && store.db != nil
}

// SetSponsoredMessagesEnabled persists the account preference transactionally.
func SetSponsoredMessagesEnabled(ctx context.Context, userID int64, enabled bool) error {
	if userID <= 0 {
		return errors.New("sponsored user id is required")
	}
	if store, ok := sponsoredPostgres(); ok {
		tx, err := store.db.BeginTx(sponsoredContext(ctx), nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err = tx.ExecContext(sponsoredContext(ctx), `INSERT INTO apifull_sponsored_user_setting
			(user_id, enabled, updated_at) VALUES ($1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id) DO UPDATE SET enabled=EXCLUDED.enabled,
			updated_at=CURRENT_TIMESTAMP`, userID, enabled); err != nil {
			return err
		}
		return tx.Commit()
	}
	return Update(fmt.Sprintf("sponsored:%d:enabled", userID), func(string) (string, error) {
		if enabled {
			return "true", nil
		}
		return "false", nil
	})
}

// RecordSponsoredEvent stores a retry-safe analytics event. A repeated event
// with the same identity is accepted only when its payload is identical.
func RecordSponsoredEvent(ctx context.Context, userID int64, eventType string, randomID []byte, payload any) error {
	if userID <= 0 || strings.TrimSpace(eventType) == "" || len(randomID) == 0 {
		return errors.New("sponsored event identity is required")
	}
	raw := []byte(`{}`)
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	ctx = sponsoredContext(ctx)
	if store, ok := sponsoredPostgres(); ok {
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err = tx.ExecContext(ctx, `INSERT INTO apifull_sponsored_event
			(user_id, event_type, random_id, payload, created_at, updated_at)
			VALUES ($1, $2, $3, $4::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id, event_type, random_id) DO NOTHING`, userID, eventType, randomID, raw); err != nil {
			return err
		}
		var stored []byte
		if err = tx.QueryRowContext(ctx, `SELECT payload FROM apifull_sponsored_event
			WHERE user_id=$1 AND event_type=$2 AND random_id=$3 FOR UPDATE`, userID, eventType, randomID).Scan(&stored); err != nil {
			return err
		}
		if !sponsoredJSONEqual(stored, raw) {
			return ErrSponsoredEventConflict
		}
		return tx.Commit()
	}
	return Default.Set(fmt.Sprintf("sponsored:%d:event:%s:%x", userID, eventType, randomID), string(raw))
}

// SetSponsoredChannelRestriction authorizes the channel creator and updates
// policy in the same row-locked transaction as the write.
func SetSponsoredChannelRestriction(ctx context.Context, actorID, channelID, accessHash int64, restricted bool) error {
	if actorID <= 0 || channelID <= 0 || accessHash == 0 {
		return ErrSponsoredChannelAccessHash
	}
	ctx = sponsoredContext(ctx)
	if store, ok := sponsoredPostgres(); ok {
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		var storedHash, creator int64
		err = tx.QueryRowContext(ctx, `SELECT access_hash, creator_user_id FROM apifull_channel
			WHERE id=$1 FOR UPDATE`, channelID).Scan(&storedHash, &creator)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSponsoredChannelMissing
		}
		if err != nil {
			return err
		}
		if storedHash != accessHash {
			return ErrSponsoredChannelAccessHash
		}
		if creator != actorID {
			return ErrSponsoredChannelUnauthorized
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO apifull_sponsored_channel_policy
			(channel_id, actor_user_id, restricted, updated_at) VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
			ON CONFLICT (channel_id) DO UPDATE SET actor_user_id=EXCLUDED.actor_user_id,
			restricted=EXCLUDED.restricted, updated_at=CURRENT_TIMESTAMP`, channelID, actorID, restricted); err != nil {
			return err
		}
		return tx.Commit()
	}
	return Default.Set(fmt.Sprintf("sponsored:channel:%d:restricted", channelID), fmt.Sprintf("%t", restricted))
}
