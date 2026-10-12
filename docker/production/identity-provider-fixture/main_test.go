package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const identityFixtureSigningKey = "identity-provider-fixture-signing-key-0123456789"

func signedIdentityRequest(t *testing.T, p *provider, header string, request any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-key")
	req.Header.Set("Idempotency-Key", "fixture-key")
	req.Header.Set(header, p.sign(body))
	recorder := httptest.NewRecorder()
	p.handle(recorder, req)
	return recorder
}

func TestIdentityProviderDispatchesConfiguredOperations(t *testing.T) {
	p := &provider{
		authKey:    "provider-key",
		signingKey: []byte(identityFixtureSigningKey),
		webTokens:  map[string]int64{"web-token": 42},
		allowedBots: map[int64]struct{}{
			43: {},
		},
	}

	auth := signedIdentityRequest(t, p, "X-Teamgram-Auth-Provider-Request-Signature", authRequest{
		RequestID: "auth-request", Operation: "verify_web_token_authorization", APIID: 1, APIHash: "0123456789abcdef0123456789abcdef", WebAuthToken: "web-token",
	})
	if auth.Code != http.StatusOK {
		t.Fatalf("auth response status=%d body=%s", auth.Code, auth.Body.String())
	}
	var authResult authResponse
	if err := json.Unmarshal(auth.Body.Bytes(), &authResult); err != nil {
		t.Fatal(err)
	}
	if !authResult.Verified || authResult.UserID != 42 {
		t.Fatalf("auth result=%+v", authResult)
	}

	passport := signedIdentityRequest(t, p, "X-Teamgram-Passport-Provider-Request-Signature", passportRequest{
		RequestID: "passport-request", Operation: "account.acceptAuthorization", UserID: 42, BotID: 43, Scope: "scope", PublicKey: "key", Credentials: json.RawMessage(`{"data":"encrypted"}`),
	})
	if passport.Code != http.StatusOK {
		t.Fatalf("passport response status=%d body=%s", passport.Code, passport.Body.String())
	}
	var passportResult passportResponse
	if err := json.Unmarshal(passport.Body.Bytes(), &passportResult); err != nil {
		t.Fatal(err)
	}
	if !passportResult.Verified || passportResult.Result == nil || !*passportResult.Result {
		t.Fatalf("passport result=%+v", passportResult)
	}

	connection := signedIdentityRequest(t, p, "X-Teamgram-Bot-Connection-Provider-Request-Signature", connectionRequest{
		RequestID: "connection-request", Operation: "account.confirmBotConnection", UserID: 42, Payload: json.RawMessage(`{"bot_id":43}`),
	})
	if connection.Code != http.StatusOK {
		t.Fatalf("connection response status=%d body=%s", connection.Code, connection.Body.String())
	}
	body, _ := io.ReadAll(connection.Body)
	var connectionResult connectionResponse
	if err := json.Unmarshal(body, &connectionResult); err != nil {
		t.Fatal(err)
	}
	if !connectionResult.Verified {
		t.Fatalf("connection result=%+v", connectionResult)
	}

	paidAuth := signedIdentityRequest(t, p, "X-Teamgram-Auth-Provider-Request-Signature", paidAuthRequest{
		RequestID: "paid-auth-request", Operation: "auth.checkPaidAuth", PhoneNumber: "+12025550105", PhoneCodeHash: "paid-code-hash", FormID: 990001,
	})
	if paidAuth.Code != http.StatusOK {
		t.Fatalf("paid auth response status=%d body=%s", paidAuth.Code, paidAuth.Body.String())
	}
	var paidAuthResult paidAuthResponse
	if err := json.Unmarshal(paidAuth.Body.Bytes(), &paidAuthResult); err != nil {
		t.Fatal(err)
	}
	if !paidAuthResult.Verified || paidAuthResult.PhoneNumber != "+12025550105" || paidAuthResult.PhoneCodeHash != "paid-code-hash" || paidAuthResult.PremiumDays != 30 || paidAuthResult.Amount != 499 {
		t.Fatalf("paid auth result=%+v", paidAuthResult)
	}
}

func TestIdentityProviderRejectsUnprovisionedValuesAndBadTransport(t *testing.T) {
	p := &provider{
		authKey:    "provider-key",
		signingKey: []byte(identityFixtureSigningKey),
		webTokens:  map[string]int64{"web-token": 42},
		allowedBots: map[int64]struct{}{
			43: {},
		},
	}
	unknown := signedIdentityRequest(t, p, "X-Teamgram-Auth-Provider-Request-Signature", authRequest{
		RequestID: "unknown", Operation: "verify_web_token_authorization", APIID: 1, APIHash: "0123456789abcdef0123456789abcdef", WebAuthToken: "missing",
	})
	var unknownResult authResponse
	if err := json.Unmarshal(unknown.Body.Bytes(), &unknownResult); err != nil {
		t.Fatal(err)
	}
	if unknownResult.Verified || unknownResult.Error == "" {
		t.Fatalf("unknown auth token result=%+v", unknownResult)
	}

	body := []byte(`{"request_id":"bad","operation":"verify_web_token_authorization","api_id":1,"api_hash":"0123456789abcdef0123456789abcdef","web_auth_token":"web-token"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-key")
	req.Header.Set("Idempotency-Key", "fixture-key")
	req.Header.Set("X-Teamgram-Auth-Provider-Request-Signature", "sha256=00")
	recorder := httptest.NewRecorder()
	p.handle(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestParseIdentityProviderConfiguration(t *testing.T) {
	tokens, err := parseWebTokens("web-token=42,second-token=43")
	if err != nil || tokens["web-token"] != 42 || tokens["second-token"] != 43 {
		t.Fatalf("tokens=%v err=%v", tokens, err)
	}
	bots, err := parseBotIDs("43,44")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bots[43]; !ok {
		t.Fatalf("bots=%v", bots)
	}
	if _, err = parseWebTokens("broken"); err == nil {
		t.Fatal("accepted malformed token mapping")
	}
	if _, err = parseBotIDs("0"); err == nil {
		t.Fatal("accepted invalid bot id")
	}
}
