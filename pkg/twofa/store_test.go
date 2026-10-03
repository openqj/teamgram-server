package twofa

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestMySQLProofStoreCompareAndDelete(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if !strings.Contains(dsn, "/teamgram_audit?") {
		t.Skip("requires the isolated teamgram_audit database")
	}
	store, err := OpenMySQLProofStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	mysqlStore := store.(*mysqlProofStore)
	key := "acct:0:srp:test-" + time.Now().Format("20060102150405.000000000")
	if _, err = mysqlStore.db.Exec(`INSERT INTO apifull_kv (k, v) VALUES (?, ?)`, key, "challenge"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = mysqlStore.db.Exec(`DELETE FROM apifull_kv WHERE k = ?`, key) })

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
