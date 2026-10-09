package core

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestMessagesReportMessagesDeliveryPostgresRoundTrip(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	db, err := persist.OpenPostgresDB(os.Getenv("APIFULL_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	uid := time.Now().UnixNano()
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM apifull_message_delivery_report WHERE user_id=$1`, uid)
		_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k IN ($1, $2)`, b18Key(uid, "delivery"), "rest:messages.reportMessagesDelivery:"+strconv.FormatInt(uid, 10))
	}
	t.Cleanup(cleanup)

	request := &mtproto.TLMessagesReportMessagesDelivery{Id: []int32{101, 102, 101}}
	if got, err := core.MessagesReportMessagesDelivery(request); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("delivery = (%v, %v)", got, err)
	}
	if got, err := core.MessagesReportMessagesDelivery(&mtproto.TLMessagesReportMessagesDelivery{Id: []int32{102}}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("delivery retry = (%v, %v)", got, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_message_delivery_report WHERE user_id=$1`, uid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("delivery rows = %d, want 2", count)
	}

	if got, err := core.MessagesReportMessagesDelivery(&mtproto.TLMessagesReportMessagesDelivery{Id: []int32{0}}); got != nil || err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("invalid delivery = (%v, %v), want MESSAGE_ID_INVALID", got, err)
	}
	var stateCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_kv WHERE k=$1`, b18Key(uid, "delivery")).Scan(&stateCount); err != nil {
		t.Fatal(err)
	}
	if stateCount != 1 {
		t.Fatalf("invalid request changed compatibility state: rows=%d", stateCount)
	}
}
