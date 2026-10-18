package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestGiveawayOptionsUseVerifiedPaymentProvider(t *testing.T) {
	const uid int64 = 902008
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Operation string `json:"operation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode giveaway request: %v", err)
			return
		}
		switch request.Operation {
		case "get_premium_gift_code_options":
			writeSignedPaymentProviderResponse(t, w, struct {
				Verified bool                             `json:"verified"`
				UserID   int64                            `json:"user_id"`
				Options  []*mtproto.PremiumGiftCodeOption `json:"options"`
			}{true, uid, []*mtproto.PremiumGiftCodeOption{{Users: 1, Months: 3, Currency: "USD", Amount: 999, StoreProduct: wrapperspb.String("premium_3m")}}})
		case "get_stars_giveaway_options":
			writeSignedPaymentProviderResponse(t, w, struct {
				Verified bool                           `json:"verified"`
				UserID   int64                          `json:"user_id"`
				Options  []*mtproto.StarsGiveawayOption `json:"options"`
			}{true, uid, []*mtproto.StarsGiveawayOption{{Stars: 100, YearlyBoosts: 1, Currency: "USD", Amount: 299, Winners: []*mtproto.StarsGiveawayWinnersOption{{Users: 2, PerUserStars: 50}}}}})
		default:
			http.Error(w, "unexpected operation", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	premium, err := c.PaymentsGetPremiumGiftCodeOptions(&mtproto.TLPaymentsGetPremiumGiftCodeOptions{BoostPeer: peer})
	if err != nil || premium == nil || len(premium.GetDatas()) != 1 || premium.GetDatas()[0].GetMonths() != 3 {
		t.Fatalf("premium gift options = (%#v, %v)", premium, err)
	}
	if err = premium.Encode(mtproto.NewEncodeBuf(2048), 229); err != nil {
		t.Fatalf("premium options did not encode: %v", err)
	}
	stars, err := c.PaymentsGetStarsGiveawayOptions(&mtproto.TLPaymentsGetStarsGiveawayOptions{})
	if err != nil || stars == nil || len(stars.GetDatas()) != 1 || stars.GetDatas()[0].GetStars() != 100 {
		t.Fatalf("Stars giveaway options = (%#v, %v)", stars, err)
	}
	if err = stars.Encode(mtproto.NewEncodeBuf(2048), 229); err != nil {
		t.Fatalf("Stars giveaway options did not encode: %v", err)
	}
}

func TestGiveawayOptionsRejectMalformedProviderData(t *testing.T) {
	const uid int64 = 902009
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, struct {
			Verified bool                             `json:"verified"`
			UserID   int64                            `json:"user_id"`
			Options  []*mtproto.PremiumGiftCodeOption `json:"options"`
		}{true, uid, []*mtproto.PremiumGiftCodeOption{{Users: 0, Months: 3, Currency: "USD", Amount: 1}}})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetPremiumGiftCodeOptions(&mtproto.TLPaymentsGetPremiumGiftCodeOptions{})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("malformed premium options = (%#v, %v), want provider-invalid refusal", got, err)
	}
}

func TestPremiumGiftCodeOptionsRejectEmptyStoreProduct(t *testing.T) {
	const uid int64 = 902012
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, struct {
			Verified bool                             `json:"verified"`
			UserID   int64                            `json:"user_id"`
			Options  []*mtproto.PremiumGiftCodeOption `json:"options"`
		}{true, uid, []*mtproto.PremiumGiftCodeOption{{Users: 1, Months: 3, StoreProduct: wrapperspb.String("  "), Currency: "USD", Amount: 999}}})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetPremiumGiftCodeOptions(&mtproto.TLPaymentsGetPremiumGiftCodeOptions{})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("empty store product = (%#v, %v), want provider-invalid refusal", got, err)
	}
}

func TestPremiumGiftCodeOptionsRejectNonPositiveStoreQuantity(t *testing.T) {
	const uid int64 = 902014
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, struct {
			Verified bool                             `json:"verified"`
			UserID   int64                            `json:"user_id"`
			Options  []*mtproto.PremiumGiftCodeOption `json:"options"`
		}{true, uid, []*mtproto.PremiumGiftCodeOption{{Users: 1, Months: 3, StoreQuantity: wrapperspb.Int32(0), Currency: "USD", Amount: 999}}})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetPremiumGiftCodeOptions(&mtproto.TLPaymentsGetPremiumGiftCodeOptions{})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("non-positive store quantity = (%#v, %v), want provider-invalid refusal", got, err)
	}
}

func TestStarsGiveawayOptionsRejectEmptyWinners(t *testing.T) {
	const uid int64 = 902013
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, struct {
			Verified bool                           `json:"verified"`
			UserID   int64                          `json:"user_id"`
			Options  []*mtproto.StarsGiveawayOption `json:"options"`
		}{true, uid, []*mtproto.StarsGiveawayOption{{Stars: 100, Currency: "USD", Amount: 299}}})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetStarsGiveawayOptions(&mtproto.TLPaymentsGetStarsGiveawayOptions{})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("empty winners = (%#v, %v), want provider-invalid refusal", got, err)
	}
}
