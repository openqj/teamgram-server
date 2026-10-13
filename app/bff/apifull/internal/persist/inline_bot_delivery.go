package persist

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	InlineBotQueryStatePending  = "pending"
	InlineBotQueryStateInflight = "inflight"
	InlineBotQueryStateComplete = "complete"
	inlineBotQueryLease         = 30 * time.Second
)

var ErrInlineQueryState = errors.New("apifull: invalid inline bot query delivery state")

func validInlineBotQueryDeliveryState(state string) bool {
	switch state {
	case InlineBotQueryStatePending, InlineBotQueryStateInflight, InlineBotQueryStateComplete:
		return true
	default:
		return false
	}
}

// ClaimInlineBotQueries leases due inline queries in one PostgreSQL transaction.
// The in-memory implementation is retained for focused unit tests; production
// APIFull always uses the PostgreSQL path.
func ClaimInlineBotQueries(ctx context.Context, limit int) ([]InlineBotQuery, error) {
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("apifull: invalid inline bot query claim limit")
	}
	if store, ok := Default.(*postgresStore); ok {
		return store.claimInlineBotQueries(ctx, limit)
	}
	if store, ok := Default.(*mem); ok {
		return claimInlineBotQueriesMemory(store, limit)
	}
	return nil, errors.New("apifull: inline bot query delivery requires PostgreSQL")
}

