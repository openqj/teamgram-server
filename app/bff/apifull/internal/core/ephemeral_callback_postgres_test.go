package core

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestEphemeralGetCallbackAnswerUsesCallerScopedPostgresControlPlane(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	previous := persist.Default
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL control-plane schema unavailable: %v", err)
	}
	db, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	const userID int64 = 9827001
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	scopeKey := fmt.Sprintf("%s:msg:%d:%s", businessControlPeerKey(userID, peer), 17, businessControlHash("data", []byte("approve")))
	operation := "ephemeral.getCallbackAnswer"
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_business_control_plane WHERE operation=$1 AND scope_key=$2 AND caller_user_id=$3`, operation, scopeKey, userID)
		_ = db.Close()
		_ = persist.ClosePostgres()
		persist.Default = previous
	})
	if _, err = db.Exec(`DELETE FROM apifull_business_control_plane WHERE operation=$1 AND scope_key=$2 AND caller_user_id=$3`, operation, scopeKey, userID); err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"predicate_name":"messages_botCallbackAnswer","cache_time":15,"message":{"value":"approved"}}`)
	if err = persist.PutBusinessControlRecord(persist.BusinessControlRecord{
		Operation: operation, ScopeKey: scopeKey, CallerUserID: userID, Result: result,
	}); err != nil {
		t.Fatal(err)
	}

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	got, err := core.EphemeralGetCallbackAnswer(&mtproto.TLEphemeralGetCallbackAnswer{
		Peer: peer, Id: 17, Data: []byte("approve"),
	})
	if err != nil || got == nil || got.GetCacheTime() != 15 || got.GetMessage() == nil || got.GetMessage().GetValue() != "approved" {
		t.Fatalf("callback answer = (%v, %v)", got, err)
	}

	other := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID + 1}}
	if got, err = other.EphemeralGetCallbackAnswer(&mtproto.TLEphemeralGetCallbackAnswer{
		Peer: peer, Id: 17, Data: []byte("approve"),
	}); got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("cross-caller callback answer = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}
}
