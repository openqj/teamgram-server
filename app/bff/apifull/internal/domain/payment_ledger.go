package domain

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	PaymentStatePending  = "pending"
	PaymentStateRejected = "rejected"
	PaymentStateSettled  = "settled"
)

var (
	ErrInvalidPaymentRequest      = errors.New("invalid payment request")
	ErrPaymentRequestConflict     = errors.New("payment request conflicts with existing request")
	ErrPaymentRequestNotFound     = errors.New("payment request not found")
	ErrPaymentRequestState        = errors.New("invalid payment request state")
	ErrPaymentUnverified          = errors.New("payment has not been verified by its provider")
	ErrPaymentTransactionConflict = errors.New("payment transaction belongs to another request")
	ErrPaymentRequestBusy         = errors.New("payment request is already being processed")
)

// PaymentRequest is the durable state machine for one client payment attempt.
// Provider verification is deliberately outside this package; SettlePayment
// accepts a verified result explicitly and never treats a receipt as proof.
type PaymentRequest struct {
	ID            int64
	UserID        int64
	RequestKey    string
	Provider      string
	Fingerprint   string
	State         string
	TransactionID string
	Currency      string
	Amount        int64
	PeerID        int64
	MsgID         int32
	ErrorText     string
	CreatedAt     int64
	UpdatedAt     int64
}

type PaymentReceipt struct {
	RequestID     int64
	UserID        int64
	Provider      string
	TransactionID string
	Currency      string
	Amount        int64
	PeerID        int64
	MsgID         int32
	Title         string
	Receipt       []byte
	CreatedAt     int64
}

type PremiumGrant struct {
	RequestID     int64
	UserID        int64
	Provider      string
	TransactionID string
	Months        int32
}

// LockPaymentRequest serializes provider calls for one idempotency key across
// APIFull instances. Rolling back its transaction releases the advisory lock.
func LockPaymentRequest(ctx context.Context, userID int64, requestKey string, wait time.Duration) (func(), error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(requestKey) == "" {
		return nil, ErrInvalidPaymentRequest
	}
	if wait <= 0 {
		wait = 5 * time.Second
	}
	keyHash := sha256.Sum256([]byte(strconv.FormatInt(userID, 10) + "\x00" + requestKey))
	lockName := "apifull_payment:" + hex.EncodeToString(keyHash[:24])
	release, acquired, err := lockPostgresTransaction(ctx, lockName, wait)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrPaymentRequestBusy
	}
	return release, nil
}

