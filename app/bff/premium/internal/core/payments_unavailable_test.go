package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

func TestStorePurchaseChecksFailClosedWithoutProvider(t *testing.T) {
	const userID int64 = 81019
	if err := persist.Default.Set("premium:81019", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.Default.Set("premium:81019", "") })

	c := &PremiumCore{MD: &metadata.RpcMetadata{UserId: userID}}
	store, err := c.PaymentsCanPurchaseStore(nil)
	if store != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("canPurchaseStore = (%#v, %v), want (nil, METHOD_NOT_IMPL)", store, err)
	}
	premium, err := c.PaymentsCanPurchasePremium(nil)
	if premium != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("canPurchasePremium = (%#v, %v), want (nil, METHOD_NOT_IMPL)", premium, err)
	}
}
