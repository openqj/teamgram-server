package twofa

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestPostgresProofStoreCompareAndDelete(t *testing.T) {
	dsn := os.Getenv("TWOFA_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TWOFA_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	store, err := OpenPostgresProofStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	postgresStore := store.(*postgresProofStore)
	t.Cleanup(func() { _ = ClosePostgresProofStores() })
	key := fmt.Sprintf("acct:0:srp:test-%d", time.Now().UnixNano())
	if _, err = postgresStore.db.Exec(`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)
		ON CONFLICT (k) DO UPDATE SET v=EXCLUDED.v`, key, "challenge"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = postgresStore.db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key) })

	if deleted, err := store.CompareAndDelete(key, "wrong"); err != nil || deleted {
		t.Fatalf("mismatch consume = (%v, %v), want (false, nil)", deleted, err)
	}
	if deleted, err := store.CompareAndDelete(key, "challenge"); err != nil || !deleted {
		t.Fatalf("valid consume = (%v, %v), want (true, nil)", deleted, err)
	}
	if deleted, err := store.CompareAndDelete(key, "challenge"); err != nil || deleted {
		t.Fatalf("replay consume = (%v, %v), want (false, nil)", deleted, err)
	}
}
