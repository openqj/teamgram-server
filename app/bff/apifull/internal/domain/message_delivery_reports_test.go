package domain

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRecordMessagesDeliveryPostgres(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close() })
	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM apifull_message_delivery_report WHERE user_id=$1`, userID); err != nil {
			t.Errorf("clean delivery reports: %v", err)
		}
	})

	ctx := context.Background()
	if err := RecordMessagesDelivery(ctx, userID, []int32{11, 12, 11}); err != nil {
		t.Fatal(err)
	}
	if err := RecordMessagesDelivery(ctx, userID, []int32{12}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_message_delivery_report WHERE user_id=$1`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("delivery report count=%d, want 2", count)
	}
	if err := RecordMessagesDelivery(ctx, userID, []int32{0}); err == nil {
		t.Fatal("invalid message id unexpectedly committed")
	}
	var countAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_message_delivery_report WHERE user_id=$1`, userID).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != count {
		t.Fatalf("invalid request changed row count from %d to %d", count, countAfter)
	}
}
