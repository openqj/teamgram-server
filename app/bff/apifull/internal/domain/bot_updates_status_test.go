package domain

import (
	"testing"
	"time"
)

func TestBotUpdatesStatusPostgresRoundTripIsIdempotent(t *testing.T) {
	requirePaymentLedgerDB(t)
	botID := time.Now().UnixNano()
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM apifull_bot_updates_status WHERE bot_user_id=$1`, botID) })
	if err := UpsertBotUpdatesStatus(botID, 4, "waiting"); err != nil {
		t.Fatalf("first status: %v", err)
	}
	if err := UpsertBotUpdatesStatus(botID, 1, "ready"); err != nil {
		t.Fatalf("retry status: %v", err)
	}
	got, found, err := LoadBotUpdatesStatus(botID)
	if err != nil || !found || got.BotUserID != botID || got.PendingUpdatesCount != 1 || got.Message != "ready" {
		t.Fatalf("status = %+v found=%v err=%v", got, found, err)
	}
	if err := UpsertBotUpdatesStatus(botID, -1, "invalid"); err == nil {
		t.Fatal("negative pending count was accepted")
	}
	if _, found, err := LoadBotUpdatesStatus(botID + 1); err != nil || found {
		t.Fatalf("missing status = found=%v err=%v", found, err)
	}
}
