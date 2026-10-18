package dao

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReceivedMessagesStoreIsMonotonic(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_MESSAGES_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("APIFULL_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("PostgreSQL DSN is not configured")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err = p.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	userID := int64(os.Getpid()) + 920000000
	_, _ = p.Exec(ctx, `DELETE FROM bff_messages_received_message WHERE user_id=$1`, userID)
	t.Cleanup(func() { _, _ = p.Exec(ctx, `DELETE FROM bff_messages_received_message WHERE user_id=$1`, userID) })

	store := NewPostgresReceivedMessagesStore(p)
	for _, maxID := range []int32{100, 50, 200} {
		if err = store.Record(ctx, userID, maxID); err != nil {
			t.Fatalf("Record(%d): %v", maxID, err)
		}
	}
	var got int32
	if err = p.QueryRow(ctx, `SELECT max_id FROM bff_messages_received_message WHERE user_id=$1`, userID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 200 {
		t.Fatalf("max_id=%d, want 200", got)
	}
}

func TestPostgresReceivedNotificationsAcknowledgeAndReplay(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_MESSAGES_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("APIFULL_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("PostgreSQL DSN is not configured")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err = p.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	userID := int64(os.Getpid()) + 921000000
	_, _ = p.Exec(ctx, `DELETE FROM bff_messages_received_notification WHERE user_id=$1`, userID)
	t.Cleanup(func() { _, _ = p.Exec(ctx, `DELETE FROM bff_messages_received_notification WHERE user_id=$1`, userID) })

	store := NewPostgresReceivedMessagesStore(p)
	for _, row := range []struct{ id, flags int32 }{{23, 0}, {7, 4}, {23, 0}} {
		if err = store.RecordNotification(ctx, userID, row.id, row.flags); err != nil {
			t.Fatalf("RecordNotification(%d,%d): %v", row.id, row.flags, err)
		}
	}
	rows, err := store.Acknowledge(ctx, userID, 23)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != 7 || rows[0].Flags != 4 || rows[1].ID != 23 || rows[1].Flags != 0 {
		t.Fatalf("acknowledged rows=%+v, want sorted 7/4 and 23/0", rows)
	}
	rows, err = store.Acknowledge(ctx, userID, 23)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("replayed acknowledgement rows=%+v, want empty", rows)
	}
}
