package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	PaymentWebhookStatePending    = "pending"
	PaymentWebhookStateInflight   = "inflight"
	PaymentWebhookStateComplete   = "complete"
	PaymentWebhookStateFailed     = "failed"
	PaymentReconciliationPending  = "pending"
	PaymentReconciliationInflight = "inflight"
	PaymentReconciliationComplete = "complete"
	PaymentReconciliationFailed   = "failed"
)

var (
	ErrPaymentWebhookConflict    = errors.New("payment webhook event conflicts with an existing event")
	ErrPaymentSettlementNotFound = errors.New("payment settlement not found")
	ErrPaymentRecoveryClaimLost  = errors.New("payment recovery claim was replaced")
	ErrPaymentRecoveryInvalid    = errors.New("invalid payment recovery event")
	ErrPaymentReversalConflict   = errors.New("payment reversal conflicts with an existing reversal")
)

type PaymentWebhook struct {
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	Provider      string `json:"provider"`
	UserID        int64  `json:"user_id"`
	RequestKey    string `json:"request_key"`
	TransactionID string `json:"transaction_id"`
	Product       string `json:"product"`
	Months        int32  `json:"months"`
	Stars         int64  `json:"stars"`
	Reason        string `json:"reason"`
	OccurredAt    int64  `json:"occurred_at"`
	PayloadHash   string `json:"-"`
	Payload       []byte `json:"-"`
	State         string `json:"-"`
	Attempts      int32  `json:"-"`
	ClaimToken    string `json:"-"`
}

type PaymentReconciliation struct {
	Provider       string
	UserID         int64
	TransactionID  string
	RequestKey     string
	Product        string
	Months         int32
	Stars          int64
	ProviderStatus string
	State          string
	Attempts       int32
	ClaimToken     string
}

type PaymentReversalResult struct {
	UserID        int64
	Provider      string
	TransactionID string
	Product       string
	Months        int32
	Stars         int64
	NeedPremium   bool
}

func validPaymentProduct(product string) bool {
	switch strings.TrimSpace(product) {
	case "premium_subscription", "stars_topup", "bot_invoice":
		return true
	default:
		return false
	}
}

func validatePaymentWebhook(event *PaymentWebhook, payload []byte) error {
	if event == nil || len(payload) == 0 {
		return ErrPaymentRecoveryInvalid
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return ErrPaymentRecoveryInvalid
	}
	event.EventID = strings.TrimSpace(event.EventID)
	event.EventType = strings.TrimSpace(event.EventType)
	event.Provider = strings.TrimSpace(event.Provider)
	event.RequestKey = strings.TrimSpace(event.RequestKey)
	event.TransactionID = strings.TrimSpace(event.TransactionID)
	event.Product = strings.TrimSpace(event.Product)
	event.Reason = strings.TrimSpace(event.Reason)
	if event.EventID == "" || len(event.EventID) > 191 ||
		(event.EventType != "payment.refunded" && event.EventType != "payment.revoked") ||
		event.Provider == "" || len(event.Provider) > 32 || event.UserID <= 0 ||
		len(event.RequestKey) > 191 || event.TransactionID == "" || len(event.TransactionID) > 191 ||
		len(event.Reason) > 255 || !validPaymentProduct(event.Product) || event.Months < 0 || event.Stars < 0 {
		return ErrPaymentRecoveryInvalid
	}
	switch event.Product {
	case "premium_subscription":
		if event.Months < 1 || event.Months > 36 || event.Stars != 0 {
			return ErrPaymentRecoveryInvalid
		}
	case "stars_topup":
		if event.Stars <= 0 || event.Months != 0 {
			return ErrPaymentRecoveryInvalid
		}
	case "bot_invoice":
		if event.Months != 0 || event.Stars != 0 {
			return ErrPaymentRecoveryInvalid
		}
	}
	if event.OccurredAt <= 0 {
		event.OccurredAt = time.Now().Unix()
	}
	return nil
}

