package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestStarsSubscriptionsFailClosedWithoutBillingLifecycle(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81016}}
	calls := []struct {
		name   string
		invoke func() (bool, error)
	}{
		{"list", func() (bool, error) {
			result, err := c.PaymentsGetStarsSubscriptions(nil)
			return result != nil, err
		}},
		{"change", func() (bool, error) {
			result, err := c.PaymentsChangeStarsSubscription(&mtproto.TLPaymentsChangeStarsSubscription{SubscriptionId: "sub"})
			return result != nil, err
		}},
		{"fulfill", func() (bool, error) {
			result, err := c.PaymentsFulfillStarsSubscription(&mtproto.TLPaymentsFulfillStarsSubscription{SubscriptionId: "sub"})
			return result != nil, err
		}},
		{"cancel", func() (bool, error) {
			result, err := c.PaymentsBotCancelStarsSubscription(nil)
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
