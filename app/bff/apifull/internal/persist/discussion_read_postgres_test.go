package persist

import (
	"fmt"
	"os"
	"testing"
)

func TestDiscussionReadCursorPostgresIsMonotonic(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := OpenPostgres(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ClosePostgres() })

	key := fmt.Sprintf("discussion:read:%d:2:91001:17", os.Getpid())
	store, ok := Default.(*postgresStore)
	if !ok {
		t.Fatal("discussion cursor store is not PostgreSQL-backed")
	}
	_, _ = store.db.Exec(`DELETE FROM apifull_discussion_read_state WHERE user_id=$1`, os.Getpid())
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_discussion_read_state WHERE user_id=$1`, os.Getpid())
	})

	if err := AdvanceDiscussionReadMaxID(key, 19); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceDiscussionReadMaxID(key, 7); err != nil {
		t.Fatal(err)
	}
	got, err := GetDiscussionReadMaxID(key)
	if err != nil {
		t.Fatal(err)
	}
	if got != 19 {
		t.Fatalf("discussion read cursor = %d, want 19", got)
	}
}
