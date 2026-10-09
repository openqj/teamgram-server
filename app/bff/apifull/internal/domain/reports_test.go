package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"
)

func requireReportsDB(t *testing.T) string {
	t.Helper()
	requirePaymentLedgerDB(t)
	return os.Getenv("APIFULL_POSTGRES_DSN")
}

func TestReportIntakeIsDurableAndIdempotent(t *testing.T) {
	dsn := requireReportsDB(t)
	actorID := time.Now().UnixNano()
	kind := "test.report"
	targetType := "user"
	targetID := actorID + 1
	payload := []byte(`{"peer":{"user_id":1},"reason":"spam"}`)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s:%d:%s", actorID, kind, targetType, targetID, payload)))
	dedupeKey := hex.EncodeToString(sum[:])
	t.Cleanup(func() {
		if db != nil {
			_, _ = db.Exec(`DELETE FROM apifull_report WHERE actor_user_id=?`, actorID)
		}
	})

	if err := SaveReport(actorID, kind, targetType, targetID, dedupeKey, payload); err != nil {
		t.Fatalf("save report: %v", err)
	}
	if err := SaveReport(actorID, kind, targetType, targetID, dedupeKey, payload); err != nil {
		t.Fatalf("replay report: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_report WHERE actor_user_id=? AND dedupe_key=?`, actorID, dedupeKey).Scan(&count); err != nil {
		t.Fatalf("count report rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("report rows = %d, want one idempotent row", count)
	}

	// Reopen the same isolated database to exercise the read-after-restart path.
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("reopen isolated report database: %v", err)
	}
	loaded, found, err := LoadReportByDedupe(dedupeKey)
	if err != nil || !found {
		t.Fatalf("load report = %+v found=%v err=%v", loaded, found, err)
	}
	if loaded.ActorID != actorID || loaded.Kind != kind || loaded.TargetType != targetType || loaded.TargetID != targetID || loaded.State != "pending" || string(loaded.Payload) != string(payload) {
		t.Fatalf("loaded report = %+v, want durable pending payload", loaded)
	}
}
