package core

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestPromoDataHidePostgresRoundTripAndConcurrentUpdates(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	// TestMain owns the process-wide PostgreSQL store; keep it open for the
	// remainder of the package suite.

	uid := time.Now().UnixNano()
	key := promoHideKey(uid)
	store, ok := persist.Default.(interface {
		Get(string) (string, error)
		Set(string, string) error
	})
	if !ok {
		t.Fatal("PostgreSQL store was not opened")
	}
	_ = store.Set(key, "")
	t.Cleanup(func() { _ = store.Set(key, "") })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	first, err := c.HelpGetPromoData(&mtproto.TLHelpGetPromoData{})
	if err != nil {
		t.Fatalf("initial promo read: %v", err)
	}
	if first.GetPredicateName() != mtproto.Predicate_help_promoData || first.GetPeer() == nil || first.GetPeer().GetUserId() != builtinPromos[0].userID {
		t.Fatalf("initial promo = %+v, want first builtin promo", first)
	}

	// Two independent hides must be serialized so neither update is lost.
	peerA := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: builtinPromos[0].userID}).To_InputPeer()
	peerB := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: builtinPromos[1].userID}).To_InputPeer()
	errCh := make(chan error, 2)
	go func() { _, err := c.HelpHidePromoData(&mtproto.TLHelpHidePromoData{Peer: peerA}); errCh <- err }()
	go func() { _, err := c.HelpHidePromoData(&mtproto.TLHelpHidePromoData{Peer: peerB}); errCh <- err }()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("hide promo: %v", err)
		}
	}

	last, err := c.HelpGetPromoData(&mtproto.TLHelpGetPromoData{})
	if err != nil {
		t.Fatalf("final promo read: %v", err)
	}
	if last.GetPredicateName() != mtproto.Predicate_help_promoDataEmpty {
		t.Fatalf("final promo = %+v, want help.promoDataEmpty", last)
	}
	raw, err := store.Get(key)
	if err != nil {
		t.Fatalf("read hidden promos: %v", err)
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		t.Fatalf("decode hidden promos %q: %v", raw, err)
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range []int64{builtinPromos[0].userID, builtinPromos[1].userID} {
		if !seen[id] {
			t.Fatalf("hidden promos %v, missing %d", ids, id)
		}
	}

	// Repeating a hide is idempotent and preserves the existing JSON state.
	if got, err := c.HelpHidePromoData(&mtproto.TLHelpHidePromoData{Peer: peerA}); err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_boolTrue {
		t.Fatalf("retry hide = (%v, %v)", got, err)
	}
}
