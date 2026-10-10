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

func TestTogglePeerTranslationsPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}

	uid := time.Now().UnixNano()
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 90201, AccessHash: 90202}).To_InputPeer()
	peerType, peerID := translationPeerTypeID(uid, peer)
	key := "translation:peer:" + strconv.FormatInt(uid, 10) + ":" + strconv.FormatInt(int64(peerType), 10) + ":" + strconv.FormatInt(peerID, 10)
	db, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key) })

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if result, err := core.MessagesTogglePeerTranslations(&mtproto.TLMessagesTogglePeerTranslations{Peer: peer, Disabled: true}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("disable peer translations = (%v, %v)", result, err)
	}
	if got, err := persist.Default.Get(key); err != nil || got != "true" {
		t.Fatalf("stored disabled state = (%q, %v), want true", got, err)
	}
	if result, err := core.MessagesTogglePeerTranslations(&mtproto.TLMessagesTogglePeerTranslations{Peer: peer, Disabled: false}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("enable peer translations = (%v, %v)", result, err)
	}
	if got, err := persist.Default.Get(key); err != nil || got != "false" {
		t.Fatalf("stored enabled state = (%q, %v), want false", got, err)
	}
}