func (s *postgresStore) claimInlineBotQueries(ctx context.Context, limit int) ([]InlineBotQuery, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT k, v FROM apifull_kv
		WHERE k LIKE 'inline-bot-query:%' ORDER BY k FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		key   string
		items []InlineBotQuery
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var key, raw string
		if err = rows.Scan(&key, &raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		items, decodeErr := decodeInlineQueries(raw)
		if decodeErr != nil {
			_ = rows.Close()
			return nil, decodeErr
		}
		candidates = append(candidates, candidate{key: key, items: items})
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	claimed := make([]InlineBotQuery, 0, limit)
	for _, candidate := range candidates {
		changed := false
		for i := range candidate.items {
			if len(claimed) >= limit {
				break
			}
			item := &candidate.items[i]
			if !inlineBotQueryDue(*item, now) {
				continue
			}
			if item.ExpiresAt > 0 && item.ExpiresAt <= now.Unix() {
				item.DeliveryState = InlineBotQueryStateComplete
				item.ClaimToken = ""
				item.LeaseUntil = 0
				item.DeliveredAt = now.Unix()
				item.UpdatedAt = now.Unix()
				changed = true
				continue
			}
			token, tokenErr := newInlineBotClaimToken()
			if tokenErr != nil {
				return nil, tokenErr
			}
			item.DeliveryState = InlineBotQueryStateInflight
			item.ClaimToken = token
			item.LeaseUntil = now.Add(inlineBotQueryLease).Unix()
			item.RetryAt = item.LeaseUntil
			item.Attempts++
			item.UpdatedAt = now.Unix()
			claimedItem := *item
			claimedItem.Results = cloneRawMessages(item.Results)
			claimed = append(claimed, claimedItem)
			changed = true
		}
		if !changed {
			continue
		}
		encoded, encodeErr := encodeInlineQueries(candidate.items)
		if encodeErr != nil {
			return nil, encodeErr
		}
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_kv SET v=$1 WHERE k=$2`, encoded, candidate.key); err != nil {
			return nil, err
		}
		if len(claimed) >= limit {
			break
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func claimInlineBotQueriesMemory(store *mem, limit int) ([]InlineBotQuery, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := time.Now().UTC()
	claimed := make([]InlineBotQuery, 0, limit)
	for key, raw := range store.m {
		if !strings.HasPrefix(key, "inline-bot-query:") {
			continue
		}
		items, err := decodeInlineQueries(raw)
		if err != nil {
			return nil, err
		}
		changed := false
		for i := range items {
			if len(claimed) >= limit {
				break
			}
			item := &items[i]
			if !inlineBotQueryDue(*item, now) {
				continue
			}
			if item.ExpiresAt > 0 && item.ExpiresAt <= now.Unix() {
				item.DeliveryState = InlineBotQueryStateComplete
				item.ClaimToken = ""
				item.LeaseUntil = 0
				item.DeliveredAt = now.Unix()
				item.UpdatedAt = now.Unix()
				changed = true
				continue
			}
			token, err := newInlineBotClaimToken()
			if err != nil {
				return nil, err
			}
			item.DeliveryState = InlineBotQueryStateInflight
			item.ClaimToken = token
			item.LeaseUntil = now.Add(inlineBotQueryLease).Unix()
			item.RetryAt = item.LeaseUntil
			item.Attempts++
			item.UpdatedAt = now.Unix()
			copy := *item
			copy.Results = cloneRawMessages(item.Results)
			claimed = append(claimed, copy)
			changed = true
		}
		if changed {
			encoded, err := encodeInlineQueries(items)
			if err != nil {
				return nil, err
			}
			store.m[key] = encoded
		}
		if len(claimed) >= limit {
			break
		}
	}
	return claimed, nil
}

func inlineBotQueryDue(item InlineBotQuery, now time.Time) bool {
	if item.DeliveryState == "" {
		item.DeliveryState = InlineBotQueryStatePending
	}
	if item.DeliveryState == InlineBotQueryStateComplete {
		return false
	}
	if item.DeliveryState == InlineBotQueryStateInflight {
		return item.LeaseUntil <= now.Unix()
	}
	return item.RetryAt <= 0 || item.RetryAt <= now.Unix()
}

func CompleteInlineBotQuery(ctx context.Context, queryID int64, claimToken string) error {
	if queryID <= 0 || strings.TrimSpace(claimToken) == "" {
		return ErrInlineQueryState
	}
	if store, ok := Default.(*postgresStore); ok {
		return store.completeInlineBotQuery(ctx, queryID, claimToken)
	}
	if store, ok := Default.(*mem); ok {
		return completeInlineBotQueryMemory(store, queryID, claimToken)
	}
	return errors.New("apifull: inline bot query delivery requires PostgreSQL")
}

func (s *postgresStore) completeInlineBotQuery(ctx context.Context, queryID int64, claimToken string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT k, v FROM apifull_kv
		WHERE k LIKE 'inline-bot-query:%' FOR UPDATE`)
	if err != nil {
		return err
	}
	found, stateMismatch := false, false
	type update struct {
		key   string
		value string
	}
	updates := make([]update, 0, 1)
	for rows.Next() {
		var key, raw string
		if err = rows.Scan(&key, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		items, decodeErr := decodeInlineQueries(raw)
		if decodeErr != nil {
			_ = rows.Close()
			return decodeErr
		}
		changed := false
		for i := range items {
			item := &items[i]
			if item.QueryID != queryID {
				continue
			}
			found = true
			if item.DeliveryState == InlineBotQueryStateComplete {
				continue
			}
			if item.DeliveryState != InlineBotQueryStateInflight || item.ClaimToken != claimToken {
				stateMismatch = true
				continue
			}
			item.DeliveryState = InlineBotQueryStateComplete
			item.ClaimToken = ""
			item.LeaseUntil = 0
			item.DeliveredAt = time.Now().UTC().Unix()
			item.UpdatedAt = item.DeliveredAt
			changed = true
		}
		if changed {
			encoded, encodeErr := encodeInlineQueries(items)
			if encodeErr != nil {
				_ = rows.Close()
				return encodeErr
			}
			updates = append(updates, update{key: key, value: encoded})
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if !found {
		return ErrInlineQueryNotFound
	}
	if stateMismatch {
		return ErrInlineQueryState
	}
	for _, item := range updates {
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_kv SET v=$1 WHERE k=$2`, item.value, item.key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func completeInlineBotQueryMemory(store *mem, queryID int64, claimToken string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	found, stateMismatch := false, false
	for key, raw := range store.m {
		if !strings.HasPrefix(key, "inline-bot-query:") {
			continue
		}
		items, err := decodeInlineQueries(raw)
		if err != nil {
			return err
		}
		changed := false
		for i := range items {
			item := &items[i]
			if item.QueryID != queryID {
				continue
			}
			found = true
			if item.DeliveryState == InlineBotQueryStateComplete {
				continue
			}
			if item.DeliveryState != InlineBotQueryStateInflight || item.ClaimToken != claimToken {
				stateMismatch = true
				continue
			}
			item.DeliveryState = InlineBotQueryStateComplete
			item.ClaimToken = ""
			item.LeaseUntil = 0
			item.DeliveredAt = time.Now().UTC().Unix()
			item.UpdatedAt = item.DeliveredAt
			changed = true
		}
		if changed {
			encoded, err := encodeInlineQueries(items)
			if err != nil {
				return err
			}
			store.m[key] = encoded
		}
	}
	if !found {
		return ErrInlineQueryNotFound
	}
	if stateMismatch {
		return ErrInlineQueryState
	}
	return nil
}

func RetryInlineBotQuery(ctx context.Context, queryID int64, claimToken string, deliveryErr error) error {
	if queryID <= 0 || strings.TrimSpace(claimToken) == "" {
		return ErrInlineQueryState
	}
	if store, ok := Default.(*postgresStore); ok {
		return store.retryInlineBotQuery(ctx, queryID, claimToken, deliveryErr)
	}
	if store, ok := Default.(*mem); ok {
		return retryInlineBotQueryMemory(store, queryID, claimToken, deliveryErr)
	}
	return errors.New("apifull: inline bot query delivery requires PostgreSQL")
}

func (s *postgresStore) retryInlineBotQuery(ctx context.Context, queryID int64, claimToken string, deliveryErr error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT k, v FROM apifull_kv
		WHERE k LIKE 'inline-bot-query:%' FOR UPDATE`)
	if err != nil {
		return err
	}
	found, stateMismatch := false, false
	now := time.Now().UTC()
	type update struct {
		key   string
		value string
	}
	updates := make([]update, 0, 1)
	for rows.Next() {
		var key, raw string
		if err = rows.Scan(&key, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		items, decodeErr := decodeInlineQueries(raw)
		if decodeErr != nil {
			_ = rows.Close()
			return decodeErr
		}
		changed := false
		for i := range items {
			item := &items[i]
			if item.QueryID != queryID {
				continue
			}
			found = true
			if item.DeliveryState == InlineBotQueryStateComplete {
				continue
			}
			if item.DeliveryState != InlineBotQueryStateInflight || item.ClaimToken != claimToken {
				stateMismatch = true
				continue
			}
			item.DeliveryState = InlineBotQueryStatePending
			item.ClaimToken = ""
			item.LeaseUntil = 0
			item.RetryAt = now.Add(inlineBotRetryDelay(item.Attempts)).Unix()
			item.LastError = inlineBotDeliveryError(deliveryErr)
			item.UpdatedAt = now.Unix()
			changed = true
		}
		if changed {
			encoded, encodeErr := encodeInlineQueries(items)
			if encodeErr != nil {
				_ = rows.Close()
				return encodeErr
			}
			updates = append(updates, update{key: key, value: encoded})
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if !found {
		return ErrInlineQueryNotFound
	}
	if stateMismatch {
		return ErrInlineQueryState
	}
	for _, item := range updates {
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_kv SET v=$1 WHERE k=$2`, item.value, item.key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func retryInlineBotQueryMemory(store *mem, queryID int64, claimToken string, deliveryErr error) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	found, stateMismatch := false, false
	now := time.Now().UTC()
	for key, raw := range store.m {
		if !strings.HasPrefix(key, "inline-bot-query:") {
			continue
		}
		items, err := decodeInlineQueries(raw)
		if err != nil {
			return err
		}
		changed := false
		for i := range items {
			item := &items[i]
			if item.QueryID != queryID {
				continue
			}
			found = true
			if item.DeliveryState == InlineBotQueryStateComplete {
				continue
			}
			if item.DeliveryState != InlineBotQueryStateInflight || item.ClaimToken != claimToken {
				stateMismatch = true
				continue
			}
			item.DeliveryState = InlineBotQueryStatePending
			item.ClaimToken = ""
			item.LeaseUntil = 0
			item.RetryAt = now.Add(inlineBotRetryDelay(item.Attempts)).Unix()
			item.LastError = inlineBotDeliveryError(deliveryErr)
			item.UpdatedAt = now.Unix()
			changed = true
		}
		if changed {
			encoded, err := encodeInlineQueries(items)
			if err != nil {
				return err
			}
			store.m[key] = encoded
		}
	}
	if !found {
		return ErrInlineQueryNotFound
	}
	if stateMismatch {
		return ErrInlineQueryState
	}
	return nil
}

func inlineBotRetryDelay(attempts int) time.Duration {
	shift := attempts
	if shift < 1 {
		shift = 1
	}
	if shift > 8 {
		shift = 8
	}
	return time.Duration(1<<shift) * time.Second
}

func inlineBotDeliveryError(err error) string {
	detail := "inline bot query delivery failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		detail = err.Error()
	}
	detail = strings.ToValidUTF8(detail, "")
	if len(detail) > 1024 {
		return detail[:1024]
	}
	return detail
}

func newInlineBotClaimToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
