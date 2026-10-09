package domain

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func requirePaymentLedgerDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open isolated payment database: %v", err)
	}
	t.Cleanup(func() { _ = Close() })
}

func TestPaymentLedgerStateTransitionsAreDurableAndIdempotent(t *testing.T) {
	requirePaymentLedgerDB(t)
	userID := time.Now().UnixNano()
	requestKey := fmt.Sprintf("payment-ledger-%d", userID)
	fingerprint := "invoice-fingerprint"
	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM apifull_payment_receipt WHERE user_id=$1`, userID)
		_, _ = db.Exec(`DELETE FROM apifull_payment_ledger WHERE user_id=$1`, userID)
		_, _ = db.Exec(`DELETE FROM apifull_payment_request WHERE user_id=$1`, userID)
	}
	t.Cleanup(cleanup)

	request, err := BeginPaymentRequest(userID, requestKey, "test", fingerprint, "USD", 499, userID, 7)
	if err != nil {
		t.Fatalf("begin payment: %v", err)
	}
	replayed, err := BeginPaymentRequest(userID, requestKey, "test", fingerprint, "USD", 499, userID, 7)
	if err != nil || replayed.ID != request.ID || replayed.State != PaymentStatePending {
		t.Fatalf("replay = %+v, %v; want same pending request", replayed, err)
	}
	if _, err = BeginPaymentRequest(userID, requestKey, "test", "different", "USD", 499, userID, 7); !errors.Is(err, ErrPaymentRequestConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrPaymentRequestConflict", err)
	}

	if _, _, err = SettlePaymentRequest(userID, requestKey, fingerprint, "tx-1", "USD", "Premium", 499, userID, 7, []byte("receipt-1"), false); !errors.Is(err, ErrPaymentUnverified) {
		t.Fatalf("unverified settlement error = %v, want ErrPaymentUnverified", err)
	}
	request, found, err := LoadPaymentRequest(userID, requestKey)
	if err != nil || !found || request.State != PaymentStatePending || request.TransactionID != "" {
		t.Fatalf("after unverified settlement = %+v found=%v err=%v, want unchanged pending", request, found, err)
	}

	settled, receipt, err := SettlePaymentRequest(userID, requestKey, fingerprint, "tx-1", "USD", "Premium", 499, userID, 7, []byte("receipt-1"), true)
	if err != nil || settled.State != PaymentStateSettled || receipt.TransactionID != "tx-1" {
		t.Fatalf("settle = request=%+v receipt=%+v err=%v", settled, receipt, err)
	}
	replayed, receiptAgain, err := SettlePaymentRequest(userID, requestKey, fingerprint, "tx-1", "USD", "Premium", 499, userID, 7, []byte("receipt-1"), true)
	if err != nil || replayed.ID != settled.ID || receiptAgain.RequestID != receipt.RequestID {
		t.Fatalf("settlement replay = request=%+v receipt=%+v err=%v", replayed, receiptAgain, err)
	}
	if _, _, err = SettlePaymentRequest(userID, requestKey, fingerprint, "tx-1", "USD", "Premium", 499, userID, 7, []byte("different-receipt"), true); !errors.Is(err, ErrPaymentRequestConflict) {
		t.Fatalf("conflicting settlement error = %v, want ErrPaymentRequestConflict", err)
	}
	loaded, found, err := LoadPaymentReceiptByMessage(userID, userID, 7)
	if err != nil || !found || loaded.TransactionID != "tx-1" || string(loaded.Receipt) != "receipt-1" {
		t.Fatalf("receipt lookup = %+v found=%v err=%v", loaded, found, err)
	}

	rejectKey := requestKey + ":reject"
	rejectFingerprint := "reject-fingerprint"
	if _, err = BeginPaymentRequest(userID, rejectKey, "test", rejectFingerprint, "USD", 1, userID, 8); err != nil {
		t.Fatalf("begin reject request: %v", err)
	}
	rejected, err := RejectPaymentRequest(userID, rejectKey, rejectFingerprint, "provider unavailable")
	if err != nil || rejected.State != PaymentStateRejected {
		t.Fatalf("reject = %+v err=%v", rejected, err)
	}
	if _, err = RejectPaymentRequest(userID, rejectKey, rejectFingerprint, "provider unavailable"); err != nil {
		t.Fatalf("idempotent reject = %v", err)
	}
	if _, _, err = SettlePaymentRequest(userID, rejectKey, rejectFingerprint, "tx-reject", "USD", "", 1, userID, 8, []byte("receipt-reject"), true); !errors.Is(err, ErrPaymentRequestState) {
		t.Fatalf("settle rejected request error = %v, want ErrPaymentRequestState", err)
	}

	// Slug invoices do not carry a local quote. The provider's verified terms
	// must still settle and replay idempotently.
	slugKey := requestKey + ":slug"
	slugFingerprint := "slug-fingerprint"
	if _, err = BeginPaymentRequest(userID, slugKey, "test", slugFingerprint, "", 0, userID, 9); err != nil {
		t.Fatalf("begin provider-quoted request: %v", err)
	}
	quoted, quotedReceipt, err := SettlePaymentRequest(userID, slugKey, slugFingerprint, "tx-slug", "USD", "Provider quote", 799, userID, 9, []byte("receipt-slug"), true)
	if err != nil || quoted.State != PaymentStateSettled || quotedReceipt.Currency != "USD" || quotedReceipt.Amount != 799 {
		t.Fatalf("provider-quoted settle = request=%+v receipt=%+v err=%v", quoted, quotedReceipt, err)
	}
	if _, _, err = SettlePaymentRequest(userID, slugKey, slugFingerprint, "tx-slug", "USD", "Provider quote", 799, userID, 9, []byte("receipt-slug"), true); err != nil {
		t.Fatalf("provider-quoted settlement replay: %v", err)
	}
}
