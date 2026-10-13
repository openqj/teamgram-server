package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// controlPlane is the provider-owned PostgreSQL state. The fixture keeps no
// customer state in process memory when it is started by main; the in-memory
// maps on fixture are only used by unit tests that do not configure a DSN.
type controlPlane struct {
	db *sql.DB
}

func (p *controlPlane) Ping(ctx context.Context) error {
	if p == nil || p.db == nil {
		return errors.New("payment provider: PostgreSQL is unavailable")
	}
	if err := p.db.PingContext(ctx); err != nil {
		return err
	}
	return validateControlPlaneSchema(ctx, p.db)
}

func openControlPlane(dsn string) (*controlPlane, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" || !(strings.HasPrefix(strings.ToLower(dsn), "postgres://") || strings.HasPrefix(strings.ToLower(dsn), "postgresql://")) {
		return nil, errors.New("payment provider: PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("payment provider: open PostgreSQL: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("payment provider: ping PostgreSQL: %w", err)
	}
	var version int
	if err = db.QueryRow(`SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("payment provider: read PostgreSQL version: %w", err)
	}
	if version/10000 != 18 {
		_ = db.Close()
		return nil, fmt.Errorf("payment provider: PostgreSQL 18 is required (server_version_num=%d)", version)
	}
	if err = validateControlPlaneSchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &controlPlane{db: db}, nil
}

func validateControlPlaneSchema(ctx context.Context, db *sql.DB) error {
	if err := requireMigrationCatalog(ctx, db, "078_apifull_payment_provider_control_plane", "079_provider_control_plane", "093_apifull_payment_provider_stars_subscriptions"); err != nil {
		return fmt.Errorf("payment provider: migration catalog is unavailable: %w", err)
	}
	for _, query := range []string{
		`SELECT operation, user_id, request_key, payload_hash, response, signature_header, transaction_id FROM apifull_payment_provider_operation LIMIT 0`,
		`SELECT operation, user_id, request_key, transaction_id, response FROM apifull_payment_provider_receipt LIMIT 0`,
		`SELECT user_id, product, transaction_id, months, stars, state FROM apifull_payment_provider_entitlement LIMIT 0`,
		`SELECT user_id, balance FROM apifull_payment_provider_stars_account LIMIT 0`,
		`SELECT user_id, subscription_id, peer, until_date, pricing_period, pricing_amount, canceled, can_refulfill, missing_balance, bot_canceled FROM apifull_payment_provider_stars_subscription LIMIT 0`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("payment provider: provider schema unavailable: %w", err)
		}
	}
	return nil
}

func requireMigrationCatalog(ctx context.Context, db *sql.DB, versions ...string) error {
	if db == nil || len(versions) == 0 {
		return errors.New("migration catalog is unavailable")
	}
	for _, version := range versions {
		var checksum string
		var appliedAt time.Time
		err := db.QueryRowContext(ctx, `
			SELECT checksum, applied_at
			FROM public.schema_migrations
			WHERE version = $1`, version).Scan(&checksum, &appliedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("migration %q is not applied", version)
		}
		if err != nil {
			return err
		}
		if !validMigrationChecksum(checksum) || appliedAt.IsZero() {
			return fmt.Errorf("migration %q has an invalid catalog record", version)
		}
	}
	return nil
}

func validMigrationChecksum(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func (p *controlPlane) close() error {
	if p == nil || p.db == nil {
		return nil
	}
	return p.db.Close()
}

type persistedProviderOperation struct {
	Response []byte
	Header   string
}

var errProviderTransactionConflict = errors.New("payment provider transaction belongs to another request")

// providerTransactionIdentityMatches keeps one provider transaction bound to
// the operation/user/request that first recorded it. The database primary key
// prevents duplicate transaction rows, but the identity check is still
// required because a conflicting insert must fail closed instead of replacing
// the existing receipt.
func providerTransactionIdentityMatches(storedUserID int64, storedRequestKey string, userID int64, requestKey string) bool {
	return storedUserID > 0 && userID > 0 && storedUserID == userID &&
		strings.TrimSpace(storedRequestKey) != "" &&
		strings.TrimSpace(storedRequestKey) == strings.TrimSpace(requestKey)
}

// loadOperation returns an existing result and verifies that a retry carries
// the same payload. A different payload under an idempotency key is a hard
// conflict and must never be silently reused.
func (p *controlPlane) loadOperation(operation, requestKey string, userID int64, payload []byte) (persistedProviderOperation, bool, error) {
	if p == nil || p.db == nil || strings.TrimSpace(operation) == "" || userID <= 0 || strings.TrimSpace(requestKey) == "" {
		return persistedProviderOperation{}, false, nil
	}
	digest := sha256.Sum256(payload)
	var storedHash, header string
	var response []byte
	err := p.db.QueryRow(`SELECT payload_hash, response, signature_header
		FROM apifull_payment_provider_operation
		WHERE operation=$1 AND user_id=$2 AND request_key=$3`, operation, userID, requestKey).
		Scan(&storedHash, &response, &header)
	if errors.Is(err, sql.ErrNoRows) {
		return persistedProviderOperation{}, false, nil
	}
	if err != nil {
		return persistedProviderOperation{}, false, err
	}
	if storedHash != hex.EncodeToString(digest[:]) {
		return persistedProviderOperation{}, false, errors.New("payment provider idempotency conflict")
	}
	canonical, err := canonicalJSON(response)
	if err != nil {
		return persistedProviderOperation{}, false, fmt.Errorf("payment provider stored response is invalid JSON: %w", err)
	}
	return persistedProviderOperation{Response: canonical, Header: header}, true, nil
}

func (p *controlPlane) saveOperation(operation, requestKey string, userID int64, payload, response []byte, header string, transactionID string, entitlementProduct string, entitlementMonths, entitlementStars int64) (persistedProviderOperation, error) {
	if p == nil || p.db == nil {
		return persistedProviderOperation{Response: append([]byte(nil), response...), Header: header}, nil
	}
	if strings.TrimSpace(operation) == "" || strings.TrimSpace(requestKey) == "" || userID <= 0 || len(response) == 0 || strings.TrimSpace(header) == "" {
		return persistedProviderOperation{}, errors.New("payment provider operation identity is invalid")
	}
	canonicalResponse, err := canonicalJSON(response)
	if err != nil {
		return persistedProviderOperation{}, fmt.Errorf("payment provider response is invalid JSON: %w", err)
	}
	response = canonicalResponse
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	tx, err := p.db.Begin()
	if err != nil {
		return persistedProviderOperation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var storedHash string
	var storedResponse []byte
	var storedHeader string
	err = tx.QueryRow(`SELECT payload_hash, response, signature_header
		FROM apifull_payment_provider_operation
		WHERE operation=$1 AND user_id=$2 AND request_key=$3 FOR UPDATE`, operation, userID, requestKey).
		Scan(&storedHash, &storedResponse, &storedHeader)
	if err == nil {
		if storedHash != hash {
			return persistedProviderOperation{}, errors.New("payment provider idempotency conflict")
		}
		storedResponse, err = canonicalJSON(storedResponse)
		if err != nil {
			return persistedProviderOperation{}, fmt.Errorf("payment provider stored response is invalid JSON: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return persistedProviderOperation{}, err
		}
		return persistedProviderOperation{Response: append([]byte(nil), storedResponse...), Header: storedHeader}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return persistedProviderOperation{}, err
	}
	now := time.Now().UTC()
	inserted, err := tx.Exec(`INSERT INTO apifull_payment_provider_operation
		(operation, user_id, request_key, payload_hash, response, signature_header, transaction_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$8)
		ON CONFLICT (operation, user_id, request_key) DO NOTHING`, operation, userID, requestKey, hash, response, header, transactionID, now)
	if err != nil {
		return persistedProviderOperation{}, err
	}
	if rows, rowsErr := inserted.RowsAffected(); rowsErr != nil {
		return persistedProviderOperation{}, rowsErr
	} else if rows == 0 {
		if err = tx.QueryRow(`SELECT payload_hash, response, signature_header
			FROM apifull_payment_provider_operation
			WHERE operation=$1 AND user_id=$2 AND request_key=$3 FOR UPDATE`, operation, userID, requestKey).
			Scan(&storedHash, &storedResponse, &storedHeader); err != nil {
			return persistedProviderOperation{}, err
		}
		if storedHash != hash {
			return persistedProviderOperation{}, errors.New("payment provider idempotency conflict")
		}
		storedResponse, err = canonicalJSON(storedResponse)
		if err != nil {
			return persistedProviderOperation{}, fmt.Errorf("payment provider stored response is invalid JSON: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return persistedProviderOperation{}, err
		}
		return persistedProviderOperation{Response: append([]byte(nil), storedResponse...), Header: storedHeader}, nil
	}
	if strings.TrimSpace(transactionID) != "" {
		var storedReceiptUserID int64
		var storedReceiptRequestKey string
		err = tx.QueryRow(`SELECT user_id, request_key
			FROM apifull_payment_provider_receipt
			WHERE operation=$1 AND transaction_id=$2 FOR UPDATE`, operation, transactionID).
			Scan(&storedReceiptUserID, &storedReceiptRequestKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return persistedProviderOperation{}, err
		}
		if err == nil && !providerTransactionIdentityMatches(storedReceiptUserID, storedReceiptRequestKey, userID, requestKey) {
			return persistedProviderOperation{}, errProviderTransactionConflict
		}
		insertedReceipt, insertErr := tx.Exec(`INSERT INTO apifull_payment_provider_receipt
			(operation, user_id, request_key, transaction_id, response, created_at)
			VALUES ($1,$2,$3,$4,$5::jsonb,$6)
			ON CONFLICT (operation, transaction_id) DO NOTHING`, operation, userID, requestKey, transactionID, response, now)
		if insertErr != nil {
			return persistedProviderOperation{}, insertErr
		}
		if rows, rowsErr := insertedReceipt.RowsAffected(); rowsErr != nil {
			return persistedProviderOperation{}, rowsErr
		} else if rows == 0 {
			return persistedProviderOperation{}, errProviderTransactionConflict
		}
	}
	if isStarsSubscriptionMutationOperation(operation) {
		if err = p.applyStarsSubscriptionMutationTx(tx, operation, payload); err != nil {
			return persistedProviderOperation{}, err
		}
	}
	if strings.TrimSpace(entitlementProduct) != "" {
		if entitlementMonths < 0 || entitlementStars < 0 {
			return persistedProviderOperation{}, errors.New("payment entitlement amount is invalid")
		}
		_, err = tx.Exec(`INSERT INTO apifull_payment_provider_entitlement
			(user_id, product, transaction_id, months, stars, state, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,'granted',$6,$6)
			ON CONFLICT (user_id, product, transaction_id) DO NOTHING`, userID, entitlementProduct, transactionID, entitlementMonths, entitlementStars, now)
		if err != nil {
			return persistedProviderOperation{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return persistedProviderOperation{}, err
	}
	return persistedProviderOperation{Response: append([]byte(nil), response...), Header: header}, nil
}

// canonicalJSON makes the exact response bytes stable across a PostgreSQL
// JSONB round-trip. JSONB does not preserve object key order, while provider
// signatures and idempotent replay are defined over the response bytes.
// UseNumber keeps large integer fields lossless while encoding/json sorts map
// keys recursively.
func canonicalJSON(body []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func providerRequestKey(fields map[string]json.RawMessage, operation string, body []byte) string {
	for _, name := range []string{"request_key", "request_id", "idempotency_key", "transaction_id", "purchase_token", "giveaway_id", "gift_id", "slug"} {
		if value := rawString(fields, name); value != "" {
			return operation + ":" + value
		}
	}
	digest := sha256.Sum256(body)
	return operation + ":" + hex.EncodeToString(digest[:])
}
