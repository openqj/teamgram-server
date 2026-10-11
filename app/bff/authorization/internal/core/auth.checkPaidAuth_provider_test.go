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
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestAuthCheckPaidAuthUsesVerifiedProvider(t *testing.T) {
	const phone = "14155552671"
	const hash = "paid-phone-code-hash"
	var request paidAuthProviderRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		response := paidAuthProviderResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			PhoneNumber: phone, PhoneCodeHash: hash, StoreProduct: "telegram-premium-login",
			SupportEmailAddress: "support@example.test", SupportEmailSubject: "Paid login",
			PremiumDays: 30, Currency: "USD", Amount: 199,
		}
		body, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("encode provider response: %v", err)
		}
		w.Header().Set("X-Teamgram-Auth-Provider-Signature", authProviderSignature(authProviderTestSigningKey, body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c := &AuthorizationCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 1},
		svcCtx: &svc.ServiceContext{Config: config.Config{
			AuthProviderEndpoint: server.URL, AuthProviderKey: "provider-key",
			AuthProviderSigningKey: authProviderTestSigningKey, AuthProviderTimeoutSeconds: 5,
		}, Dao: &dao.Dao{}},
	}
	got, err := c.AuthCheckPaidAuth(&mtproto.TLAuthCheckPaidAuth{PhoneNumber: "+1 415 555 2671", PhoneCodeHash: hash, FormId: 77})
	if err != nil || got == nil {
		t.Fatalf("check paid auth = (%#v, %v)", got, err)
	}
	if request.Operation != "auth.checkPaidAuth" || request.PhoneNumber != phone || request.PhoneCodeHash != hash || request.FormID != 77 {
		t.Fatalf("provider request = %+v", request)
	}
	if got.GetPredicateName() != mtproto.Predicate_auth_sentCodePaymentRequired || got.GetPhoneCodeHash() != hash || got.GetStoreProduct() != "telegram-premium-login" || got.GetCurrency() != "USD" || got.GetAmount() != 199 || got.GetPremiumDays() != 30 {
		t.Fatalf("payment-required result = %+v", got)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("payment-required result did not encode: %v", err)
	}
}

func TestAuthCheckPaidAuthRejectsUntrustedProviderResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request paidAuthProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		response := paidAuthProviderResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			PhoneNumber: request.PhoneNumber, PhoneCodeHash: "different-hash",
			StoreProduct: "telegram-premium-login", Currency: "USD", Amount: 199,
		}
		body, _ := json.Marshal(response)
		w.Header().Set("X-Teamgram-Auth-Provider-Signature", authProviderSignature(authProviderTestSigningKey, body))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c := &AuthorizationCore{
		ctx: context.Background(), Logger: logx.WithContext(context.Background()),
		MD: &metadata.RpcMetadata{PermAuthKeyId: 1},
		svcCtx: &svc.ServiceContext{Config: config.Config{
			AuthProviderEndpoint: server.URL, AuthProviderKey: "provider-key", AuthProviderSigningKey: authProviderTestSigningKey,
		}, Dao: &dao.Dao{}},
	}
	got, err := c.AuthCheckPaidAuth(&mtproto.TLAuthCheckPaidAuth{PhoneNumber: "+14155552671", PhoneCodeHash: "expected-hash", FormId: 1})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("check paid auth = (%#v, %v), want provider-invalid refusal", got, err)
	}
}