func EnqueuePaymentWebhook(ctx context.Context, event PaymentWebhook, payload []byte) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	if err := validatePaymentWebhook(&event, payload); err != nil {
		return false, err
	}
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	now := time.Now().Unix()
	if event.OccurredAt <= 0 {
		event.OccurredAt = now
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO apifull_payment_provider_webhook
		(event_id, event_type, provider, user_id, request_key, transaction_id, product, months, stars, reason,
		 occurred_at, payload_hash, payload, state, attempts, lease_until, claim_token, next_attempt_at, last_error, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,0,0,'',$15,'',$15,$15)
		ON CONFLICT (event_id) DO NOTHING`, event.EventID, event.EventType, event.Provider, event.UserID, event.RequestKey,
		event.TransactionID, event.Product, event.Months, event.Stars, event.Reason, event.OccurredAt, hash, payload,
		PaymentWebhookStatePending, now)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		var storedHash string
		if err = tx.QueryRowContext(ctx, `SELECT payload_hash FROM apifull_payment_provider_webhook WHERE event_id=$1 FOR UPDATE`, event.EventID).Scan(&storedHash); err != nil {
			return false, err
		}
		if storedHash != hash {
			return false, ErrPaymentWebhookConflict
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return inserted == 1, nil
}

func CheckPaymentRecoverySchema(ctx context.Context) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	for _, query := range []string{
		`SELECT event_id, state, claim_token FROM apifull_payment_provider_webhook LIMIT 0`,
		`SELECT provider, transaction_id, state FROM apifull_payment_provider_reversal LIMIT 0`,
		`SELECT provider, transaction_id, state, claim_token FROM apifull_payment_provider_reconciliation LIMIT 0`,
	} {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		_ = rows.Close()
	}
	return nil
}

func LockPaymentRecoveryReconciler(ctx context.Context) (func(), bool, error) {
	if db == nil {
		return nil, false, errors.New("domain PostgreSQL is not open")
	}
	return lockPostgresTransaction(ctx, "apifull_payment_recovery_reconciler", 0)
}

func newRecoveryClaimToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func ClaimPaymentWebhooks(ctx context.Context, limit int) ([]PaymentWebhook, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	now := time.Now().Unix()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT event_id, event_type, provider, user_id, request_key, transaction_id,
		product, months, stars, reason, occurred_at, payload_hash, payload, state, attempts
		FROM apifull_payment_provider_webhook
		WHERE next_attempt_at <= $1 AND (state IN ('pending','failed') OR (state='inflight' AND lease_until <= $1))
		ORDER BY created_at, event_id FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	var candidates []PaymentWebhook
	for rows.Next() {
		var event PaymentWebhook
		var payload []byte
		if err = rows.Scan(&event.EventID, &event.EventType, &event.Provider, &event.UserID, &event.RequestKey,
			&event.TransactionID, &event.Product, &event.Months, &event.Stars, &event.Reason, &event.OccurredAt,
			&event.PayloadHash, &payload, &event.State, &event.Attempts); err != nil {
			return nil, err
		}
		event.Payload = append([]byte(nil), payload...)
		candidates = append(candidates, event)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	claimed := make([]PaymentWebhook, 0, len(candidates))
	for _, event := range candidates {
		token, tokenErr := newRecoveryClaimToken()
		if tokenErr != nil {
			return nil, tokenErr
		}
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_payment_provider_webhook
			SET state='inflight', attempts=attempts+1, lease_until=$1+60, claim_token=$2, updated_at=$1
			WHERE event_id=$3`, now, token, event.EventID); err != nil {
			return nil, err
		}
		event.State = PaymentWebhookStateInflight
		event.Attempts++
		event.ClaimToken = token
		claimed = append(claimed, event)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func CompletePaymentWebhook(ctx context.Context, eventID, claimToken string) error {
	return updatePaymentWebhookState(ctx, eventID, claimToken, PaymentWebhookStateComplete, "")
}

func RetryPaymentWebhook(ctx context.Context, eventID, claimToken string, cause error) error {
	message := "payment webhook processing failed"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > 255 {
		message = message[:255]
	}
	return updatePaymentWebhookState(ctx, eventID, claimToken, PaymentWebhookStateFailed, message)
}

