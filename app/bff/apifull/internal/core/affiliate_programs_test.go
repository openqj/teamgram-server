package core

import (
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
			result, err := c.PaymentsLaunchPrepaidGiveaway(&mtproto.TLPaymentsLaunchPrepaidGiveaway{
				Peer:       mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42}).To_InputPeer(),
				GiveawayId: 77,
				Purpose:    mtproto.MakeTLInputStorePaymentPremiumGiveaway(nil).To_InputStorePaymentPurpose(),
			})
			return result != nil, err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.invoke()
			if result || !paymentRPCErrorCode(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result present=%v err=%v, want nil result and METHOD_NOT_IMPL", result, err)
			}
		})
	}
}

func TestAffiliateProviderRejectsWrongConstructors(t *testing.T) {
	core := newBusinessProviderGapCore(t, "bots.updateStarRefProgram", map[string]any{
		"predicate_name": "payments_connectedStarRefBots", "bot_id": 7, "commission_permille": 125,
	})
	result, err := core.BotsUpdateStarRefProgram(&mtproto.TLBotsUpdateStarRefProgram{
		Bot: &mtproto.InputUser{UserId: 7, AccessHash: 9}, CommissionPermille: 125,
	})
	if result != nil || !paymentRPCErrorCode(err, mtproto.ErrInternalServerError) {
		t.Fatalf("wrong affiliate constructor = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}

	core = newBusinessProviderGapCore(t, "payments.getSuggestedStarRefBots", map[string]any{
		"predicate_name": "payments_connectedStarRefBots", "count": 0,
	})
	suggested, err := core.PaymentsGetSuggestedStarRefBots(&mtproto.TLPaymentsGetSuggestedStarRefBots{Limit: 10})
	if suggested != nil || !paymentRPCErrorCode(err, mtproto.ErrInternalServerError) {
		t.Fatalf("wrong suggested constructor = (%v, %v), want INTERNAL_SERVER_ERROR", suggested, err)
	}
}
