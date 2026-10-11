package core

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestStarsOptionsReadBackFromPostgresCatalog(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.Close() })
	productPrefix := fmt.Sprintf("pg-options-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if err := domain.DeleteStarsOffersByProductPrefix(productPrefix + "-%"); err != nil {
			t.Errorf("clean stars options: %v", err)
		}
	})
	if err := domain.UpsertStarsOffer("topup", 25, productPrefix+"-topup", "USD", 250, false); err != nil {
		t.Fatalf("upsert topup offer: %v", err)
	}
	if err := domain.UpsertStarsOffer("gift", 50, productPrefix+"-gift", "USD", 500, true); err != nil {
		t.Fatalf("upsert gift offer: %v", err)
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 99112233}}
	topup, err := c.PaymentsGetStarsTopupOptions(&mtproto.TLPaymentsGetStarsTopupOptions{})
	if err != nil || topup == nil {
		t.Fatalf("topup options = (%#v, %v)", topup, err)
	}
	var foundTopup bool
	for _, option := range topup.GetDatas() {
		if option != nil && option.GetStoreProduct().GetValue() == productPrefix+"-topup" {
			foundTopup = option.GetStars() == 25 && option.GetCurrency() == "USD" && option.GetAmount() == 250
		}
	}
	if !foundTopup {
		t.Fatalf("topup catalog did not return persisted option: %#v", topup.GetDatas())
	}

	self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	gift, err := c.PaymentsGetStarsGiftOptions(&mtproto.TLPaymentsGetStarsGiftOptions{UserId: self})
	if err != nil || gift == nil {
		t.Fatalf("gift options = (%#v, %v)", gift, err)
	}
	var foundGift bool
	for _, option := range gift.GetDatas() {
		if option != nil && option.GetStoreProduct().GetValue() == productPrefix+"-gift" {
			foundGift = option.GetStars() == 50 && option.GetCurrency() == "USD" && option.GetAmount() == 500 && option.GetExtended()
		}
	}
	if !foundGift {
		t.Fatalf("gift catalog did not return persisted option: %#v", gift.GetDatas())
	}
	other := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 99112234}).To_InputUser()
	if got, err := c.PaymentsGetStarsGiftOptions(&mtproto.TLPaymentsGetStarsGiftOptions{UserId: other}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("gift target without access hash = (%v, %v), want USER_ID_INVALID", got, err)
	}

	if _, err := c.PaymentsGetStarsTopupOptions(nil); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("nil topup request = %v, want METHOD_NOT_IMPL", err)
	}
}
