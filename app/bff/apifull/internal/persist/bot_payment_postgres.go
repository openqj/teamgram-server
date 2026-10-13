package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/providercontract"
)

const (
	BotPaymentOutboxEventShippingQuery    = "shipping_query"
	BotPaymentOutboxEventPrecheckoutQuery = "precheckout_query"
	BotPaymentOutboxEventShipping         = BotPaymentStageShippingAnswered
	BotPaymentOutboxEventPrecheckout      = BotPaymentStagePrecheckoutAnswered
	BotPaymentOutboxStatePending          = "pending"
	BotPaymentOutboxStateInflight         = "inflight"
	BotPaymentOutboxStateComplete         = "complete"
)

const botPaymentOutboxLease = 30 * time.Second

var (
	ErrBotPaymentOutboxNotFound         = errors.New("apifull: bot payment outbox item not found")
	ErrBotPaymentOutboxState            = errors.New("apifull: bot payment outbox item is not claimable")
	ErrBotPaymentOutboxRequiresPostgres = errors.New("apifull: bot payment delivery requires PostgreSQL")
)

// BotPaymentOutbox is the typed delivery intent emitted when a bot answers a
// shipping or precheckout query. The payload is a canonical JSON object and
// remains available until a worker explicitly completes the item.
type BotPaymentOutbox struct {
	ID            int64
	QueryID       int64
	BotUserID     int64
	BuyerUserID   int64
	EventType     string
	ClaimToken    string
	Payload       json.RawMessage
	State         string
	Attempts      int
	NextAttemptAt time.Time
	LeaseUntil    *time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}

func botPaymentPostgresStore() (*postgresStore, bool) {
	store, ok := Default.(*postgresStore)
	return store, ok && store != nil && store.db != nil
}

func validBotPaymentOutboxEvent(eventType string) bool {
	switch eventType {
	case BotPaymentOutboxEventShippingQuery, BotPaymentOutboxEventPrecheckoutQuery,
		BotPaymentOutboxEventShipping, BotPaymentOutboxEventPrecheckout:
		return true
	default:
		return false
	}
}

func validBotPaymentDeliveryEvent(eventType string, stage string) bool {
	switch eventType {
	case BotPaymentOutboxEventShippingQuery:
		return stage == BotPaymentStageShippingPending
	case BotPaymentOutboxEventPrecheckoutQuery:
		return stage == BotPaymentStagePrecheckoutPending
	default:
		return false
	}
}

func botPaymentQueryDeliveryPayload(query BotPaymentQuery) any {
	return struct {
		QueryID      int64           `json:"query_id"`
		BotUserID    int64           `json:"bot_user_id"`
		BuyerUserID  int64           `json:"buyer_user_id"`
		QueryPayload json.RawMessage `json:"query_payload,omitempty"`
		Currency     string          `json:"currency,omitempty"`
		Amount       int64           `json:"amount"`
	}{query.QueryID, query.BotUserID, query.BuyerUserID, json.RawMessage(query.Payload), query.Currency, query.Amount}
}

