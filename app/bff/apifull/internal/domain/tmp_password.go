package domain

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"
)

// StoreTmpPassword replaces the caller's temporary payment password in one
// transaction. Only the digest is persisted; the clear token is returned to
// the client by account.getTmpPassword and is never stored server-side.
func StoreTmpPassword(ctx context.Context, userID int64, token []byte, validUntil int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || len(token) == 0 || validUntil <= time.Now().Unix() {
		return ErrInvalidPaymentRequest
	}
	digest := sha256.Sum256(token)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT INTO apifull_tmp_password
		(user_id, token_digest, valid_until, created_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (user_id) DO UPDATE SET token_digest=EXCLUDED.token_digest,
		valid_until=EXCLUDED.valid_until, created_at=EXCLUDED.created_at`,
		userID, digest[:], validUntil, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// ValidateTmpPassword verifies a temporary payment password without
// consuming it. Telegram temporary passwords remain usable until expiry so a
// client can retry a payment after an ambiguous provider response.
func ValidateTmpPassword(ctx context.Context, userID int64, token []byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || len(token) == 0 {
		return false, nil
	}
	digest := sha256.Sum256(token)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var stored []byte
	var validUntil int64
	err = tx.QueryRowContext(ctx, `SELECT token_digest, valid_until
		FROM apifull_tmp_password WHERE user_id=$1 FOR UPDATE`, userID).
		Scan(&stored, &validUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	if validUntil <= time.Now().Unix() {
		if _, err = tx.ExecContext(ctx, `DELETE FROM apifull_tmp_password WHERE user_id=$1`, userID); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	ok := len(stored) == len(digest) && subtle.ConstantTimeCompare(stored, digest[:]) == 1
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return ok, nil
}
