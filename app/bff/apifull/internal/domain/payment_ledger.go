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

// LockPaymentRequest serializes provider calls for one idempotency key across
// APIFull instances. MySQL releases the named lock when its connection closes.
func LockPaymentRequest(ctx context.Context, userID int64, requestKey string, wait time.Duration) (func(), error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if userID <= 0 || strings.TrimSpace(requestKey) == "" {
		return nil, ErrInvalidPaymentRequest
	}
	if wait <= 0 {
		wait = 5 * time.Second
	}
	seconds := int(wait / time.Second)
	if wait%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	keyHash := sha256.Sum256([]byte(strconv.FormatInt(userID, 10) + "\x00" + requestKey))
	lockName := "apifull_payment:" + hex.EncodeToString(keyHash[:24])
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var acquired sql.NullInt64
	if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, lockName, seconds).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		_ = conn.Close()
		return nil, ErrPaymentRequestBusy
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var released sql.NullInt64
		_ = conn.QueryRowContext(releaseCtx, `SELECT RELEASE_LOCK(?)`, lockName).Scan(&released)
		_ = conn.Close()
	}, nil
}

func BeginPaymentRequest(userID int64, requestKey, provider, fingerprint, currency string, amount, peerID int64, msgID int32) (PaymentRequest, error) {
	if db == nil {
		return PaymentRequest{}, errors.New("domain mysql is not open")
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

	result, err := tx.Exec(`INSERT IGNORE INTO apifull_payment_request
		(user_id, request_key, provider, fingerprint, state, currency, amount, peer_id, msg_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, userID, requestKey, provider, fingerprint, PaymentStatePending, currency, amount, peerID, msgID, now, now)
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
		VALUES (?,?,?,?,?,?,?)`, request.ID, userID, PaymentStatePending, provider, currency, amount, now); err != nil {
		return PaymentRequest{}, err
	}
	if err = tx.Commit(); err != nil {
		return PaymentRequest{}, err
	}
	return request, nil
}

func RejectPaymentRequest(userID int64, requestKey, fingerprint, reason string) (PaymentRequest, error) {
	if db == nil {
		return PaymentRequest{}, errors.New("domain mysql is not open")
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
	if _, err = tx.Exec(`UPDATE apifull_payment_request SET state=?, error_text=?, updated_at=? WHERE id=?`, PaymentStateRejected, reason, now, request.ID); err != nil {
		return PaymentRequest{}, err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_payment_ledger
		(request_id, user_id, state, provider, transaction_id, currency, amount, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, request.ID, request.UserID, PaymentStateRejected, request.Provider, request.TransactionID, request.Currency, request.Amount, now); err != nil {
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
	if db == nil {
		return PaymentRequest{}, PaymentReceipt{}, errors.New("domain mysql is not open")
	}
	if !verified {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentUnverified
	}
	if userID <= 0 || strings.TrimSpace(requestKey) == "" || strings.TrimSpace(fingerprint) == "" || strings.TrimSpace(transactionID) == "" || len(receipt) == 0 || amount < 0 || peerID < 0 || msgID < 0 {
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
	if _, err = tx.Exec(`INSERT INTO apifull_payment_receipt
		(request_id, user_id, provider, transaction_id, currency, amount, peer_id, msg_id, title, receipt, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, request.ID, request.UserID, request.Provider, transactionID, currency, amount, peerID, msgID, title, receipt, now); err != nil {
		if !isDuplicateKey(err) {
			return PaymentRequest{}, PaymentReceipt{}, err
		}
		receiptRow, receiptErr := loadPaymentReceiptTx(tx, request.ID)
		if receiptErr == nil && receiptRow.TransactionID == transactionID && equalHash(receiptRow.Receipt, receipt) {
			if err = tx.Commit(); err != nil {
				return PaymentRequest{}, PaymentReceipt{}, err
			}
			return request, receiptRow, nil
		}
		if receiptErr != nil && receiptErr != sql.ErrNoRows {
			return PaymentRequest{}, PaymentReceipt{}, receiptErr
		}
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentTransactionConflict
	}
	if _, err = tx.Exec(`UPDATE apifull_payment_request SET state=?, transaction_id=?, updated_at=? WHERE id=?`, PaymentStateSettled, transactionID, now, request.ID); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_payment_ledger
		(request_id, user_id, state, provider, transaction_id, currency, amount, receipt_hash, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, request.ID, request.UserID, PaymentStateSettled, request.Provider, transactionID, currency, amount, hex.EncodeToString(hash[:]), now); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	request.State, request.TransactionID, request.UpdatedAt = PaymentStateSettled, transactionID, now
	receiptRow := PaymentReceipt{RequestID: request.ID, UserID: request.UserID, Provider: request.Provider, TransactionID: transactionID, Currency: currency, Amount: amount, PeerID: peerID, MsgID: msgID, Title: title, Receipt: append([]byte(nil), receipt...), CreatedAt: now}
	if err = tx.Commit(); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	return request, receiptRow, nil
}

func LoadPaymentRequest(userID int64, requestKey string) (PaymentRequest, bool, error) {
	if db == nil {
		return PaymentRequest{}, false, errors.New("domain mysql is not open")
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
		return PaymentReceipt{}, false, errors.New("domain mysql is not open")
	}
	if userID <= 0 || peerID <= 0 || msgID <= 0 {
		return PaymentReceipt{}, false, ErrInvalidPaymentRequest
	}
	var row PaymentReceipt
	err := db.QueryRow(`SELECT request_id, user_id, provider, transaction_id, currency, amount, peer_id, msg_id, title, receipt, created_at
		FROM apifull_payment_receipt WHERE user_id=? AND peer_id=? AND msg_id=?`, userID, peerID, msgID).
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
	query := `SELECT id, user_id, request_key, provider, fingerprint, state, transaction_id, currency, amount, peer_id, msg_id, error_text, created_at, updated_at FROM apifull_payment_request WHERE user_id=? AND request_key=?`
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
		FROM apifull_payment_receipt WHERE request_id=?`, requestID).
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