func updatePaymentWebhookState(ctx context.Context, eventID, claimToken, state, message string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if state != PaymentWebhookStateComplete && state != PaymentWebhookStateFailed {
		return ErrPaymentRecoveryInvalid
	}
	now := time.Now().Unix()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_payment_provider_webhook
		SET state=CAST($1 AS varchar(16)), lease_until=0, claim_token='', last_error=CAST($2 AS varchar(255)),
			next_attempt_at=CASE WHEN CAST($1 AS varchar(16))='complete' THEN next_attempt_at
				ELSE CAST($3 AS bigint)+LEAST(3600::bigint, power(2::numeric, LEAST(attempts, 10))::bigint) END,
			updated_at=CAST($3 AS bigint) WHERE event_id=CAST($4 AS varchar(191))
			AND state='inflight' AND claim_token=CAST($5 AS varchar(64))`, state, message, now, eventID, claimToken); err != nil {
		return err
	}
	var changed int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM apifull_payment_provider_webhook WHERE event_id=$1 AND state=$2 AND claim_token=''`, eventID, state).Scan(&changed); errors.Is(err, sql.ErrNoRows) {
		return ErrPaymentRecoveryClaimLost
	} else if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// ApplyPaymentReversal applies the local half of a refund/revoke exactly once.
// Premium's user-service mutation is intentionally returned to the caller and
// executed after this transaction commits; the durable webhook claim makes a
// crash between those two steps retryable.
func ApplyPaymentReversal(ctx context.Context, event PaymentWebhook) (PaymentReversalResult, error) {
	if db == nil {
		return PaymentReversalResult{}, errors.New("domain PostgreSQL is not open")
	}
	if err := validatePaymentWebhook(&event, event.Payload); err != nil {
		return PaymentReversalResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PaymentReversalResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var result PaymentReversalResult
	var requestID, receiptUserID, peerID int64
	var amount int64
	var msgID int32
	var receiptProvider, receiptTransaction, receiptRequestKey, currency, title string
	err = tx.QueryRowContext(ctx, `SELECT p.request_id, p.user_id, p.provider, p.transaction_id, r.request_key,
		p.currency, p.amount, p.peer_id, p.msg_id, p.title
		FROM apifull_payment_receipt p JOIN apifull_payment_request r ON r.id=p.request_id
		WHERE p.user_id=$1 AND p.provider=$2 AND p.transaction_id=$3 FOR UPDATE`,
		event.UserID, event.Provider, event.TransactionID).Scan(&requestID, &receiptUserID, &receiptProvider, &receiptTransaction, &receiptRequestKey,
		&currency, &amount, &peerID, &msgID, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentReversalResult{}, ErrPaymentSettlementNotFound
	}
	if err != nil {
		return PaymentReversalResult{}, err
	}
	if receiptUserID != event.UserID || receiptProvider != event.Provider || receiptTransaction != event.TransactionID {
		return PaymentReversalResult{}, ErrPaymentReversalConflict
	}
	if event.RequestKey != "" && event.RequestKey != receiptRequestKey {
		return PaymentReversalResult{}, ErrPaymentReversalConflict
	}
	result = PaymentReversalResult{UserID: event.UserID, Provider: event.Provider, TransactionID: event.TransactionID, Product: event.Product}
	var actualProduct string
	if err = tx.QueryRowContext(ctx, `SELECT months FROM apifull_payment_entitlement_outbox WHERE request_id=$1`, requestID).Scan(&result.Months); err == nil {
		actualProduct = "premium_subscription"
		result.NeedPremium = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PaymentReversalResult{}, err
	} else {
		idem := starsPaymentCreditIdem(event.Provider, event.TransactionID)
		if err = tx.QueryRowContext(ctx, `SELECT amount FROM apifull_star_tx WHERE user_id=$1 AND idem=$2 FOR UPDATE`, event.UserID, idem).Scan(&result.Stars); err == nil {
			actualProduct = "stars_topup"
		} else if !errors.Is(err, sql.ErrNoRows) {
			return PaymentReversalResult{}, err
		} else {
			actualProduct = "bot_invoice"
		}
	}
	if actualProduct != event.Product {
		return PaymentReversalResult{}, ErrPaymentReversalConflict
	}
	if event.Product == "premium_subscription" {
		if event.Months != result.Months {
			return PaymentReversalResult{}, ErrPaymentReversalConflict
		}
	} else if event.Product == "stars_topup" {
		if result.Stars <= 0 || event.Stars != result.Stars {
			return PaymentReversalResult{}, ErrPaymentReversalConflict
		}
	}
	var existingProduct string
	var existingUser, existingStars int64
	err = tx.QueryRowContext(ctx, `SELECT product, user_id, stars FROM apifull_payment_provider_reversal
		WHERE provider=$1 AND transaction_id=$2 FOR UPDATE`, event.Provider, event.TransactionID).
		Scan(&existingProduct, &existingUser, &existingStars)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().Unix()
		if _, err = tx.ExecContext(ctx, `INSERT INTO apifull_payment_provider_reversal
			(provider, user_id, transaction_id, event_id, product, stars, state, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,'applied',$7,$7)`, event.Provider, event.UserID, event.TransactionID, event.EventID, event.Product, result.Stars, now); err != nil {
			return PaymentReversalResult{}, err
		}
		if result.Product == "stars_topup" {
			if _, err = applyStarsTx(tx, event.UserID, -result.Stars, starsWebhookReversalIdem(event.Provider, event.TransactionID)); err != nil {
				return PaymentReversalResult{}, err
			}
		}
	} else if err != nil {
		return PaymentReversalResult{}, err
	} else if existingProduct != event.Product || existingUser != event.UserID || existingStars != result.Stars {
		return PaymentReversalResult{}, ErrPaymentReversalConflict
	}
	_, err = tx.ExecContext(ctx, `UPDATE apifull_payment_provider_entitlement SET state='revoked', updated_at=CURRENT_TIMESTAMP
		WHERE user_id=$1 AND product=$2 AND transaction_id=$3`, event.UserID, event.Product, event.TransactionID)
	if err != nil {
		return PaymentReversalResult{}, err
	}
	now := time.Now().Unix()
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_payment_request
		SET state=$1, error_text=$2, updated_at=$3 WHERE id=$4`, PaymentStateReversed, event.Reason, now, requestID); err != nil {
		return PaymentReversalResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO apifull_payment_ledger
		(request_id, user_id, state, provider, transaction_id, currency, amount, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (request_id, state) DO NOTHING`,
		requestID, event.UserID, PaymentStateReversed, event.Provider, event.TransactionID, currency, amount, now); err != nil {
		return PaymentReversalResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return PaymentReversalResult{}, err
	}
	return result, nil
}

