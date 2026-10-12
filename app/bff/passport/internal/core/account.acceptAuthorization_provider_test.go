package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passport/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/passport/internal/svc"
)

func TestPassportProviderFixtureHostRequiresExplicitMode(t *testing.T) {
	parsed, err := url.Parse("http://identity-provider-fixture.test:23192")
	if err != nil {
		t.Fatal(err)
	}
	if securePassportProviderURL(parsed, false) {
		t.Fatal("fixture host accepted without explicit fixture mode")
	}
	if !securePassportProviderURL(parsed, true) {
		t.Fatal("fixture host rejected in explicit fixture mode")
	}
}

const passportProviderTestSigningKey = "passport-provider-test-key-with-at-least-32-bytes"

func TestAccountAcceptAuthorizationUsesVerifiedProvider(t *testing.T) {
	const callerID int64 = 8801200
	const botID int64 = 8801201
	scope := `{"v":1,"nonce":"n","data":[{"type":"passport"}]}`
	publicKey := "public-key"
	var request passportAuthorizationProviderRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read provider request: %v", err)
		}
		if !verifyPassportProviderSignature(passportProviderTestSigningKey, r.Header.Get("X-Teamgram-Passport-Provider-Request-Signature"), body) {
			t.Fatal("provider request signature is invalid")
		}
		if got := r.Header.Get("Idempotency-Key"); len(got) != 64 {
			t.Fatalf("provider idempotency key length = %d, want 64", len(got))
		}
		if err = json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		response := passportAuthorizationProviderResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			UserID: request.UserID, BotID: request.BotID, Scope: request.Scope, PublicKey: request.PublicKey,
			Result: passportBoolPtr(true),
		}
		responseBody, _ := json.Marshal(response)
		w.Header().Set("X-Teamgram-Passport-Provider-Signature", passportProviderSignature(passportProviderTestSigningKey, responseBody))
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()
	caller := passportTestUser(callerID, 100, false)
	bot := passportTestUser(botID, 101, true)
	c := passportVerifiedUserCore(callerID, false, caller, bot)
	c.svcCtx = &svc.ServiceContext{Config: config.Config{
		PassportProviderEndpoint: server.URL, PassportProviderKey: "provider-key",
		PassportProviderSigningKey: passportProviderTestSigningKey,
	}, Dao: nil}
	c.ctx = context.Background()
	credentials := &mtproto.SecureCredentialsEncrypted{Data: []byte("encrypted"), Hash: []byte("hash"), Secret: []byte("secret")}
	got, err := c.AccountAcceptAuthorization(&mtproto.TLAccountAcceptAuthorization{
		BotId: botID, Scope: scope, PublicKey: publicKey, ValueHashes: []*mtproto.SecureValueHash{{Hash: []byte("value-hash")}}, Credentials: credentials,
	})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("accept authorization = (%v, %v), want BoolTrue", got, err)
	}
	if request.Operation != "account.acceptAuthorization" || request.UserID != callerID || request.BotID != botID || request.Scope != scope || request.PublicKey != publicKey || request.Credentials == nil || len(request.ValueHashes) != 1 {
		t.Fatalf("provider request = %+v", request)
	}
}

func TestAccountAcceptAuthorizationRejectsUntrustedProviderResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request passportAuthorizationProviderRequest
		_ = json.Unmarshal(body, &request)
		responseBody, _ := json.Marshal(passportAuthorizationProviderResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			UserID: request.UserID + 1, BotID: request.BotID, Scope: request.Scope, PublicKey: request.PublicKey, Result: passportBoolPtr(true),
		})
		w.Header().Set("X-Teamgram-Passport-Provider-Signature", passportProviderSignature(passportProviderTestSigningKey, responseBody))
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()
	callerID, botID := int64(8801202), int64(8801203)
	c := passportVerifiedUserCore(callerID, false, passportTestUser(callerID, 100, false), passportTestUser(botID, 101, true))
	c.svcCtx = &svc.ServiceContext{Config: config.Config{
		PassportProviderEndpoint: server.URL, PassportProviderSigningKey: passportProviderTestSigningKey,
	}}
	got, err := c.AccountAcceptAuthorization(&mtproto.TLAccountAcceptAuthorization{
		BotId: botID, Scope: `{"v":1,"nonce":"n","data":[]}`, PublicKey: "pk",
		Credentials: &mtproto.SecureCredentialsEncrypted{Data: []byte("encrypted")},
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("accept authorization = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}

func passportBoolPtr(value bool) *bool { return &value }
