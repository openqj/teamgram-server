package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStarsSubscriptionsUseVerifiedProvider(t *testing.T) {
	const uid int64 = 903101
	const target int64 = 903102
	var operations []string
	var idempotencyKeys []string
	balance := int64(27)
	subscription := mtproto.MakeTLStarsSubscription(&mtproto.StarsSubscription{
		Id:        "sub-1",
		Peer:      mtproto.MakePeerUser(uid),
		UntilDate: 1720000000,
		Pricing: mtproto.MakeTLStarsSubscriptionPricing(&mtproto.StarsSubscriptionPricing{
			Period: 30 * 24 * 60 * 60,
			Amount: 100,
		}).To_StarsSubscriptionPricing(),
	}).To_StarsSubscription()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Operation      string `json:"operation"`
			CallerUserID   int64  `json:"caller_user_id"`
			UserID         int64  `json:"user_id"`
			SubscriptionID string `json:"subscription_id"`
			ChargeID       string `json:"charge_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
			return
		}
		operations = append(operations, request.Operation)
		idempotencyKeys = append(idempotencyKeys, r.Header.Get("Idempotency-Key"))
		switch request.Operation {
		case "get_stars_subscriptions":
			writeSignedPaymentProviderResponse(t, w, starsSubscriptionsProviderResponse{
				Verified: true, UserID: uid, Balance: &balance,
				Subscriptions: []*mtproto.StarsSubscription{subscription},
				NextOffset:    wrapperspb.String("next-1"),
			})
		case "change_stars_subscription", "fulfill_stars_subscription":
			writeSignedPaymentProviderResponse(t, w, starsSubscriptionMutationProviderResponse{
				Verified: true, CallerUserID: uid, UserID: uid,
				SubscriptionID: request.SubscriptionID, Result: mtproto.BoolTrue,
			})
		case "bot_cancel_stars_subscription":
			writeSignedPaymentProviderResponse(t, w, starsSubscriptionMutationProviderResponse{
				Verified: true, CallerUserID: uid, UserID: target,
				SubscriptionID: request.ChargeID, Result: mtproto.BoolTrue,
			})
		default:
			http.Error(w, "unexpected operation", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{
			PaymentProviderEndpoint:   server.URL,
			PaymentProviderSigningKey: paymentProviderTestSigningKey,
		}},
		MD: &metadata.RpcMetadata{UserId: uid},
	}
	selfPeer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	status, err := c.PaymentsGetStarsSubscriptions(&mtproto.TLPaymentsGetStarsSubscriptions{Peer: selfPeer, Offset: "0"})
	if err != nil || status == nil || status.GetBalance_INT64() != balance || len(status.GetSubscriptions()) != 1 || status.GetSubscriptionsNextOffset().GetValue() != "next-1" {
		t.Fatalf("subscriptions = (%#v, %v)", status, err)
	}
	if err = status.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("subscriptions did not encode: %v", err)
	}
	changed, err := c.PaymentsChangeStarsSubscription(&mtproto.TLPaymentsChangeStarsSubscription{
		SubscriptionId: "sub-1", Peer: selfPeer, Canceled: mtproto.BoolTrue,
	})
	if err != nil || !mtproto.FromBool(changed) {
		t.Fatalf("change subscription = (%#v, %v)", changed, err)
	}
	fulfilled, err := c.PaymentsFulfillStarsSubscription(&mtproto.TLPaymentsFulfillStarsSubscription{SubscriptionId: "sub-1", Peer: selfPeer})
	if err != nil || !mtproto.FromBool(fulfilled) {
		t.Fatalf("fulfill subscription = (%#v, %v)", fulfilled, err)
	}
	canceled, err := c.PaymentsBotCancelStarsSubscription(&mtproto.TLPaymentsBotCancelStarsSubscription{
		UserId: &mtproto.InputUser{UserId: target}, ChargeId_STRING: "charge-1",
	})
	if err != nil || !mtproto.FromBool(canceled) {
		t.Fatalf("bot cancel subscription = (%#v, %v)", canceled, err)
	}
	want := []string{"get_stars_subscriptions", "change_stars_subscription", "fulfill_stars_subscription", "bot_cancel_stars_subscription"}
	if fmt.Sprint(operations) != fmt.Sprint(want) {
		t.Fatalf("provider operations = %v, want %v", operations, want)
	}
	if idempotencyKeys[0] != "" || idempotencyKeys[1] == "" || idempotencyKeys[2] == "" || idempotencyKeys[3] == "" || idempotencyKeys[1] == idempotencyKeys[2] || idempotencyKeys[2] == idempotencyKeys[3] {
		t.Fatalf("provider idempotency keys = %v, want unique keys for mutations only", idempotencyKeys)
	}
}

func TestStarsSubscriptionsRejectMalformedProviderResult(t *testing.T) {
	const uid int64 = 903103
	selfPeer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		balance := int64(1)
		writeSignedPaymentProviderResponse(t, w, starsSubscriptionsProviderResponse{
			Verified: true, UserID: uid, Balance: &balance,
			Subscriptions: []*mtproto.StarsSubscription{{Id: "duplicate"}, {Id: "duplicate"}},
		})
	}))
	defer server.Close()
	c := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetStarsSubscriptions(&mtproto.TLPaymentsGetStarsSubscriptions{Peer: selfPeer})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("malformed subscriptions = (%#v, %v), want provider-invalid refusal", got, err)
	}
}
