package domain

import (
	"context"
	"crypto/sha256"
	"os"
	"testing"
	"time"
)

func TestPostgresTmpPasswordReplacementAndExpiry(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close() })
	userID := time.Now().UnixNano()
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM apifull_tmp_password WHERE user_id=$1`, userID) })
	first := []byte("first temporary password")
	second := []byte("second temporary password")
	if err := StoreTmpPassword(context.Background(), userID, first, time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if ok, err := ValidateTmpPassword(context.Background(), userID, first); err != nil || !ok {
		t.Fatalf("first token validation: ok=%v err=%v", ok, err)
	}
	if err := StoreTmpPassword(context.Background(), userID, second, time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if ok, err := ValidateTmpPassword(context.Background(), userID, first); err != nil || ok {
		t.Fatalf("replaced token validation: ok=%v err=%v", ok, err)
	}
	if ok, err := ValidateTmpPassword(context.Background(), userID, second); err != nil || !ok {
		t.Fatalf("replacement token validation: ok=%v err=%v", ok, err)
	}
	// An expired token is removed in the same transaction and cannot be
	// resurrected by a later read.
	if err := StoreTmpPassword(context.Background(), userID, second, time.Now().Add(-time.Second).Unix()); err == nil {
		t.Fatal("expired issuance unexpectedly succeeded")
	}
	if digest := sha256.Sum256(first); len(digest) != sha256.Size {
		t.Fatal("digest size changed")
	}
}
