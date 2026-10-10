package persist

import (
	"os"
	"sync"
	"testing"
)

func TestURLAuthPostgresIsAtomicScopedAndIdempotent(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	store, ok := Default.(*postgresStore)
	if !ok {
		t.Fatal("postgres store was not installed")
	}
	userID := int64(781000000 + os.Getpid())
	otherUserID := userID + 1
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_url_auth WHERE owner_user_id IN ($1,$2)`, userID, otherUserID)
		_ = ClosePostgres()
		Default = previous
	})

	const rawURL = "https://example.invalid/auth"
	const hash int64 = 991001
	if err := RequestURLAuth(userID, URLAuthRecord{Hash: hash, URL: rawURL, BotID: 42}); err != nil {
		t.Fatal(err)
	}
	if matched, err := CheckURLAuthMatchCode(userID, rawURL, "unused"); err != nil || matched {
		t.Fatalf("pending match check: matched=%v err=%v", matched, err)
	}

	accepted, err := AcceptURLAuth(userID, URLAuthRecord{Hash: hash, URL: rawURL, MatchCode: "match", BotID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != "accepted" || accepted.URL != rawURL || accepted.Hash != hash {
		t.Fatalf("accepted record = %#v", accepted)
	}
	// A retry must update one row, retaining a single authorization.
	if _, err = AcceptURLAuth(userID, URLAuthRecord{Hash: hash, URL: rawURL, MatchCode: "match", BotID: 42}); err != nil {
		t.Fatal(err)
	}
	items, err := ListURLAuth(userID)
	if err != nil || len(items) != 1 {
		t.Fatalf("accepted list: len=%d err=%v", len(items), err)
	}
	if matched, err := CheckURLAuthMatchCode(userID, rawURL, "match"); err != nil || !matched {
		t.Fatalf("accepted match check: matched=%v err=%v", matched, err)
	}
	if err := RequestURLAuth(userID, URLAuthRecord{Hash: hash, URL: rawURL, MatchCode: "new-request", BotID: 99}); err != nil {
		t.Fatal(err)
	}
	items, err = ListURLAuth(userID)
	if err != nil || len(items) != 1 || items[0].MatchCode != "match" || items[0].BotID != 42 {
		t.Fatalf("repeat request changed accepted authorization: items=%#v err=%v", items, err)
	}
	if matched, err := CheckURLAuthMatchCode(otherUserID, rawURL, "match"); err != nil || matched {
		t.Fatalf("cross-user match check: matched=%v err=%v", matched, err)
	}

	// Concurrent accepts serialize on the caller lock and cannot duplicate rows.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, callErr := AcceptURLAuth(userID, URLAuthRecord{Hash: hash, URL: rawURL, MatchCode: "match", BotID: int64(42 + i)})
			if callErr != nil {
				t.Errorf("concurrent accept %d: %v", i, callErr)
			}
		}(i)
	}
	wg.Wait()
	items, err = ListURLAuth(userID)
	if err != nil || len(items) != 1 {
		t.Fatalf("concurrent accepted list: len=%d err=%v", len(items), err)
	}

	if err := DeclineURLAuth(userID, hash, rawURL); err != nil {
		t.Fatal(err)
	}
	items, err = ListURLAuth(userID)
	if err != nil || len(items) != 0 {
		t.Fatalf("declined list: len=%d err=%v", len(items), err)
	}
}
