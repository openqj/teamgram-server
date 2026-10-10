package core

import (
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestPaymentInfoHandlersUsePostgresStore(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open payment PostgreSQL: %v", err)
	}
	// TestMain owns the process-wide domain handle. Closing it here makes the
	// result depend on test order and leaves later PostgreSQL-backed handlers
	// observing a closed global store.
	cleanup, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatalf("open cleanup PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = cleanup.Close() })
	uid := time.Now().UnixNano()
	t.Cleanup(func() { _, _ = cleanup.Exec(`DELETE FROM apifull_payment_saved_info WHERE user_id=$1`, uid) })
	if err = domain.SaveSavedPaymentInfo(uid, "Alice", "+8613800000000", "alice@example.com"); err != nil {
		t.Fatalf("save info: %v", err)
	}
	if err = domain.SetSavedPaymentCredentials(uid, true); err != nil {
		t.Fatalf("save credentials marker: %v", err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	got, err := c.PaymentsGetSavedInfo(&mtproto.TLPaymentsGetSavedInfo{})
	if err != nil || got == nil || !got.GetHasSavedCredentials() || got.GetSavedInfo() == nil || got.GetSavedInfo().GetEmail().GetValue() != "alice@example.com" {
		t.Fatalf("get saved info = %#v err=%v", got, err)
	}
	if _, err = c.PaymentsClearSavedInfo(&mtproto.TLPaymentsClearSavedInfo{Info: true, Credentials: true}); err != nil {
		t.Fatalf("clear saved info: %v", err)
	}
	if _, found, err := domain.LoadSavedPaymentInfo(uid); err != nil || found {
		t.Fatalf("saved row after clear: found=%v err=%v", found, err)
	}
}
