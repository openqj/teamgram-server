package persist

import (
	"os"
	"testing"
	"time"
)

func TestUserEmojiStatusPermissionPostgresRoundTrip(t *testing.T) {
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
	ownerID := time.Now().UnixNano()
	botID := ownerID + 1
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_user_emoji_status_permission WHERE owner_user_id = $1 AND bot_user_id = $2`, ownerID, botID)
		_ = ClosePostgres()
		Default = previous
	})

	if enabled, err := UserEmojiStatusPermission(t.Context(), ownerID, botID); err != nil || enabled {
		t.Fatalf("initial permission = %v, %v", enabled, err)
	}
	if err := SetUserEmojiStatusPermission(t.Context(), ownerID, botID, true); err != nil {
		t.Fatal(err)
	}
	if enabled, err := UserEmojiStatusPermission(t.Context(), ownerID, botID); err != nil || !enabled {
		t.Fatalf("enabled permission = %v, %v", enabled, err)
	}
	if err := SetUserEmojiStatusPermission(t.Context(), ownerID, botID, true); err != nil {
		t.Fatal(err)
	}
	if err := SetUserEmojiStatusPermission(t.Context(), ownerID, botID, false); err != nil {
		t.Fatal(err)
	}
	if enabled, err := UserEmojiStatusPermission(t.Context(), ownerID, botID); err != nil || enabled {
		t.Fatalf("disabled permission = %v, %v", enabled, err)
	}
}
