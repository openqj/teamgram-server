package persist

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
)

var errStateValidation = errors.New("state validation failed")

func openStatePostgres(t *testing.T) *postgresStore {
	t.Helper()
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ClosePostgres()
		Default = previous
	})
	store, ok := Default.(*postgresStore)
	if !ok {
		t.Fatal("postgres store was not installed")
	}
	return store
}

func TestWallpaperListMutationIsTransactional(t *testing.T) {
	store := openStatePostgres(t)
	const userID int64 = 780001
	_, _ = store.db.Exec(`DELETE FROM apifull_wallpaper_state WHERE user_id = $1`, userID)
	const workers = 8
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := MutateWallpaperList(userID, false, func(raw string) (string, error) {
				var values []int
				if raw != "" {
					if err := json.Unmarshal([]byte(raw), &values); err != nil {
						return "", err
					}
				}
				values = append(values, i)
				encoded, err := json.Marshal(values)
				return string(encoded), err
			}); err != nil {
				t.Errorf("mutation %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	raw, err := LoadWallpaperList(userID, false)
	if err != nil {
		t.Fatal(err)
	}
	var values []int
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != workers {
		t.Fatalf("wallpaper mutation lost rows: got %d want %d (%s)", len(values), workers, raw)
	}
}

func TestTakeoutMutationAllocatesStableIdentity(t *testing.T) {
	store := openStatePostgres(t)
	const userID int64 = 780002
	_, _ = store.db.Exec(`DELETE FROM apifull_takeout_session WHERE user_id = $1`, userID)
	id, _, err := MutateTakeout(userID, func(id int64, current string) (string, error) {
		if id <= 0 || current != "" {
			t.Fatalf("first takeout state: id=%d current=%q", id, current)
		}
		return `{"active":true}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, current, err := MutateTakeout(userID, func(gotID int64, current string) (string, error) {
		var state map[string]bool
		if err := json.Unmarshal([]byte(current), &state); err != nil || !state["active"] {
			t.Fatalf("second takeout payload: %q (err=%v)", current, err)
		}
		if gotID != id {
			t.Fatalf("second takeout state: id=%d current=%q", gotID, current)
		}
		return current, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]bool
	if err := json.Unmarshal([]byte(current), &state); err != nil || !state["active"] {
		t.Fatalf("takeout payload changed: %q (err=%v)", current, err)
	}
	if id2 != id {
		t.Fatalf("takeout identity changed: first=%d second=%d payload=%q", id, id2, current)
	}
	if consumed, err := ConsumeTakeout(userID, func(string) error { return errStateValidation }); consumed || err != errStateValidation {
		t.Fatalf("takeout rollback: consumed=%v err=%v", consumed, err)
	}
	if got, err := LoadTakeout(userID); err != nil || got == "" {
		t.Fatalf("takeout rollback lost state: value=%q err=%v", got, err)
	}
	if consumed, err := ConsumeTakeout(userID, func(string) error { return nil }); err != nil || !consumed {
		t.Fatalf("takeout consume: consumed=%v err=%v", consumed, err)
	}
	if got, err := LoadTakeout(userID); err != nil || got != "" {
		t.Fatalf("takeout delete: value=%q err=%v", got, err)
	}
}

func TestMessageSplitRangesUseAuthoritativeMessages(t *testing.T) {
	store := openStatePostgres(t)
	const userID int64 = 780003
	_, _ = store.db.Exec(`DELETE FROM messages WHERE user_id = $1`, userID)
	for _, id := range []int{1, 100001, 200005} {
		if _, err := store.db.Exec(`INSERT INTO messages(user_id, user_message_box_id, sender_user_id)
			VALUES ($1, $2, $1)`, userID, id); err != nil {
			t.Fatal(err)
		}
	}
	ranges, err := GetMessageSplitRanges(userID)
	if err != nil {
		t.Fatal(err)
	}
	want := []MessageSplitRange{{MinID: 1, MaxID: 100000}, {MinID: 100001, MaxID: 200000}, {MinID: 200001, MaxID: 200005}}
	if len(ranges) != len(want) {
		t.Fatalf("ranges=%v want=%v", ranges, want)
	}
	for i := range want {
		if ranges[i] != want[i] {
			t.Fatalf("ranges=%v want=%v", ranges, want)
		}
	}
	_, _ = store.db.Exec(`DELETE FROM messages WHERE user_id = $1`, userID)
}
