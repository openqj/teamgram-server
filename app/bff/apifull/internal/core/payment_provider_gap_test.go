package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/grpc/status"
)

func paymentRPCErrorCode(got, want error) bool {
	return got != nil && want != nil && status.Code(got) == status.Code(want) &&
		status.Convert(got).Message() == status.Convert(want).Message()
}

// Payment methods that would mutate provider-owned inventory or recurring
// entitlements must not manufacture a successful TL result while those
// authoritative providers are absent.
func TestPaymentProviderGapsFailClosed(t *testing.T) {
	const userID int64 = 17001
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	selfPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID}).To_InputPeer()

	tests := []struct {
		name string
		call func() (any, error)
		want error
	}{
		{
			name: "launch prepaid giveaway",
			call: func() (any, error) {
				return core.PaymentsLaunchPrepaidGiveaway(nil)
			},
			want: mtproto.ErrMethodNotImpl,
		},
		{
			name: "get Stars subscriptions",
			call: func() (any, error) {
				return core.PaymentsGetStarsSubscriptions(&mtproto.TLPaymentsGetStarsSubscriptions{
					Peer: selfPeer, Offset: "0",
				})
			},
			want: mtproto.ErrPaymentUnsupported,
		},
		{
			name: "change Stars subscription",
			call: func() (any, error) {
				return core.PaymentsChangeStarsSubscription(&mtproto.TLPaymentsChangeStarsSubscription{
					SubscriptionId: "sub-1", Peer: selfPeer, Canceled: mtproto.BoolTrue,
				})
			},
			want: mtproto.ErrPaymentUnsupported,
		},
		{
			name: "fulfill Stars subscription",
			call: func() (any, error) {
				return core.PaymentsFulfillStarsSubscription(&mtproto.TLPaymentsFulfillStarsSubscription{
					SubscriptionId: "sub-1", Peer: selfPeer,
				})
			},
			want: mtproto.ErrPaymentUnsupported,
		},
		{
			name: "cancel Stars subscription",
			call: func() (any, error) {
				return core.PaymentsBotCancelStarsSubscription(&mtproto.TLPaymentsBotCancelStarsSubscription{
					UserId: &mtproto.InputUser{UserId: userID}, ChargeId_STRING: "charge-1",
				})
			},
			want: mtproto.ErrPaymentUnsupported,
		},
		{
			name: "gift resale",
			call: func() (any, error) {
				return core.PaymentsGetResaleStarGifts(nil)
			},
		},
		{
			name: "gift upgrade preview",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftUpgradePreview(nil)
			},
		},
		{
			name: "gift upgrade",
			call: func() (any, error) {
				return core.PaymentsUpgradeStarGift(nil)
			},
		},
		{
			name: "gift price update",
			call: func() (any, error) {
				return core.PaymentsUpdateStarGiftPrice(nil)
			},
		},
		{
			name: "gift value info",
			call: func() (any, error) {
				return core.PaymentsGetUniqueStarGiftValueInfo(nil)
			},
		},
		{
			name: "gift send check",
			call: func() (any, error) {
				return core.PaymentsCheckCanSendGift(nil)
			},
		},
		{
			name: "gift auction state",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftAuctionState(nil)
			},
		},
		{
			name: "gift auction acquired",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftAuctionAcquiredGifts(nil)
			},
		},
		{
			name: "gift active auctions",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftActiveAuctions(nil)
			},
		},
		{
			name: "gift offer resolve",
			call: func() (any, error) {
				return core.PaymentsResolveStarGiftOffer(nil)
			},
		},
		{
			name: "gift offer send",
			call: func() (any, error) {
				return core.PaymentsSendStarGiftOffer(nil)
			},
		},
		{
			name: "gift upgrade attributes",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftUpgradeAttributes(nil)
			},
		},
		{
			name: "gift craft list",
			call: func() (any, error) {
				return core.PaymentsGetCraftStarGifts(nil)
			},
		},
		{
			name: "gift craft",
			call: func() (any, error) {
				return core.PaymentsCraftStarGift(nil)
			},
		},
		{
			name: "gift withdrawal",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftWithdrawalUrl(nil)
			},
		},
		{
			name: "affiliate list",
			call: func() (any, error) {
				return core.PaymentsGetConnectedStarRefBots(nil)
			},
		},
		{
			name: "affiliate get",
			call: func() (any, error) {
				return core.PaymentsGetConnectedStarRefBot(nil)
			},
		},
		{
			name: "affiliate suggested",
			call: func() (any, error) {
				return core.PaymentsGetSuggestedStarRefBots(nil)
			},
		},
		{
			name: "affiliate connect",
			call: func() (any, error) {
				return core.PaymentsConnectStarRefBot(nil)
			},
		},
		{
			name: "affiliate edit",
			call: func() (any, error) {
				return core.PaymentsEditConnectedStarRefBot(nil)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.call()
			want := test.want
			if want == nil {
				want = mtproto.ErrMethodNotImpl
			}
			if !nilRPCResult(result) || !paymentRPCErrorCode(err, want) {
				t.Fatalf("result = %#v, err = %v; want nil and %v", result, err, want)
			}
		})
	}
}
