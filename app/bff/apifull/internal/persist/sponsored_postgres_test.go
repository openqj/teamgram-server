package persist

import (
	"context"
	"errors"
	"os"
	"testing"
)

func openSponsoredPostgres(t *testing.T) *postgresStore {
	t.Helper()
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ClosePostgres() })
	store, ok := Default.(*postgresStore)
	if !ok || store == nil {
		t.Fatal("PostgreSQL store was not installed")
	}
	return store
}

func TestSponsoredPostgresPreferenceAndEventIdempotency(t *testing.T) {
	store := openSponsoredPostgres(t)
	ctx := context.Background()
	const userID int64 = 981001
	randomID := []byte{1, 2, 3, 4}
	_, _ = store.db.Exec(`DELETE FROM apifull_sponsored_event WHERE user_id=$1`, userID)
	_, _ = store.db.Exec(`DELETE FROM apifull_sponsored_user_setting WHERE user_id=$1`, userID)

	if err := SetSponsoredMessagesEnabled(ctx, userID, true); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := store.db.QueryRow(`SELECT enabled FROM apifull_sponsored_user_setting WHERE user_id=$1`, userID).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("sponsored preference was not enabled")
	}
	payload := map[string]any{"media": true, "placement": "feed"}
	if err := RecordSponsoredEvent(ctx, userID, "message_view", randomID, payload); err != nil {
		t.Fatal(err)
	}
	if err := RecordSponsoredEvent(ctx, userID, "message_view", randomID, payload); err != nil {
		t.Fatalf("idempotent retry failed: %v", err)
	}
	if err := RecordSponsoredEvent(ctx, userID, "message_view", randomID, map[string]any{"media": false}); !errors.Is(err, ErrSponsoredEventConflict) {
		t.Fatalf("conflicting retry error = %v, want ErrSponsoredEventConflict", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM apifull_sponsored_event WHERE user_id=$1 AND event_type='message_view' AND random_id=$2`, userID, randomID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("event count = %d, want 1", count)
	}
}

func TestSponsoredPostgresChannelPolicyAuthorization(t *testing.T) {
	store := openSponsoredPostgres(t)
	ctx := context.Background()
	const channelID, creatorID, outsiderID, accessHash int64 = 981002, 981003, 981004, 778899
	_, _ = store.db.Exec(`DELETE FROM apifull_sponsored_channel_policy WHERE channel_id=$1`, channelID)
	_, _ = store.db.Exec(`DELETE FROM apifull_channel WHERE id=$1`, channelID)
	if _, err := store.db.Exec(`INSERT INTO apifull_channel (id, access_hash, creator_user_id, title, created_at) VALUES ($1,$2,$3,'sponsored-test',1)`, channelID, accessHash, creatorID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_sponsored_channel_policy WHERE channel_id=$1`, channelID)
		_, _ = store.db.Exec(`DELETE FROM apifull_channel WHERE id=$1`, channelID)
	})
	if err := SetSponsoredChannelRestriction(ctx, outsiderID, channelID, accessHash, true); !errors.Is(err, ErrSponsoredChannelUnauthorized) {
		t.Fatalf("outsider error = %v, want ErrSponsoredChannelUnauthorized", err)
	}
	if err := SetSponsoredChannelRestriction(ctx, creatorID, channelID, accessHash+1, true); !errors.Is(err, ErrSponsoredChannelAccessHash) {
		t.Fatalf("wrong hash error = %v, want ErrSponsoredChannelAccessHash", err)
	}
	if err := SetSponsoredChannelRestriction(ctx, creatorID, channelID, accessHash, true); err != nil {
		t.Fatal(err)
	}
	var restricted bool
	if err := store.db.QueryRow(`SELECT restricted FROM apifull_sponsored_channel_policy WHERE channel_id=$1`, channelID).Scan(&restricted); err != nil {
		t.Fatal(err)
	}
	if !restricted {
		t.Fatal("channel restriction was not persisted")
	}
}
