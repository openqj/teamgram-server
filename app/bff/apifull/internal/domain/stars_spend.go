package domain

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	StarsSpendGift             = "star_gift"
	StarsSpendPremiumGift      = "premium_gift"
	StarsSpendBusinessTransfer = "business_stars_transfer"
)

// SettleStarsSpendRequest commits a locally funded Layer 229 Stars purchase.
// The payment request must have been created by BeginPaymentRequest with the
// same payer, quote and fingerprint. All balance and entitlement mutations
// happen in this transaction so a retry cannot spend the same Stars twice.
func SettleStarsSpendRequest(payerID, beneficiaryID, recipientID int64, requestKey, fingerprint, transactionID, currency, title string, amount, stars int64, receipt []byte, kind, giftSlug string, premiumMonths int32) (PaymentRequest, PaymentReceipt, error) {
	if db == nil {
		return PaymentRequest{}, PaymentReceipt{}, errors.New("domain PostgreSQL is not open")
	}
	if payerID <= 0 || beneficiaryID < 0 || recipientID < 0 || strings.TrimSpace(requestKey) == "" || strings.TrimSpace(fingerprint) == "" || strings.TrimSpace(transactionID) == "" || len(transactionID) > 191 || strings.TrimSpace(currency) == "" || strings.TrimSpace(title) == "" || amount < 0 || amount != stars || stars <= 0 || len(receipt) == 0 {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidPaymentRequest
	}
	if kind != StarsSpendGift && kind != StarsSpendPremiumGift && kind != StarsSpendBusinessTransfer {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidPaymentRequest
	}
	if kind == StarsSpendGift && (beneficiaryID != 0 || recipientID <= 0 || strings.TrimSpace(giftSlug) == "" || premiumMonths != 0) {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidPaymentRequest
	}
	if kind == StarsSpendPremiumGift && (beneficiaryID != 0 || recipientID <= 0 || (premiumMonths != 3 && premiumMonths != 6 && premiumMonths != 12) || giftSlug != "") {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidPaymentRequest
	}
	if kind == StarsSpendBusinessTransfer && (beneficiaryID <= 0 || beneficiaryID == payerID || recipientID != 0 || premiumMonths != 0 || giftSlug != "") {
		return PaymentRequest{}, PaymentReceipt{}, ErrInvalidPaymentRequest
	}

	tx, err := db.Begin()
	if err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	request, err := loadPaymentRequestTx(tx, payerID, requestKey, true)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestNotFound
	}
	if err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	if request.Fingerprint != fingerprint || request.Currency != currency || request.Amount != amount || request.PeerID != 0 || request.MsgID != 0 {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestConflict
	}
	if request.State == PaymentStateSettled {
		stored, receiptErr := loadPaymentReceiptTx(tx, request.ID)
		if receiptErr != nil {
			return PaymentRequest{}, PaymentReceipt{}, receiptErr
		}
		if stored.TransactionID != transactionID || !equalHash(stored.Receipt, receipt) {
			return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestConflict
		}
		if err = ensureStarsSpendSideEffectsTx(tx, request.ID, payerID, beneficiaryID, recipientID, requestKey, request.Provider, transactionID, stars, kind, giftSlug, premiumMonths); err != nil {
			return PaymentRequest{}, PaymentReceipt{}, err
		}
		if err = tx.Commit(); err != nil {
			return PaymentRequest{}, PaymentReceipt{}, err
		}
		return request, stored, nil
	}
	if request.State != PaymentStatePending {
		return PaymentRequest{}, PaymentReceipt{}, ErrPaymentRequestState
	}
	now := time.Now().Unix()
	receiptHash := sha256.Sum256(receipt)
	result, err := tx.Exec(`INSERT INTO apifull_payment_receipt
		(request_id, user_id, provider, transaction_id, currency, amount, peer_id, msg_id, title, receipt, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,0,0,$7,$8,$9) ON CONFLICT DO NOTHING`, request.ID, payerID, request.Provider, transactionID, currency, amount, title, receipt, now)
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
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, request.ID, payerID, PaymentStateSettled, request.Provider, transactionID, currency, amount, hex.EncodeToString(receiptHash[:]), now); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	if err = ensureStarsSpendSideEffectsTx(tx, request.ID, payerID, beneficiaryID, recipientID, requestKey, request.Provider, transactionID, stars, kind, giftSlug, premiumMonths); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	request.State, request.TransactionID, request.UpdatedAt = PaymentStateSettled, transactionID, now
	stored := PaymentReceipt{RequestID: request.ID, UserID: payerID, Provider: request.Provider, TransactionID: transactionID, Currency: currency, Amount: amount, Title: title, Receipt: append([]byte(nil), receipt...), CreatedAt: now}
	if err = tx.Commit(); err != nil {
		return PaymentRequest{}, PaymentReceipt{}, err
	}
	return request, stored, nil
}

func ensureStarsSpendSideEffectsTx(tx *sql.Tx, requestID, payerID, beneficiaryID, recipientID int64, requestKey, provider, transactionID string, stars int64, kind, giftSlug string, premiumMonths int32) error {
	if tx == nil || requestID <= 0 || payerID <= 0 || strings.TrimSpace(requestKey) == "" || strings.TrimSpace(provider) == "" || strings.TrimSpace(transactionID) == "" || stars <= 0 {
		return ErrInvalidPaymentRequest
	}
	debitKey := starsSpendIdempotencyKey(requestKey, "debit")
	creditKey := starsSpendIdempotencyKey(requestKey, "credit")
	if kind == StarsSpendBusinessTransfer {
		// Lock balances in a stable order to avoid deadlocks between opposing
		// transfers while retaining the signed payer/beneficiary directions.
		if payerID < beneficiaryID {
			if _, err := applyStarsTx(tx, payerID, -stars, debitKey); err != nil {
				return err
			}
			_, err := applyStarsTx(tx, beneficiaryID, stars, creditKey)
			return err
		}
		if _, err := applyStarsTx(tx, beneficiaryID, stars, creditKey); err != nil {
			return err
		}
		_, err := applyStarsTx(tx, payerID, -stars, debitKey)
		return err
	}
	if _, err := applyStarsTx(tx, payerID, -stars, debitKey); err != nil {
		return err
	}
	switch kind {
	case StarsSpendGift:
		result, err := tx.Exec(`INSERT INTO apifull_gift
			(from_user, to_user, slug, stars, saved, payment_request_id)
			VALUES ($1,$2,$3,$4,1,$5) ON CONFLICT (payment_request_id) DO NOTHING`, payerID, recipientID, giftSlug, stars, requestID)
		if err != nil {
			return err
		}
		if inserted, insertErr := result.RowsAffected(); insertErr != nil {
			return insertErr
		} else if inserted == 0 {
			var from, to, value int64
			var slug string
			if err = tx.QueryRow(`SELECT from_user, to_user, slug, stars FROM apifull_gift WHERE payment_request_id=$1 FOR UPDATE`, requestID).Scan(&from, &to, &slug, &value); err != nil {
				return err
			}
			if from != payerID || to != recipientID || slug != giftSlug || value != stars {
				return ErrPaymentRequestConflict
			}
		}
	case StarsSpendPremiumGift:
		if err := insertPremiumGrantTx(tx, requestID, recipientID, provider, transactionID, premiumMonths); err != nil {
			return err
		}
	}
	return nil
}

func starsSpendIdempotencyKey(requestKey, direction string) string {
	return "payment:" + direction + ":" + shortHash(requestKey)
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