func BeginPaymentRequest(userID int64, requestKey, provider, fingerprint, currency string, amount, peerID int64, msgID int32) (PaymentRequest, error) {
	if db == nil {
		return PaymentRequest{}, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(requestKey) == "" || strings.TrimSpace(provider) == "" || strings.TrimSpace(fingerprint) == "" || amount < 0 || peerID < 0 || msgID < 0 {
		return PaymentRequest{}, ErrInvalidPaymentRequest
	}
	now := time.Now().Unix()
	tx, err := db.Begin()
	if err != nil {
		return PaymentRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`INSERT INTO apifull_payment_request
		(user_id, request_key, provider, fingerprint, state, currency, amount, peer_id, msg_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (user_id, request_key) DO NOTHING`, userID, requestKey, provider, fingerprint, PaymentStatePending, currency, amount, peerID, msgID, now, now)
	if err != nil {
		return PaymentRequest{}, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return PaymentRequest{}, err
	}
	request, err := loadPaymentRequestTx(tx, userID, requestKey, true)
	if err != nil {
		return PaymentRequest{}, err
	}
	if !samePaymentRequest(request, provider, fingerprint, currency, amount, peerID, msgID) {
		return PaymentRequest{}, ErrPaymentRequestConflict
	}
	if inserted == 0 {
		if err = tx.Commit(); err != nil {
			return PaymentRequest{}, err
		}
		return request, nil
	}
	if _, err = tx.Exec(`INSERT INTO apifull_payment_ledger
		(request_id, user_id, state, provider, currency, amount, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, request.ID, userID, PaymentStatePending, provider, currency, amount, now); err != nil {
		return PaymentRequest{}, err
	}
	if err = tx.Commit(); err != nil {
		return PaymentRequest{}, err
	}
	return request, nil
}

func RejectPaymentRequest(userID int64, requestKey, fingerprint, reason string) (PaymentRequest, error) {
	if db == nil {
		return PaymentRequest{}, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(requestKey) == "" || strings.TrimSpace(fingerprint) == "" {
		return PaymentRequest{}, ErrInvalidPaymentRequest
	}
	tx, err := db.Begin()
	if err != nil {
		return PaymentRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	request, err := loadPaymentRequestTx(tx, userID, requestKey, true)
	if err == sql.ErrNoRows {
		return PaymentRequest{}, ErrPaymentRequestNotFound
	}
	if err != nil {
		return PaymentRequest{}, err
	}
	if request.Fingerprint != fingerprint {
		return PaymentRequest{}, ErrPaymentRequestConflict
	}
	if request.State == PaymentStateRejected {
		if err = tx.Commit(); err != nil {
			return PaymentRequest{}, err
		}
		return request, nil
	}
	if request.State != PaymentStatePending {
		return PaymentRequest{}, ErrPaymentRequestState
	}
	now := time.Now().Unix()
	if _, err = tx.Exec(`UPDATE apifull_payment_request SET state=$1, error_text=$2, updated_at=$3 WHERE id=$4`, PaymentStateRejected, reason, now, request.ID); err != nil {
		return PaymentRequest{}, err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_payment_ledger
		(request_id, user_id, state, provider, transaction_id, currency, amount, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, request.ID, request.UserID, PaymentStateRejected, request.Provider, request.TransactionID, request.Currency, request.Amount, now); err != nil {
		return PaymentRequest{}, err
	}
	request.State, request.ErrorText, request.UpdatedAt = PaymentStateRejected, reason, now
	if err = tx.Commit(); err != nil {
		return PaymentRequest{}, err
	}
	return request, nil
}

// SettlePaymentRequest records a provider-verified payment atomically. A false
// verified flag is rejected before any state or receipt is written.
func SettlePaymentRequest(userID int64, requestKey, fingerprint, transactionID, currency, title string, amount, peerID int64, msgID int32, receipt []byte, verified bool) (PaymentRequest, PaymentReceipt, error) {
	return settlePaymentRequest(userID, requestKey, fingerprint, transactionID, currency, title, amount, peerID, msgID, receipt, verified, 0, 0)
}

func SettlePremiumPaymentRequest(userID int64, requestKey, fingerprint, transactionID, currency, title string, amount, peerID int64, msgID int32, receipt []byte, months int32) (PaymentRequest, PaymentReceipt, error) {
	if months < 1 || months > 36 {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidPaymentRequest
	}
	return settlePaymentRequest(userID, requestKey, fingerprint, transactionID, currency, title, amount, peerID, msgID, receipt, true, months, 0)
}

func SettleStarsPaymentRequest(userID int64, requestKey, fingerprint, transactionID, currency, title string, amount, stars int64, receipt []byte) (PaymentRequest, PaymentReceipt, error) {
	if stars <= 0 {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidStarsTransaction
	}
	return settlePaymentRequest(userID, requestKey, fingerprint, transactionID, currency, title, amount, 0, 0, receipt, true, 0, stars)
}

func settlePaymentRequest(userID int64, requestKey, fingerprint, transactionID, currency, title string, amount, peerID int64, msgID int32, receipt []byte, verified bool, premiumMonths int32, stars int64) (PaymentRequest, PaymentReceipt, error) {
	if db == nil {
		return PaymentRequest{}, PaymentReceipt{}, errors.New("domain PostgreSQL is not open")
	}
	if !verified {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentUnverified
	}
	if userID <= 0 || strings.TrimSpace(requestKey) == "" || strings.TrimSpace(fingerprint) == "" || strings.TrimSpace(transactionID) == "" || len(receipt) == 0 || amount < 0 || peerID < 0 || msgID < 0 || stars < 0 {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidPaymentRequest
	}
	tx, err := db.Begin()
	if err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	request, err := loadPaymentRequestTx(tx, userID, requestKey, true)
	if err == sql.ErrNoRows {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestNotFound
	}
	if err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	// A slug invoice may not carry a local quote. In that case the provider's
	// verified currency and amount become authoritative for the receipt while
	// the request keeps its empty quote for idempotent retries. Purpose-based
	// invoices retain exact currency/amount matching.
	if request.Fingerprint != fingerprint ||
		((request.Currency != "" || request.Amount != 0) && (request.Currency != currency || request.Amount != amount)) ||
		request.PeerID != peerID || request.MsgID != msgID {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestConflict
	}
	if request.State == PaymentStateSettled {
		receiptRow, receiptErr := loadPaymentReceiptTx(tx, request.ID)
		if receiptErr != nil {
			return PaymentRequest{}, PaymentReceipt{}, receiptErr
		}
		if receiptRow.TransactionID != transactionID || !equalHash(receiptRow.Receipt, receipt) {
			return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestConflict
		}
		if premiumMonths > 0 {
			if err = insertPremiumGrantTx(tx, request.ID, userID, request.Provider, transactionID, premiumMonths); err != nil {
				return PaymentRequest{}, PaymentReceipt{}, err
			}
		}
		if stars > 0 {
			if err = insertStarsGrantTx(tx, userID, request.Provider, transactionID, stars); err != nil {
				return PaymentRequest{}, PaymentReceipt{}, err
			}
		}
		if err = tx.Commit(); err != nil {
			return PaymentRequest{}, PaymentReceipt{}, err
		}
		return request, receiptRow, nil
	}
	if request.State != PaymentStatePending {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestState
	}
	now := time.Now().Unix()
	hash := sha256.Sum256(receipt)
	result, err := tx.Exec(`INSERT INTO apifull_payment_receipt
		(request_id, user_id, provider, transaction_id, currency, amount, peer_id, msg_id, title, receipt, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT DO NOTHING`, request.ID, request.UserID, request.Provider, transactionID, currency, amount, peerID, msgID, title, receipt, now)
	if err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	if inserted == 0 {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentTransactionConflict
	}
	if _, err = tx.Exec(`UPDATE apifull_payment_request SET state=$1, transaction_id=$2, updated_at=$3 WHERE id=$4`, PaymentStateSettled, transactionID, now, request.ID); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_payment_ledger
		(request_id, user_id, state, provider, transaction_id, currency, amount, receipt_hash, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, request.ID, request.UserID, PaymentStateSettled, request.Provider, transactionID, currency, amount, hex.EncodeToString(hash[:]), now); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	if premiumMonths > 0 {
		if err = insertPremiumGrantTx(tx, request.ID, userID, request.Provider, transactionID, premiumMonths); err != nil {
			return PaymentRequest{}, PaymentReceipt{}, err
		}
	}
	if stars > 0 {
		if err = insertStarsGrantTx(tx, userID, request.Provider, transactionID, stars); err != nil {
			return PaymentRequest{}, PaymentReceipt{}, err
		}
	}
	request.State, request.TransactionID, request.UpdatedAt = PaymentStateSettled, transactionID, now
	receiptRow := PaymentReceipt{RequestID: request.ID, UserID: request.UserID, Provider: request.Provider, TransactionID: transactionID, Currency: currency, Amount: amount, PeerID: peerID, MsgID: msgID, Title: title, Receipt: append([]byte(nil), receipt...), CreatedAt: now}
	if err = tx.Commit(); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	return request, receiptRow, nil
}

func insertStarsGrantTx(tx *sql.Tx, userID int64, provider, transactionID string, stars int64) error {
	if userID <= 0 || strings.TrimSpace(provider) == "" || strings.TrimSpace(transactionID) == "" || stars <= 0 {
		return ErrInvalidStarsTransaction
	}
	digest := sha256.Sum256([]byte(provider + "\x00" + transactionID))
	idem := "payment:" + hex.EncodeToString(digest[:])
	_, err := applyStarsTx(tx, userID, stars, idem)
	return err
}

func EnsurePremiumGrant(requestID, userID int64, provider, transactionID string, months int32) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if requestID <= 0 || userID <= 0 || strings.TrimSpace(provider) == "" || len(provider) > 32 || strings.TrimSpace(transactionID) == "" || len(transactionID) > 191 || months < 1 || months > 36 {
		return ErrInvalidPaymentRequest
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = insertPremiumGrantTx(tx, requestID, userID, provider, transactionID, months); err != nil {
		return err
	}
	return tx.Commit()
}

func insertPremiumGrantTx(tx *sql.Tx, requestID, userID int64, provider, transactionID string, months int32) error {
	if requestID <= 0 || userID <= 0 || strings.TrimSpace(provider) == "" || len(provider) > 32 || strings.TrimSpace(transactionID) == "" || len(transactionID) > 191 || months < 1 || months > 36 {
		return ErrInvalidPaymentRequest
	}
	now := time.Now().Unix()
	_, err := tx.Exec(`INSERT INTO apifull_payment_entitlement_outbox
		(request_id, user_id, provider, transaction_id, months, state, attempts, next_attempt_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,'pending',0,$6,$6,$6)
		ON CONFLICT DO NOTHING`, requestID, userID, provider, transactionID, months, now)
	if err != nil {
		return err
	}
	var existing PremiumGrant
	err = tx.QueryRow(`SELECT request_id, user_id, provider, transaction_id, months FROM apifull_payment_entitlement_outbox WHERE request_id=$1 FOR UPDATE`, requestID).
		Scan(&existing.RequestID, &existing.UserID, &existing.Provider, &existing.TransactionID, &existing.Months)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPaymentTransactionConflict
	}
	if err != nil {
		return err
	}
	if existing.UserID != userID || existing.Provider != provider || existing.TransactionID != transactionID || existing.Months != months {
		return ErrPaymentTransactionConflict
	}
	return nil
}

func LockPremiumGrantReconciler(ctx context.Context) (func(), bool, error) {
	if db == nil {
		return nil, false, errors.New("domain PostgreSQL is not open")
	}
	return lockPostgresTransaction(ctx, "apifull_premium_grant_reconciler", 0)
}

func CheckPremiumGrantOutbox(ctx context.Context) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.QueryContext(ctx, `SELECT request_id FROM apifull_payment_entitlement_outbox LIMIT 0`)
	if err != nil {
		return err
	}
	return rows.Close()
}

func ListDuePremiumGrants(ctx context.Context, limit int) ([]PremiumGrant, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `SELECT request_id, user_id, provider, transaction_id, months
		FROM apifull_payment_entitlement_outbox WHERE state='pending' AND next_attempt_at<=$1
		ORDER BY created_at, request_id LIMIT $2`, time.Now().Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make([]PremiumGrant, 0)
	for rows.Next() {
		var grant PremiumGrant
		if err = rows.Scan(&grant.RequestID, &grant.UserID, &grant.Provider, &grant.TransactionID, &grant.Months); err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func CompletePremiumGrant(ctx context.Context, requestID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_payment_entitlement_outbox SET state='complete', last_error='', updated_at=$1 WHERE request_id=$2`, time.Now().Unix(), requestID); err != nil {
		return err
	}
	return tx.Commit()
}

func RetryPremiumGrant(ctx context.Context, requestID int64, cause error) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if requestID <= 0 {
		return ErrInvalidPaymentRequest
	}
	message := "grant failed"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > 255 {
		message = message[:255]
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_payment_entitlement_outbox
		SET next_attempt_at=$1+LEAST(900::bigint, power(2::numeric, LEAST(attempts+1, 10))::bigint),
			attempts=attempts+1,
			last_error=$2, updated_at=$1
		WHERE request_id=$3 AND state='pending'`, time.Now().Unix(), message, requestID); err != nil {
		return err
	}
	return tx.Commit()
}

func LoadPaymentRequest(userID int64, requestKey string) (PaymentRequest, bool, error) {
	if db == nil {
		return PaymentRequest{}, false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(requestKey) == "" {
		return PaymentRequest{}, false, ErrInvalidPaymentRequest
	}
	request, err := loadPaymentRequestTx(db, userID, requestKey, false)
	if err == sql.ErrNoRows {
		return PaymentRequest{}, false, nil
	}
	if err != nil {
		return PaymentRequest{}, false, err
	}
	return request, true, nil
}

func LoadPaymentReceiptByRequest(userID int64, requestKey string) (PaymentReceipt, bool, error) {
	request, found, err := LoadPaymentRequest(userID, requestKey)
	if err != nil || !found {
		return PaymentReceipt{}, false, err
	}
	receipt, err := loadPaymentReceiptTx(db, request.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentReceipt{}, false, nil
	}
	if err != nil {
		return PaymentReceipt{}, false, err
	}
	return receipt, true, nil
}

func LoadPaymentReceiptByMessage(userID, peerID int64, msgID int32) (PaymentReceipt, bool, error) {
	if db == nil {
		return PaymentReceipt{}, false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || peerID <= 0 || msgID <= 0 {
		return PaymentReceipt{}, false, ErrInvalidPaymentRequest
	}
	var row PaymentReceipt
	err := db.QueryRow(`SELECT request_id, user_id, provider, transaction_id, currency, amount, peer_id, msg_id, title, receipt, created_at
		FROM apifull_payment_receipt WHERE user_id=$1 AND peer_id=$2 AND msg_id=$3`, userID, peerID, msgID).
		Scan(&row.RequestID, &row.UserID, &row.Provider, &row.TransactionID, &row.Currency, &row.Amount, &row.PeerID, &row.MsgID, &row.Title, &row.Receipt, &row.CreatedAt)
	if err == sql.ErrNoRows {
		return PaymentReceipt{}, false, nil
	}
	if err != nil {
		return PaymentReceipt{}, false, err
	}
	return row, true, nil
}

func loadPaymentRequestTx(q interface {
	QueryRow(query string, args ...any) *sql.Row
}, userID int64, requestKey string, lock bool) (PaymentRequest, error) {
	query := `SELECT id, user_id, request_key, provider, RTRIM(fingerprint), state, transaction_id, currency, amount, peer_id, msg_id, error_text, created_at, updated_at FROM apifull_payment_request WHERE user_id=$1 AND request_key=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	var request PaymentRequest
	err := q.QueryRow(query, userID, requestKey).Scan(&request.ID, &request.UserID, &request.RequestKey, &request.Provider, &request.Fingerprint, &request.State, &request.TransactionID, &request.Currency, &request.Amount, &request.PeerID, &request.MsgID, &request.ErrorText, &request.CreatedAt, &request.UpdatedAt)
	return request, err
}

func loadPaymentReceiptTx(q interface {
	QueryRow(query string, args ...any) *sql.Row
}, requestID int64) (PaymentReceipt, error) {
	var row PaymentReceipt
	err := q.QueryRow(`SELECT request_id, user_id, provider, transaction_id, currency, amount, peer_id, msg_id, title, receipt, created_at
		FROM apifull_payment_receipt WHERE request_id=$1`, requestID).
		Scan(&row.RequestID, &row.UserID, &row.Provider, &row.TransactionID, &row.Currency, &row.Amount, &row.PeerID, &row.MsgID, &row.Title, &row.Receipt, &row.CreatedAt)
	return row, err
}

func samePaymentRequest(request PaymentRequest, provider, fingerprint, currency string, amount, peerID int64, msgID int32) bool {
	return request.Provider == provider && request.Fingerprint == fingerprint && request.Currency == currency && request.Amount == amount && request.PeerID == peerID && request.MsgID == msgID
}

func equalHash(a, b []byte) bool {
	left := sha256.Sum256(a)
	right := sha256.Sum256(b)
	return left == right
}
