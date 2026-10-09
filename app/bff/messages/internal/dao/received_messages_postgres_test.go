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
