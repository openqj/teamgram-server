package persist

import (
	"os"
	"testing"
	"time"
)

func TestBotAccessSettingsPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	store, ok := Default.(*postgresStore)
	if !ok {
		t.Fatal("postgres store was not installed")
	}
	botID := time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_bot_access_settings WHERE bot_user_id = $1`, botID)
		_ = ClosePostgres()
		Default = previous
	})

	initial, err := GetBotAccessSettings(t.Context(), botID)
	if err != nil || initial.Restricted || len(initial.AddUserIDs) != 0 {
		t.Fatalf("default settings = %#v, %v", initial, err)
	}
	updated, err := EditBotAccessSettings(t.Context(), botID, true, []int64{11, 12})
	if err != nil || !updated.Restricted || len(updated.AddUserIDs) != 2 {
		t.Fatalf("updated settings = %#v, %v", updated, err)
	}
	preserved, err := EditBotAccessSettings(t.Context(), botID, false, nil)
	if err != nil || preserved.Restricted || len(preserved.AddUserIDs) != 2 || preserved.AddUserIDs[0] != 11 || preserved.AddUserIDs[1] != 12 {
		t.Fatalf("preserved users = %#v, %v", preserved, err)
	}
	cleared, err := EditBotAccessSettings(t.Context(), botID, false, []int64{})
	if err != nil || len(cleared.AddUserIDs) != 0 {
		t.Fatalf("cleared users = %#v, %v", cleared, err)
	}
}
