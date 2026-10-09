package domain

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const StarsRefundStateComplete = "complete"

var (
	ErrStarsChargeNotFound = errors.New("stars charge not found")
	ErrStarsChargeInvalid  = errors.New("invalid stars charge")
	ErrStarsRefundConflict = errors.New("stars refund conflicts with existing refund")
)

// StarsCharge is the provider-settled Stars purchase that can be refunded.
// Stars is recovered from the immutable credit ledger rather than trusting a
// client supplied amount or a mutable provider response.
type StarsCharge struct {
	RequestID     int64
	UserID        int64
	Provider      string
	TransactionID string
	Currency      string
	Amount        int64
	Title         string
	Stars         int64
}

type StarsRefund struct {
	ID                  int64
	UserID              int64
	ChargeID            string
	Provider            string
	Stars               int64
	RefundTransactionID string
	State               string
	Receipt             []byte
	CreatedAt           int64
	UpdatedAt           int64
}

func starsPaymentCreditIdem(provider, transactionID string) string {
	digest := sha256.Sum256([]byte(provider + "\x00" + transactionID))
	return "payment:" + hex.EncodeToString(digest[:])
}

func starsRefundIdem(provider, chargeID string) string {
	digest := sha256.Sum256([]byte(provider + "\x00" + chargeID))
	return "refund:" + hex.EncodeToString(digest[:])
}

// LoadStarsCharge returns a caller-owned, provider-settled charge. A receipt
// without a matching positive Stars credit is treated as corrupt and is not
// eligible for refund.
func LoadStarsCharge(userID int64, chargeID string) (StarsCharge, bool, error) {
	if db == nil {
		return StarsCharge{}, false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(chargeID) == "" || len(chargeID) > 191 {
		return StarsCharge{}, false, ErrStarsChargeInvalid
	}
	var charge StarsCharge
	err := db.QueryRow(`SELECT request_id, user_id, provider, transaction_id, currency, amount, title
		FROM apifull_payment_receipt WHERE user_id=$1 AND transaction_id=$2`, userID, chargeID).
		Scan(&charge.RequestID, &charge.UserID, &charge.Provider, &charge.TransactionID, &charge.Currency, &charge.Amount, &charge.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return StarsCharge{}, false, nil
	}
	if err != nil {
		return StarsCharge{}, false, err
	}
	if charge.Provider == "" || charge.TransactionID == "" {
		return StarsCharge{}, false, ErrStarsChargeInvalid
	}
	if err = db.QueryRow(`SELECT amount FROM apifull_star_tx WHERE user_id=$1 AND idem=$2`, userID, starsPaymentCreditIdem(charge.Provider, charge.TransactionID)).Scan(&charge.Stars); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StarsCharge{}, false, ErrStarsChargeInvalid
		}
		return StarsCharge{}, false, err
	}
	if charge.Stars <= 0 {
		return StarsCharge{}, false, ErrStarsChargeInvalid
	}
	return charge, true, nil
}

