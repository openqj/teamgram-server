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
	selfPeer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()

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
			want: mtproto.ErrPeerIdInvalid,
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
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift upgrade preview",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftUpgradePreview(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift upgrade",
			call: func() (any, error) {
				return core.PaymentsUpgradeStarGift(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift price update",
			call: func() (any, error) {
				return core.PaymentsUpdateStarGiftPrice(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift value info",
			call: func() (any, error) {
				return core.PaymentsGetUniqueStarGiftValueInfo(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift send check",
			call: func() (any, error) {
				return core.PaymentsCheckCanSendGift(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift auction state",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftAuctionState(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift auction acquired",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftAuctionAcquiredGifts(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift active auctions",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftActiveAuctions(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift offer resolve",
			call: func() (any, error) {
				return core.PaymentsResolveStarGiftOffer(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift offer send",
			call: func() (any, error) {
				return core.PaymentsSendStarGiftOffer(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift upgrade attributes",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftUpgradeAttributes(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift craft list",
			call: func() (any, error) {
				return core.PaymentsGetCraftStarGifts(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift craft",
			call: func() (any, error) {
				return core.PaymentsCraftStarGift(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "gift withdrawal",
			call: func() (any, error) {
				return core.PaymentsGetStarGiftWithdrawalUrl(nil)
			},
			want: mtproto.ErrInputConstructorInvalid,
		},
		{
			name: "affiliate list",
			call: func() (any, error) {
				return core.PaymentsGetConnectedStarRefBots(nil)
			},
			want: mtproto.ErrLimitInvalid,
		},
		{
			name: "affiliate get",
			call: func() (any, error) {
				return core.PaymentsGetConnectedStarRefBot(nil)
			},
			want: mtproto.ErrInputRequestInvalid,
		},
		{
			name: "affiliate suggested",
			call: func() (any, error) {
				return core.PaymentsGetSuggestedStarRefBots(nil)
			},
			want: mtproto.ErrLimitInvalid,
		},
		{
			name: "affiliate connect",
			call: func() (any, error) {
				return core.PaymentsConnectStarRefBot(nil)
			},
			want: mtproto.ErrInputRequestInvalid,
		},
		{
			name: "affiliate edit",
			call: func() (any, error) {
				return core.PaymentsEditConnectedStarRefBot(nil)
			},
			want: mtproto.ErrInputRequestInvalid,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.call()
			want := test.want
			if !nilRPCResult(result) || !paymentRPCErrorCode(err, want) {
				t.Fatalf("result = %#v, err = %v; want nil and %v", result, err, want)
			}
		})
	}
}
