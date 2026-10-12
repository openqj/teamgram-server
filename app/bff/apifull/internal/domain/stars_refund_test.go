package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestPostgresStarsRefundIsAtomicAndIdempotent(t *testing.T) {
	userID := paymentPostgresFixture(t)
	key := fmt.Sprintf("stars-refund-%d", userID)
	fingerprint := "stars-refund-fingerprint"
	chargeID := fmt.Sprintf("stars-charge-%d", userID)
	request, err := BeginPaymentRequest(userID, key, "stars-provider", fingerprint, "USD", 199, 0, 0)
	if err != nil {
		t.Fatalf("begin Stars payment: %v", err)
	}
	if _, _, err = SettleStarsPaymentRequest(userID, key, fingerprint, chargeID, "USD", "Stars top-up", 199, 25, []byte(`{"provider":"verified"}`)); err != nil {
		t.Fatalf("settle Stars payment: %v", err)
	}
	charge, found, err := LoadStarsCharge(userID, chargeID)
	if err != nil || !found || charge.RequestID != request.ID || charge.Stars != 25 {
		t.Fatalf("charge = %+v found=%v err=%v", charge, found, err)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 25 {
		t.Fatalf("balance before refund = %d err=%v", balance, err)
	}

	refundTransactionID := fmt.Sprintf("refund-%d", userID)
	first, err := CommitStarsRefund(userID, chargeID, "stars-provider", refundTransactionID, 25, []byte(`{"refund":true}`))
	if err != nil || first.State != StarsRefundStateComplete || first.Stars != 25 {
		t.Fatalf("first refund = %+v err=%v", first, err)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 0 {
		t.Fatalf("balance after refund = %d err=%v", balance, err)
	}
	second, err := CommitStarsRefund(userID, chargeID, "stars-provider", refundTransactionID, 25, []byte(`{"refund":true}`))
	if err != nil || second.ID != first.ID || second.RefundTransactionID != first.RefundTransactionID {
		t.Fatalf("idempotent refund = %+v err=%v first=%+v", second, err, first)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 0 {
		t.Fatalf("balance after replay = %d err=%v", balance, err)
	}
	if _, err = CommitStarsRefund(userID, chargeID, "stars-provider", refundTransactionID+"-conflict", 24, []byte(`{"refund":true}`)); !errors.Is(err, ErrStarsRefundConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrStarsRefundConflict", err)
	}
	if _, err = CommitStarsRefund(userID, chargeID+"-missing", "stars-provider", "refund-3", 25, []byte(`{"refund":true}`)); !errors.Is(err, ErrStarsChargeNotFound) {
		t.Fatalf("unknown charge error = %v, want ErrStarsChargeNotFound", err)
	}
}

func TestPostgresStarsRefundRejectsProviderMismatch(t *testing.T) {
	userID := paymentPostgresFixture(t)
	key := fmt.Sprintf("stars-refund-provider-%d", userID)
	chargeID := fmt.Sprintf("stars-charge-provider-%d", userID)
	if _, err := BeginPaymentRequest(userID, key, "stars-provider", "provider-fingerprint", "USD", 199, 0, 0); err != nil {
		t.Fatalf("begin Stars payment: %v", err)
	}
	if _, _, err := SettleStarsPaymentRequest(userID, key, "provider-fingerprint", chargeID, "USD", "Stars top-up", 199, 25, []byte(`{"provider":"verified"}`)); err != nil {
		t.Fatalf("settle Stars payment: %v", err)
	}
	if _, err := CommitStarsRefund(userID, chargeID, "forged-provider", "refund-provider-mismatch", 25, []byte(`{"refund":true}`)); !errors.Is(err, ErrStarsChargeInvalid) {
		t.Fatalf("provider mismatch error = %v, want ErrStarsChargeInvalid", err)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 25 {
		t.Fatalf("balance after rejected refund = %d err=%v, want 25", balance, err)
	}
	if _, found, err := LoadStarsRefund(userID, chargeID); err != nil || found {
		t.Fatalf("refund row after rejected refund = found=%v err=%v, want absent", found, err)
	}
}