func LoadStarsRefund(userID int64, chargeID string) (StarsRefund, bool, error) {
	if db == nil {
		return StarsRefund{}, false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(chargeID) == "" || len(chargeID) > 191 {
		return StarsRefund{}, false, ErrStarsChargeInvalid
	}
	var refund StarsRefund
	err := db.QueryRow(`SELECT id, user_id, charge_id, provider, stars, refund_transaction_id, state, receipt, created_at, updated_at
		FROM apifull_stars_refund WHERE user_id=$1 AND charge_id=$2`, userID, chargeID).
		Scan(&refund.ID, &refund.UserID, &refund.ChargeID, &refund.Provider, &refund.Stars, &refund.RefundTransactionID, &refund.State, &refund.Receipt, &refund.CreatedAt, &refund.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StarsRefund{}, false, nil
	}
	if err != nil {
		return StarsRefund{}, false, err
	}
	return refund, true, nil
}

// CommitStarsRefund atomically records provider-approved refund state and
// debits the originally credited Stars. It is safe to retry after a process
// crash between provider approval and database commit.
func CommitStarsRefund(userID int64, chargeID, provider, refundTransactionID string, stars int64, receipt []byte) (StarsRefund, error) {
	if db == nil {
		return StarsRefund{}, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(chargeID) == "" || len(chargeID) > 191 || strings.TrimSpace(provider) == "" || len(provider) > 32 || strings.TrimSpace(refundTransactionID) == "" || len(refundTransactionID) > 191 || stars <= 0 || len(receipt) == 0 {
		return StarsRefund{}, ErrStarsChargeInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return StarsRefund{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var chargeUser int64
	var chargeProvider, transactionID string
	err = tx.QueryRow(`SELECT user_id, provider, transaction_id FROM apifull_payment_receipt
		WHERE user_id=$1 AND transaction_id=$2 FOR UPDATE`, userID, chargeID).Scan(&chargeUser, &chargeProvider, &transactionID)
	if errors.Is(err, sql.ErrNoRows) {
		return StarsRefund{}, ErrStarsChargeNotFound
	}
	if err != nil {
		return StarsRefund{}, err
	}
	if chargeUser != userID || chargeProvider == "" || transactionID != chargeID {
		return StarsRefund{}, ErrStarsChargeInvalid
	}
	var existing StarsRefund
	err = tx.QueryRow(`SELECT id, user_id, charge_id, provider, stars, refund_transaction_id, state, receipt, created_at, updated_at
		FROM apifull_stars_refund WHERE user_id=$1 AND charge_id=$2 FOR UPDATE`, userID, chargeID).
		Scan(&existing.ID, &existing.UserID, &existing.ChargeID, &existing.Provider, &existing.Stars, &existing.RefundTransactionID, &existing.State, &existing.Receipt, &existing.CreatedAt, &existing.UpdatedAt)
	existingFound := err == nil
	if err == nil {
		if existing.Provider != provider || existing.Stars != stars || existing.RefundTransactionID != refundTransactionID {
			return StarsRefund{}, ErrStarsRefundConflict
		}
		if existing.State == StarsRefundStateComplete {
			if err = tx.Commit(); err != nil {
				return StarsRefund{}, err
			}
			return existing, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return StarsRefund{}, err
	}
	var credited int64
	if err = tx.QueryRow(`SELECT amount FROM apifull_star_tx WHERE user_id=$1 AND idem=$2 FOR UPDATE`, userID, starsPaymentCreditIdem(chargeProvider, transactionID)).Scan(&credited); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StarsRefund{}, ErrStarsChargeInvalid
		}
		return StarsRefund{}, err
	}
	if credited <= 0 || credited != stars {
		return StarsRefund{}, ErrStarsChargeInvalid
	}

	now := time.Now().Unix()
	if !existingFound {
		if _, err = tx.Exec(`INSERT INTO apifull_stars_refund
			(user_id, charge_id, provider, stars, refund_transaction_id, state, receipt, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, userID, chargeID, provider, stars, refundTransactionID, StarsRefundStateComplete, receipt, now, now); err != nil {
			return StarsRefund{}, err
		}
	} else {
		if _, err = tx.Exec(`UPDATE apifull_stars_refund SET refund_transaction_id=$1, state=$2, receipt=$3, updated_at=$4
			WHERE user_id=$5 AND charge_id=$6`, refundTransactionID, StarsRefundStateComplete, receipt, now, userID, chargeID); err != nil {
			return StarsRefund{}, err
		}
	}
	if _, err = applyStarsTx(tx, userID, -stars, starsRefundIdem(provider, chargeID)); err != nil {
		return StarsRefund{}, err
	}
	if err = tx.Commit(); err != nil {
		return StarsRefund{}, err
	}
	// Read the committed row back so the generated identity is returned on the
	// first call as well as on idempotent retries.
	committed, found, err := LoadStarsRefund(userID, chargeID)
	if err != nil {
		return StarsRefund{}, err
	}
	if !found {
		return StarsRefund{}, ErrStarsRefundConflict
	}
	return committed, nil
}
