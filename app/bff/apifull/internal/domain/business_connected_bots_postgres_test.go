package domain

import (
	"os"
	"testing"
)

func TestPostgresConnectedBotStateRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" || !Ready() {
		t.Skip("APIFULL_POSTGRES_DSN and an open PostgreSQL domain are required")
	}
	ownerID := int64(982000000 + os.Getpid()%100000)
	botID := ownerID + 1
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_connected_bot_peer WHERE owner_user_id=$1`, ownerID)
		_, _ = db.Exec(`DELETE FROM apifull_connected_bot WHERE owner_user_id=$1`, ownerID)
	})
	record := ConnectedBotRecord{BotID: botID, CanReply: true, Rights: []byte(`{"read":true}`), RecipientsBot: []byte(`{"existing_chats":true}`), Recipients: []byte(`{"contacts":true}`)}
	if err := SetConnectedBot(ownerID, record); err != nil {
		t.Fatalf("set connected bot: %v", err)
	}
	rows, err := ListConnectedBots(ownerID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list connected bots: rows=%d err=%v", len(rows), err)
	}
	if rows[0].BotID != botID || !rows[0].CanReply || string(rows[0].Rights) != string(record.Rights) || string(rows[0].RecipientsBot) != string(record.RecipientsBot) || string(rows[0].Recipients) != string(record.Recipients) {
		t.Fatalf("connected bot mismatch: %#v", rows[0])
	}
	paused := true
	disabled := false
	if err := SetConnectedBotPeerState(ownerID, 0, 7001, &paused, nil); err != nil {
		t.Fatalf("set paused: %v", err)
	}
	if err := SetConnectedBotPeerState(ownerID, 0, 7001, nil, &disabled); err != nil {
		t.Fatalf("set disabled: %v", err)
	}
	var gotPaused, gotDisabled bool
	if err := db.QueryRow(`SELECT paused, disabled FROM apifull_connected_bot_peer WHERE owner_user_id=$1 AND peer_type=0 AND peer_id=7001`, ownerID).Scan(&gotPaused, &gotDisabled); err != nil {
		t.Fatalf("read peer state: %v", err)
	}
	if !gotPaused || gotDisabled {
		t.Fatalf("peer state was not merged atomically: paused=%v disabled=%v", gotPaused, gotDisabled)
	}
	if err := DeleteConnectedBot(ownerID, botID); err != nil {
		t.Fatalf("delete connected bot: %v", err)
	}
}