func createBotPaymentQueryPostgres(store *postgresStore, query BotPaymentQuery, deliveryEvent string) error {
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockBotPaymentQueryTx(tx, query.QueryID); err != nil {
		return err
	}
	payload := query.Payload
	if payload == nil {
		payload = []byte{}
	}
	expiresAt := time.Unix(query.ExpiresAt, 0).UTC()
	_, err = tx.Exec(`INSERT INTO apifull_bot_payment_query
		(query_id, bot_user_id, buyer_user_id, stage, payload, currency, amount,
		 shipping_error, shipping_options, precheckout_success, precheckout_error,
		 expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
		ON CONFLICT (query_id) DO NOTHING`,
		query.QueryID, query.BotUserID, query.BuyerUserID, query.Stage,
		payload, strings.TrimSpace(query.Currency), query.Amount,
		query.ShippingError, query.ShippingOptions, nullableBool(query.PrecheckoutSuccess),
		query.PrecheckoutError, expiresAt)
	if err != nil {
		return err
	}
	var current BotPaymentQuery
	if err = scanBotPaymentQuery(tx.QueryRow(`SELECT query_id, bot_user_id, buyer_user_id,
		stage, payload, currency, amount, shipping_error, shipping_options,
		precheckout_success, precheckout_error, expires_at, updated_at
		FROM apifull_bot_payment_query WHERE query_id=$1 FOR UPDATE`, query.QueryID), &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBotPaymentQueryNotFound
		}
		return err
	}
	if !sameBotPaymentQueryIdentity(current, query) {
		return ErrBotPaymentQueryConflict
	}
	if deliveryEvent != "" {
		if !validBotPaymentDeliveryEvent(deliveryEvent, current.Stage) {
			return ErrBotPaymentQueryState
		}
		if err = enqueueBotPaymentOutboxTx(tx, current, deliveryEvent, botPaymentQueryDeliveryPayload(current)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func sameBotPaymentQueryIdentity(current, requested BotPaymentQuery) bool {
	if current.BotUserID != requested.BotUserID || current.BuyerUserID != requested.BuyerUserID {
		return false
	}
	if requested.Currency != "" && current.Currency != strings.TrimSpace(requested.Currency) {
		return false
	}
	if requested.Amount != 0 && current.Amount != requested.Amount {
		return false
	}
	if len(requested.Payload) > 0 && !jsonEqual(current.Payload, requested.Payload) {
		return false
	}
	return true
}

func advanceBotPaymentQueryPostgres(store *postgresStore, queryID, botUserID int64, stage string, payload []byte, currency string, amount int64, enqueueDelivery bool) error {
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockBotPaymentQueryTx(tx, queryID); err != nil {
		return err
	}
	var query BotPaymentQuery
	if err = scanBotPaymentQuery(tx.QueryRow(`SELECT query_id, bot_user_id, buyer_user_id,
		stage, payload, currency, amount, shipping_error, shipping_options,
		precheckout_success, precheckout_error, expires_at, updated_at
		FROM apifull_bot_payment_query WHERE query_id=$1 FOR UPDATE`, queryID), &query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBotPaymentQueryNotFound
		}
		return err
	}
	if query.BotUserID != botUserID {
		return ErrBotPaymentQueryOwner
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return ErrBotPaymentQueryExpired
	}
	if query.Stage == BotPaymentStagePrecheckoutPending || query.Stage == BotPaymentStagePrecheckoutAnswered {
		if len(payload) > 0 && !jsonEqual(query.Payload, payload) {
			return ErrBotPaymentQueryState
		}
		if enqueueDelivery && (query.Currency != strings.TrimSpace(currency) || query.Amount != amount) {
			return ErrBotPaymentQueryState
		}
		if enqueueDelivery && query.Stage == BotPaymentStagePrecheckoutPending {
			if err = enqueueBotPaymentOutboxTx(tx, query, BotPaymentOutboxEventPrecheckoutQuery, botPaymentQueryDeliveryPayload(query)); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	if query.Stage != BotPaymentStageShippingAnswered || stage != BotPaymentStagePrecheckoutPending {
		return ErrBotPaymentQueryState
	}
	if enqueueDelivery {
		query.Payload = append(query.Payload[:0], payload...)
		query.Currency = strings.TrimSpace(currency)
		query.Amount = amount
		_, err = tx.Exec(`UPDATE apifull_bot_payment_query
			SET stage=$1, payload=$2, currency=$3, amount=$4, updated_at=CURRENT_TIMESTAMP
			WHERE query_id=$5`, stage, query.Payload, query.Currency, query.Amount, queryID)
	} else {
		_, err = tx.Exec(`UPDATE apifull_bot_payment_query SET stage=$1, updated_at=CURRENT_TIMESTAMP WHERE query_id=$2`, stage, queryID)
	}
	if err != nil {
		return err
	}
	if enqueueDelivery {
		if err = enqueueBotPaymentOutboxTx(tx, query, BotPaymentOutboxEventPrecheckoutQuery, botPaymentQueryDeliveryPayload(query)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func setBotShippingResultsPostgres(store *postgresStore, botUserID, queryID int64, errorText string, options []byte) error {
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockBotPaymentQueryTx(tx, queryID); err != nil {
		return err
	}
	var query BotPaymentQuery
	if err = scanBotPaymentQuery(tx.QueryRow(`SELECT query_id, bot_user_id, buyer_user_id,
		stage, payload, currency, amount, shipping_error, shipping_options,
		precheckout_success, precheckout_error, expires_at, updated_at
		FROM apifull_bot_payment_query WHERE query_id=$1 FOR UPDATE`, queryID), &query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBotPaymentQueryNotFound
		}
		return err
	}
	if query.BotUserID != botUserID {
		return ErrBotPaymentQueryOwner
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return ErrBotPaymentQueryExpired
	}
	if query.Stage == BotPaymentStageShippingAnswered {
		if query.ShippingError == errorText && jsonEqual(query.ShippingOptions, options) {
			return tx.Commit()
		}
		return ErrBotPaymentQueryState
	}
	if query.Stage != BotPaymentStageShippingPending {
		return ErrBotPaymentQueryState
	}
	if _, err = tx.Exec(`UPDATE apifull_bot_payment_query
		SET stage=$1, shipping_error=$2, shipping_options=$3::jsonb, updated_at=CURRENT_TIMESTAMP
		WHERE query_id=$4`, BotPaymentStageShippingAnswered, errorText, options, queryID); err != nil {
		return err
	}
	payload := struct {
		QueryID         int64           `json:"query_id"`
		BotUserID       int64           `json:"bot_user_id"`
		BuyerUserID     int64           `json:"buyer_user_id"`
		QueryPayload    json.RawMessage `json:"query_payload,omitempty"`
		Currency        string          `json:"currency,omitempty"`
		Amount          int64           `json:"amount"`
		Error           string          `json:"error,omitempty"`
		ShippingOptions json.RawMessage `json:"shipping_options"`
	}{query.QueryID, query.BotUserID, query.BuyerUserID, json.RawMessage(query.Payload), query.Currency, query.Amount, errorText, json.RawMessage(options)}
	if err = enqueueBotPaymentOutboxTx(tx, query, BotPaymentOutboxEventShipping, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func setBotPrecheckoutResultsPostgres(store *postgresStore, botUserID, queryID int64, success bool, errorText string) error {
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockBotPaymentQueryTx(tx, queryID); err != nil {
		return err
	}
	var query BotPaymentQuery
	if err = scanBotPaymentQuery(tx.QueryRow(`SELECT query_id, bot_user_id, buyer_user_id,
		stage, payload, currency, amount, shipping_error, shipping_options,
		precheckout_success, precheckout_error, expires_at, updated_at
		FROM apifull_bot_payment_query WHERE query_id=$1 FOR UPDATE`, queryID), &query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBotPaymentQueryNotFound
		}
		return err
	}
	if query.BotUserID != botUserID {
		return ErrBotPaymentQueryOwner
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return ErrBotPaymentQueryExpired
	}
	if query.Stage == BotPaymentStagePrecheckoutAnswered {
		if query.PrecheckoutSuccess != nil && *query.PrecheckoutSuccess == success && query.PrecheckoutError == errorText {
			return tx.Commit()
		}
		return ErrBotPaymentQueryState
	}
	if query.Stage != BotPaymentStagePrecheckoutPending {
		return ErrBotPaymentQueryState
	}
	if _, err = tx.Exec(`UPDATE apifull_bot_payment_query
		SET stage=$1, precheckout_success=$2, precheckout_error=$3, updated_at=CURRENT_TIMESTAMP
		WHERE query_id=$4`, BotPaymentStagePrecheckoutAnswered, success, errorText, queryID); err != nil {
		return err
	}
	payload := struct {
		QueryID      int64           `json:"query_id"`
		BotUserID    int64           `json:"bot_user_id"`
		BuyerUserID  int64           `json:"buyer_user_id"`
		QueryPayload json.RawMessage `json:"query_payload,omitempty"`
		Currency     string          `json:"currency,omitempty"`
		Amount       int64           `json:"amount"`
		Success      bool            `json:"success"`
		Error        string          `json:"error,omitempty"`
	}{query.QueryID, query.BotUserID, query.BuyerUserID, json.RawMessage(query.Payload), query.Currency, query.Amount, success, errorText}
	if err = enqueueBotPaymentOutboxTx(tx, query, BotPaymentOutboxEventPrecheckout, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func loadBotPaymentQueryPostgres(store *postgresStore, queryID int64) (BotPaymentQuery, bool, error) {
	var query BotPaymentQuery
	err := scanBotPaymentQuery(store.db.QueryRow(`SELECT query_id, bot_user_id, buyer_user_id,
		stage, payload, currency, amount, shipping_error, shipping_options,
		precheckout_success, precheckout_error, expires_at, updated_at
		FROM apifull_bot_payment_query WHERE query_id=$1`, queryID), &query)
	if errors.Is(err, sql.ErrNoRows) {
		return BotPaymentQuery{}, false, nil
	}
	if err != nil {
		return BotPaymentQuery{}, false, err
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return BotPaymentQuery{}, false, nil
	}
	return query, true, nil
}

func lockBotPaymentQueryTx(tx *sql.Tx, queryID int64) error {
	_, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "apifull-bot-payment-query:"+itoa(queryID))
	return err
}

func itoa(value int64) string {
	// strconv.FormatInt is kept behind this tiny helper so SQL lock keys remain
	// identical across all bot-payment operations.
	return strconv.FormatInt(value, 10)
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func jsonEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && jsonEqualValue(a, b)
}

func jsonEqualValue(left, right any) bool {
	a, ok := left.(map[string]any)
	b, okB := right.(map[string]any)
	if ok && okB {
		if len(a) != len(b) {
			return false
		}
		for key, value := range a {
			other, exists := b[key]
			if !exists || !jsonEqualValue(value, other) {
				return false
			}
		}
		return true
	}
	aa, ok := left.([]any)
	bb, okB := right.([]any)
	if ok && okB {
		if len(aa) != len(bb) {
			return false
		}
		for i := range aa {
			if !jsonEqualValue(aa[i], bb[i]) {
				return false
			}
		}
		return true
	}
	return left == right
}

type botPaymentQueryScanner interface {
	Scan(dest ...any) error
}

func scanBotPaymentQuery(scanner botPaymentQueryScanner, query *BotPaymentQuery) error {
	var payload, options []byte
	var precheckout sql.NullBool
	var expiresAt, updatedAt time.Time
	err := scanner.Scan(&query.QueryID, &query.BotUserID, &query.BuyerUserID,
		&query.Stage, &payload, &query.Currency, &query.Amount, &query.ShippingError,
		&options, &precheckout, &query.PrecheckoutError, &expiresAt, &updatedAt)
	if err != nil {
		return err
	}
	if !validBotPaymentStage(query.Stage) || query.QueryID <= 0 || query.BotUserID <= 0 || query.BuyerUserID <= 0 || query.Amount < 0 || !json.Valid(options) {
		return ErrBotPaymentQueryCorrupt
	}
	query.Payload = append(query.Payload[:0], payload...)
	query.ShippingOptions = append(query.ShippingOptions[:0], options...)
	if precheckout.Valid {
		value := precheckout.Bool
		query.PrecheckoutSuccess = &value
	}
	query.ExpiresAt = expiresAt.Unix()
	query.UpdatedAt = updatedAt.Unix()
	return nil
}

func enqueueBotPaymentOutboxTx(tx *sql.Tx, query BotPaymentQuery, eventType string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil || !json.Valid(encoded) {
		return ErrBotPaymentQueryCorrupt
	}
	_, err = tx.Exec(`INSERT INTO apifull_bot_payment_outbox
		(query_id, bot_user_id, buyer_user_id, event_type, payload, state,
		 attempts, next_attempt_at, last_error, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,'pending',0,CURRENT_TIMESTAMP,'',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
		ON CONFLICT (query_id, event_type) DO NOTHING`,
		query.QueryID, query.BotUserID, query.BuyerUserID, eventType, encoded)
	return err
}

// ClaimBotPaymentOutbox leases due callback results for one worker. Expired
// inflight leases are reclaimed after a process restart.
func ClaimBotPaymentOutbox(ctx context.Context, limit int) ([]BotPaymentOutbox, error) {
	store, ok := botPaymentPostgresStore()
	if !ok {
		return nil, errors.New("apifull: bot payment outbox requires PostgreSQL")
	}
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("apifull: invalid bot payment outbox limit")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id, query_id, bot_user_id, buyer_user_id,
		event_type, claim_token, payload, state, attempts, next_attempt_at, lease_until,
		last_error, created_at, updated_at, completed_at
		FROM apifull_bot_payment_outbox
		WHERE (state='pending' AND next_attempt_at <= CURRENT_TIMESTAMP)
		   OR (state='inflight' AND (lease_until IS NULL OR lease_until <= CURRENT_TIMESTAMP))
		ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	claimed := make([]BotPaymentOutbox, 0, limit)
	for rows.Next() {
		item, err := scanBotPaymentOutbox(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		claimed = append(claimed, item)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range claimed {
		leaseUntil := time.Now().UTC().Add(botPaymentOutboxLease)
		var claimToken string
		if err = tx.QueryRowContext(ctx, `UPDATE apifull_bot_payment_outbox SET
			state='inflight', attempts=attempts+1, claim_token=gen_random_uuid(), lease_until=$1,
			next_attempt_at=$1, updated_at=CURRENT_TIMESTAMP WHERE id=$2
			RETURNING claim_token::text`, leaseUntil, claimed[i].ID).Scan(&claimToken); err != nil {
			return nil, err
		}
		claimed[i].State = BotPaymentOutboxStateInflight
		claimed[i].ClaimToken = claimToken
		claimed[i].Attempts++
		claimed[i].LeaseUntil = &leaseUntil
		claimed[i].NextAttemptAt = leaseUntil
		claimed[i].UpdatedAt = time.Now().UTC()
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func CompleteBotPaymentOutbox(ctx context.Context, id int64) error {
	store, ok := botPaymentPostgresStore()
	if !ok || id <= 0 {
		return ErrBotPaymentOutboxNotFound
	}
	var state string
	var claimToken sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT state, claim_token::text FROM apifull_bot_payment_outbox WHERE id=$1`, id).Scan(&state, &claimToken); errors.Is(err, sql.ErrNoRows) {
		return ErrBotPaymentOutboxNotFound
	} else if err != nil {
		return err
	}
	if state == BotPaymentOutboxStateComplete {
		return nil
	}
	if !claimToken.Valid || strings.TrimSpace(claimToken.String) == "" {
		return ErrBotPaymentOutboxState
	}
	return CompleteBotPaymentOutboxWithToken(ctx, id, claimToken.String)
}

// CompleteBotPaymentOutboxWithToken completes an item only while the supplied
// worker lease is still current. A stale worker receives ErrBotPaymentOutboxState.
func CompleteBotPaymentOutboxWithToken(ctx context.Context, id int64, claimToken string) error {
	store, ok := botPaymentPostgresStore()
	if !ok || id <= 0 {
		return ErrBotPaymentOutboxNotFound
	}
	if strings.TrimSpace(claimToken) == "" {
		return ErrBotPaymentOutboxState
	}
	result, err := store.db.ExecContext(ctx, `UPDATE apifull_bot_payment_outbox
		SET state='complete', completed_at=CURRENT_TIMESTAMP, lease_until=NULL,
			claim_token=NULL, last_error='', updated_at=CURRENT_TIMESTAMP
		WHERE id=$1 AND state='inflight' AND claim_token=$2::uuid`, id, claimToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	var state string
	err = store.db.QueryRowContext(ctx, `SELECT state FROM apifull_bot_payment_outbox WHERE id=$1`, id).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBotPaymentOutboxNotFound
	}
	if err != nil {
		return err
	}
	if state == BotPaymentOutboxStateComplete {
		return nil
	}
	return ErrBotPaymentOutboxState
}

func RetryBotPaymentOutbox(ctx context.Context, id int64, deliveryErr error) error {
	store, ok := botPaymentPostgresStore()
	if !ok || id <= 0 {
		return ErrBotPaymentOutboxNotFound
	}
	var state string
	var claimToken sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT state, claim_token::text FROM apifull_bot_payment_outbox WHERE id=$1`, id).Scan(&state, &claimToken); errors.Is(err, sql.ErrNoRows) {
		return ErrBotPaymentOutboxNotFound
	} else if err != nil {
		return err
	}
	if state == BotPaymentOutboxStateComplete {
		return nil
	}
	if !claimToken.Valid || strings.TrimSpace(claimToken.String) == "" {
		return ErrBotPaymentOutboxState
	}
	return RetryBotPaymentOutboxWithToken(ctx, id, claimToken.String, deliveryErr)
}

// RetryBotPaymentOutboxWithToken releases an item only while the supplied
// worker lease is current, preventing a stale worker from delaying a newer
// attempt.
func RetryBotPaymentOutboxWithToken(ctx context.Context, id int64, claimToken string, deliveryErr error) error {
	store, ok := botPaymentPostgresStore()
	if !ok || id <= 0 {
		return ErrBotPaymentOutboxNotFound
	}
	if strings.TrimSpace(claimToken) == "" {
		return ErrBotPaymentOutboxState
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var state, currentToken string
	var attempts int
	if err = tx.QueryRowContext(ctx, `SELECT state, attempts, COALESCE(claim_token::text, '') FROM apifull_bot_payment_outbox WHERE id=$1 FOR UPDATE`, id).Scan(&state, &attempts, &currentToken); errors.Is(err, sql.ErrNoRows) {
		return ErrBotPaymentOutboxNotFound
	} else if err != nil {
		return err
	}
	if state == BotPaymentOutboxStateComplete {
		return tx.Commit()
	}
	if currentToken != claimToken {
		return ErrBotPaymentOutboxState
	}
	if state != BotPaymentOutboxStateInflight {
		return ErrBotPaymentOutboxState
	}
	detail := "bot payment callback delivery failed"
	if deliveryErr != nil && strings.TrimSpace(deliveryErr.Error()) != "" {
		detail = deliveryErr.Error()
	}
	if len(detail) > 1024 {
		detail = detail[:1024]
	}
	detail = strings.ToValidUTF8(detail, "")
	shift := attempts
	if shift < 1 {
		shift = 1
	}
	if shift > 8 {
		shift = 8
	}
	next := time.Now().UTC().Add(time.Second << shift)
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_bot_payment_outbox SET
		state='pending', lease_until=NULL, claim_token=NULL, next_attempt_at=$1, last_error=$2,
		updated_at=CURRENT_TIMESTAMP WHERE id=$3 AND state='inflight' AND claim_token=$4::uuid`, next, detail, id, claimToken); err != nil {
		return err
	}
	return tx.Commit()
}

func scanBotPaymentOutbox(scanner botPaymentQueryScanner) (BotPaymentOutbox, error) {
	var item BotPaymentOutbox
	var payload []byte
	var leaseUntil, completedAt sql.NullTime
	var claimToken sql.NullString
	if err := scanner.Scan(&item.ID, &item.QueryID, &item.BotUserID, &item.BuyerUserID,
		&item.EventType, &claimToken, &payload, &item.State, &item.Attempts, &item.NextAttemptAt,
		&leaseUntil, &item.LastError, &item.CreatedAt, &item.UpdatedAt, &completedAt); err != nil {
		return BotPaymentOutbox{}, err
	}
	if claimToken.Valid {
		item.ClaimToken = claimToken.String
	}
	if item.ID <= 0 || item.QueryID <= 0 || item.BotUserID <= 0 || item.BuyerUserID <= 0 ||
		!validBotPaymentOutboxEvent(item.EventType) ||
		!providercontract.ValidDeliveryState(item.State) ||
		item.Attempts < 0 || !json.Valid(payload) {
		return BotPaymentOutbox{}, ErrBotPaymentOutboxState
	}
	item.Payload = append(item.Payload[:0], payload...)
	if leaseUntil.Valid {
		value := leaseUntil.Time
		item.LeaseUntil = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		item.CompletedAt = &value
	}
	return item, nil
}