func starsWebhookReversalIdem(provider, transactionID string) string {
	digest := sha256.Sum256([]byte("webhook\x00" + provider + "\x00" + transactionID))
	return "refund-webhook:" + hex.EncodeToString(digest[:])
}

func EnsurePaymentReconciliations(ctx context.Context, limit int) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	now := time.Now().Unix()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT r.user_id, r.provider, r.request_key, r.transaction_id
		FROM apifull_payment_request r
		JOIN apifull_payment_receipt p ON p.request_id=r.id
		WHERE r.state IN ('settled','reversed') AND r.transaction_id<>''
		  AND NOT EXISTS (
			SELECT 1 FROM apifull_payment_provider_reconciliation x
			WHERE x.provider=r.provider AND x.user_id=r.user_id AND x.transaction_id=r.transaction_id
		  )
		ORDER BY r.updated_at, r.id LIMIT $1`, limit)
	if err != nil {
		return err
	}
	type reconciliationCandidate struct {
		userID        int64
		provider      string
		requestKey    string
		transactionID string
	}
	var candidates []reconciliationCandidate
	for rows.Next() {
		var candidate reconciliationCandidate
		if err = rows.Scan(&candidate.userID, &candidate.provider, &candidate.requestKey, &candidate.transactionID); err != nil {
			return err
		}
		candidates = append(candidates, candidate)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, candidate := range candidates {
		userID, provider, requestKey, transactionID := candidate.userID, candidate.provider, candidate.requestKey, candidate.transactionID
		product := "bot_invoice"
		var months int32
		var stars int64
		if err = tx.QueryRowContext(ctx, `SELECT months FROM apifull_payment_entitlement_outbox o
			JOIN apifull_payment_request r ON r.id=o.request_id WHERE r.user_id=$1 AND r.provider=$2 AND r.transaction_id=$3`, userID, provider, transactionID).Scan(&months); err == nil {
			product = "premium_subscription"
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		} else {
			if err = tx.QueryRowContext(ctx, `SELECT amount FROM apifull_star_tx WHERE user_id=$1 AND idem=$2`, userID, starsPaymentCreditIdem(provider, transactionID)).Scan(&stars); err == nil && stars > 0 {
				product = "stars_topup"
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO apifull_payment_provider_reconciliation
			(provider, user_id, transaction_id, request_key, product, months, stars, provider_status, state,
			 attempts, lease_until, claim_token, next_check_at, last_checked_at, last_error, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'','pending',0,0,'',$8,0,'',$8,$8)
			ON CONFLICT (provider, user_id, transaction_id) DO NOTHING`, provider, userID, transactionID, requestKey, product, months, stars, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ClaimPaymentReconciliations(ctx context.Context, limit int) ([]PaymentReconciliation, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	now := time.Now().Unix()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT provider, user_id, transaction_id, request_key, product, months, stars,
		provider_status, state, attempts FROM apifull_payment_provider_reconciliation
		WHERE next_check_at <= $1 AND (state IN ('pending','failed') OR
			(state='complete' AND provider_status='settled') OR
			(state='inflight' AND lease_until <= $1))
		ORDER BY created_at, provider, transaction_id FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	var candidates []PaymentReconciliation
	for rows.Next() {
		var item PaymentReconciliation
		if err = rows.Scan(&item.Provider, &item.UserID, &item.TransactionID, &item.RequestKey, &item.Product,
			&item.Months, &item.Stars, &item.ProviderStatus, &item.State, &item.Attempts); err != nil {
			return nil, err
		}
		candidates = append(candidates, item)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	claimed := make([]PaymentReconciliation, 0, len(candidates))
	for _, item := range candidates {
		token, tokenErr := newRecoveryClaimToken()
		if tokenErr != nil {
			return nil, tokenErr
		}
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_payment_provider_reconciliation
			SET state='inflight', attempts=attempts+1, lease_until=$1+60, claim_token=$2, updated_at=$1
			WHERE provider=$3 AND user_id=$4 AND transaction_id=$5`, now, token, item.Provider, item.UserID, item.TransactionID); err != nil {
			return nil, err
		}
		item.State = PaymentReconciliationInflight
		item.Attempts++
		item.ClaimToken = token
		claimed = append(claimed, item)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func CompletePaymentReconciliation(ctx context.Context, item PaymentReconciliation, status string) error {
	return updatePaymentReconciliation(ctx, item, PaymentReconciliationComplete, status, "")
}

func RetryPaymentReconciliation(ctx context.Context, item PaymentReconciliation, status string, cause error) error {
	message := "payment reconciliation pending"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > 255 {
		message = message[:255]
	}
	return updatePaymentReconciliation(ctx, item, PaymentReconciliationFailed, status, message)
}

func updatePaymentReconciliation(ctx context.Context, item PaymentReconciliation, state, status, message string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if state != PaymentReconciliationComplete && state != PaymentReconciliationFailed {
		return ErrPaymentRecoveryInvalid
	}
	now := time.Now().Unix()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_payment_provider_reconciliation
		SET state=CAST($1 AS varchar(16)), provider_status=CAST($2 AS varchar(32)), last_checked_at=CAST($3 AS bigint),
		lease_until=0, claim_token='', last_error=CAST($4 AS varchar(255)),
		next_check_at=CASE
			WHEN CAST($1 AS varchar(16))='complete' AND CAST($2 AS varchar(32))='settled' THEN CAST($3 AS bigint)+3600
			WHEN CAST($1 AS varchar(16))='complete' THEN next_check_at
			ELSE CAST($3 AS bigint)+LEAST(86400::bigint, power(2::numeric, LEAST(attempts, 12))::bigint)
		END,
		updated_at=CAST($3 AS bigint) WHERE provider=CAST($5 AS varchar(32)) AND user_id=CAST($6 AS bigint)
		AND transaction_id=CAST($7 AS varchar(191)) AND state='inflight' AND claim_token=CAST($8 AS varchar(64))`,
		state, status, now, message, item.Provider, item.UserID, item.TransactionID, item.ClaimToken); err != nil {
		return err
	}
	var changed int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM apifull_payment_provider_reconciliation
		WHERE provider=$1 AND user_id=$2 AND transaction_id=$3 AND state=$4 AND claim_token=''`, item.Provider, item.UserID, item.TransactionID, state).Scan(&changed); errors.Is(err, sql.ErrNoRows) {
		return ErrPaymentRecoveryClaimLost
	} else if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func PaymentRecoveryEventID(provider, transactionID, status string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(transactionID) + "\x00" + strings.TrimSpace(status)))
	return "reconcile:" + hex.EncodeToString(digest[:])
}

func (e PaymentWebhook) String() string {
	return fmt.Sprintf("%s/%s/%s", e.Provider, e.TransactionID, e.EventType)
}
