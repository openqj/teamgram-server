package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/svc"
)

const premiumProviderTestKey = "premium-provider-test-signing-key-0123456789"

func TestPaymentsCanPurchaseStoreUsesVerifiedProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		result, err := json.Marshal(mtproto.BoolTrue)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(map[string]any{"request_id": request.RequestID, "operation": request.Operation, "verified": true, "user_id": request.UserID, "result": json.RawMessage(result)})
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, []byte(premiumProviderTestKey))
		_, _ = mac.Write(body)
		w.Header().Set("X-Teamgram-Payment-Provider-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c := New(context.Background(), &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: premiumProviderTestKey}})
	c.MD = &metadata.RpcMetadata{UserId: 42}
	got, err := c.PaymentsCanPurchaseStore(&mtproto.TLPaymentsCanPurchaseStore{Purpose: mtproto.MakeTLInputStorePaymentPremiumSubscription(nil).To_InputStorePaymentPurpose()})
	if err != nil || got == nil || got.GetPredicateName() == "" {
		t.Fatalf("PaymentsCanPurchaseStore = (%v, %v)", got, err)
	}
}

func newPremiumProviderTestCore(t *testing.T, result any, signResponse bool) *PremiumCore {
	t.Helper()
	const userID int64 = 42
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
			return
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			t.Errorf("marshal provider result: %v", err)
			return
		}
		body, err := json.Marshal(map[string]any{
			"request_id": request.RequestID,
			"operation":  request.Operation,
			"verified":   true,
			"user_id":    request.UserID,
			"result":     json.RawMessage(resultJSON),
		})
		if err != nil {
			t.Errorf("marshal provider response: %v", err)
			return
		}
		if signResponse {
			mac := hmac.New(sha256.New, []byte(premiumProviderTestKey))
			_, _ = mac.Write(body)
			w.Header().Set("X-Teamgram-Payment-Provider-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return New(context.Background(), &svc.ServiceContext{Config: config.Config{
		PaymentProviderEndpoint:       server.URL,
		PaymentProviderSigningKey:     premiumProviderTestKey,
		PaymentProviderTimeoutSeconds: 2,
	}})
}

func TestPremiumProviderAssignmentAndPromoResults(t *testing.T) {
	core := newPremiumProviderTestCore(t, mtproto.MakeEmptyUpdates(), true)
	core.MD = &metadata.RpcMetadata{UserId: 42}
	purpose := mtproto.MakeTLInputStorePaymentPremiumSubscription(&mtproto.InputStorePaymentPurpose{Currency: "USD", Amount: 499}).To_InputStorePaymentPurpose()
	updates, err := core.PaymentsAssignAppStoreTransaction(&mtproto.TLPaymentsAssignAppStoreTransaction{Receipt: []byte("receipt"), Purpose: purpose})
	if err != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("app store assignment = (%v, %v)", updates, err)
	}

	promo := &mtproto.Help_PremiumPromo{Currency: "USD", MonthlyAmount: 499, PeriodOptions: []*mtproto.PremiumSubscriptionOption{{Months: 1, Currency: "USD", Amount: 499}}}
	promoCore := newPremiumProviderTestCore(t, map[string]any{"promo": promo}, true)
	promoCore.MD = &metadata.RpcMetadata{UserId: 42}
	got, err := promoCore.HelpGetPremiumPromo(&mtproto.TLHelpGetPremiumPromo{})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_help_premiumPromo || got.GetPeriodOptions()[0].GetPredicateName() != mtproto.Predicate_premiumSubscriptionOption {
		t.Fatalf("premium promo = (%v, %v)", got, err)
	}
}

func TestPremiumProviderRejectsUnsignedAssignment(t *testing.T) {
	core := newPremiumProviderTestCore(t, mtproto.MakeEmptyUpdates(), false)
	core.MD = &metadata.RpcMetadata{UserId: 42}
	purpose := mtproto.MakeTLInputStorePaymentPremiumSubscription(&mtproto.InputStorePaymentPurpose{}).To_InputStorePaymentPurpose()
	updates, err := core.PaymentsAssignPlayMarketTransaction(&mtproto.TLPaymentsAssignPlayMarketTransaction{Receipt: &mtproto.DataJSON{Data: "{}"}, Purpose: purpose})
	if updates != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("unsigned assignment = (%v, %v)", updates, err)
	}
}
