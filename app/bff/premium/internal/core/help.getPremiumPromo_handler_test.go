package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestHelpGetPremiumPromoFailsClosedWithoutOfferProvider(t *testing.T) {
	c := &PremiumCore{MD: &metadata.RpcMetadata{UserId: 42}}
	got, err := c.HelpGetPremiumPromo(&mtproto.TLHelpGetPremiumPromo{})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("HelpGetPremiumPromo() = (%v, %v), want nil and METHOD_NOT_IMPL", got, err)
	}
}

func TestHelpGetPremiumPromoAuthenticatesBeforeValidation(t *testing.T) {
	c := &PremiumCore{}
	got, err := c.HelpGetPremiumPromo(nil)
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("HelpGetPremiumPromo() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
