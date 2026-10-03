package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAffiliateAndGiveawayMethodsFailClosedWithoutProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81009}}
	calls := []struct {
		name   string
		invoke func() (bool, error)
	}{
		{"update referral program", func() (bool, error) {
			result, err := c.BotsUpdateStarRefProgram(&mtproto.TLBotsUpdateStarRefProgram{Bot: &mtproto.InputUser{UserId: 42}})
			return result != nil, err
		}},
		{"connect referral bot", func() (bool, error) {
			result, err := c.PaymentsConnectStarRefBot(&mtproto.TLPaymentsConnectStarRefBot{Bot: &mtproto.InputUser{UserId: 42}})
			return result != nil, err
		}},
		{"launch giveaway", func() (bool, error) {
			result, err := c.PaymentsLaunchPrepaidGiveaway(&mtproto.TLPaymentsLaunchPrepaidGiveaway{GiveawayId: 77})
			return result != nil, err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.invoke()
			if result || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result present=%v err=%v, want nil result and METHOD_NOT_IMPL", result, err)
			}
		})
	}
}
