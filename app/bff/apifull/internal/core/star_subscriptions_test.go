package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestStarsSubscriptionsFailClosedWithoutBillingProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81016}}
	selfPeer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	calls := []struct {
		name   string
		invoke func() (bool, error)
		want   error
	}{
		{"list", func() (bool, error) {
			result, err := c.PaymentsGetStarsSubscriptions(nil)
			return result != nil, err
		}, mtproto.ErrInputConstructorInvalid},
		{"change", func() (bool, error) {
			result, err := c.PaymentsChangeStarsSubscription(&mtproto.TLPaymentsChangeStarsSubscription{SubscriptionId: "sub", Peer: selfPeer, Canceled: mtproto.BoolTrue})
			return result != nil, err
		}, mtproto.ErrPaymentUnsupported},
		{"fulfill", func() (bool, error) {
			result, err := c.PaymentsFulfillStarsSubscription(&mtproto.TLPaymentsFulfillStarsSubscription{SubscriptionId: "sub", Peer: selfPeer})
			return result != nil, err
		}, mtproto.ErrPaymentUnsupported},
		{"cancel", func() (bool, error) {
			result, err := c.PaymentsBotCancelStarsSubscription(nil)
			return result != nil, err
		}, mtproto.ErrInputConstructorInvalid},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.invoke()
			if result || !paymentRPCErrorCode(err, tc.want) {
				t.Fatalf("result present=%v err=%v, want nil result and %v", result, err, tc.want)
			}
		})
	}
}
