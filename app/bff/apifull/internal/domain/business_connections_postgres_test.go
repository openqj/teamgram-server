package domain

import (
	"context"
	"os"
	"testing"
)

func TestBusinessConnectionRegistryAndOutboxRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" || !Ready() {
		t.Skip("APIFULL_POSTGRES_DSN and an open PostgreSQL domain are required")
	}
	ownerID := int64(983000000 + os.Getpid()%100000)
	botID := ownerID + 1
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_business_connection_outbox WHERE bot_user_id=$1`, botID)
		_, _ = db.Exec(`DELETE FROM apifull_business_connection WHERE bot_user_id=$1`, botID)
		_, _ = db.Exec(`DELETE FROM apifull_connected_bot WHERE owner_user_id=$1 AND bot_user_id=$2`, ownerID, botID)
	})
	record, err := UpsertConnectedBotAndConnection(ownerID, botID, true,
		[]byte(`{"reply":true}`), []byte(`{"existing_chats":true}`), nil, 2, false)
	if err != nil {
		t.Fatalf("create business connection: %v", err)
	}
	if len(record.ConnectionID) != 64 || record.OwnerUserID != ownerID || record.BotUserID != botID || record.QTS <= 0 || record.Disabled {
		t.Fatalf("unexpected connection record: %+v", record)
	}
	loaded, found, err := LoadBusinessConnection(botID, record.ConnectionID)
	if err != nil || !found || loaded.ConnectionID != record.ConnectionID || loaded.OwnerUserID != ownerID || loaded.DCID != 2 {
		t.Fatalf("load business connection: found=%v err=%v record=%+v", found, err, loaded)
	}
	deliveries, err := ListDueBusinessConnectionDeliveries(context.Background(), 10)
	if err != nil || len(deliveries) != 1 || deliveries[0].Connection.QTS != record.QTS {
		t.Fatalf("list due business connection: deliveries=%+v err=%v", deliveries, err)
	}
	if err := CompleteBusinessConnectionDelivery(context.Background(), deliveries[0].ID); err != nil {
		t.Fatalf("complete delivery: %v", err)
	}
	if due, err := ListDueBusinessConnectionDeliveries(context.Background(), 10); err != nil || len(due) != 0 {
		t.Fatalf("completed delivery still due: due=%+v err=%v", due, err)
	}
	updated, err := UpsertConnectedBotAndConnection(ownerID, botID, false,
		[]byte(`{"reply":false}`), []byte(`{"new_chats":true}`), nil, 2, true)
	if err != nil || updated.ConnectionID != record.ConnectionID || updated.QTS <= record.QTS || !updated.Disabled {
		t.Fatalf("disable business connection: record=%+v err=%v", updated, err)
	}
	deliveries, err = ListDueBusinessConnectionDeliveries(context.Background(), 10)
	if err != nil || len(deliveries) != 1 || !deliveries[0].Connection.Disabled {
		t.Fatalf("disabled update delivery: deliveries=%+v err=%v", deliveries, err)
	}
}
