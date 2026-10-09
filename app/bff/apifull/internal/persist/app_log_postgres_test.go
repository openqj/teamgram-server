package persist

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestSaveAppLogEventsPostgresTransaction(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ClosePostgres() })

	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		t.Fatal("PostgreSQL store was not opened")
	}
	ctx := context.Background()
	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(ctx, `DELETE FROM apifull_app_log WHERE user_id = $1`, userID)
	})

	events := []*mtproto.InputAppEvent{
		{Type: "app_open", Time: 1.25, Peer: 42, Data: &mtproto.JSONValue{Value_STRING: "ok"}},
		nil,
		{Type: "", Time: 2},
	}
	if err := SaveAppLogEvents(ctx, userID, events); err != nil {
		t.Fatal(err)
	}
	var count int
	var data string
	if err := store.db.QueryRowContext(ctx, `SELECT count(*), max(data::text) FROM apifull_app_log WHERE user_id = $1`, userID).Scan(&count, &data); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !strings.Contains(data, "ok") {
		t.Fatalf("persisted app log count=%d data=%q, want one JSON payload", count, data)
	}

	stamp := time.Now().UnixNano()
	functionName := fmt.Sprintf("teamgram_test_app_log_fail_%d", stamp)
	triggerName := fmt.Sprintf("teamgram_test_app_log_fail_trigger_%d", stamp)
	if _, err := store.db.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.event_type = 'rollback' THEN
    RAISE EXCEPTION 'intentional app-log rollback';
  END IF;
  RETURN NEW;
END
$$;
CREATE TRIGGER %s BEFORE INSERT ON apifull_app_log FOR EACH ROW EXECUTE FUNCTION %s();`, functionName, triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON apifull_app_log", triggerName))
		_, _ = store.db.ExecContext(context.Background(), fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName))
	})

	rollbackUser := userID + 1
	err := SaveAppLogEvents(ctx, rollbackUser, []*mtproto.InputAppEvent{
		{Type: "accepted", Time: 3},
		{Type: "rollback", Time: 4},
	})
	if err == nil {
		t.Fatal("SaveAppLogEvents succeeded despite trigger failure")
	}
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM apifull_app_log WHERE user_id = $1`, rollbackUser).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback left %d rows", count)
	}
}
