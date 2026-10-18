package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/providercontract"
)

const businessFixtureTestKey = "business-provider-fixture-test-signing-key-0123456789"

type healthProbeStore struct {
	businessStore
	err error
}

func (s *healthProbeStore) Ping(context.Context) error { return s.err }

func signedFixtureRequest(t *testing.T, fixture *server, request requestEnvelope) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Teamgram-Business-Provider-Request-Signature", fixture.sign(body))
	req.Header.Set("Idempotency-Key", providercontract.IdempotencyKey(request.UserID, request.Operation, request.Payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, req)
	return recorder
}

func TestSignedBusinessProviderDispatchesAndIsIdempotent(t *testing.T) {
	store := newMemoryStore()
	fixture := &server{key: []byte(businessFixtureTestKey), store: store, maxAge: 5}
	payload := json.RawMessage(`{"random_id":99,"peer":{"predicate_name":"inputPeerSelf"}}`)
	first := signedFixtureRequest(t, fixture, requestEnvelope{RequestID: "request-1", Operation: "stories.startLive", UserID: 42, Payload: payload})
	if first.Code != http.StatusOK {
		t.Fatalf("first response status = %d, body=%s", first.Code, first.Body.String())
	}
	second := signedFixtureRequest(t, fixture, requestEnvelope{RequestID: "request-2", Operation: "stories.startLive", UserID: 42, Payload: payload})
	if second.Code != http.StatusOK {
		t.Fatalf("replay response status = %d, body=%s", second.Code, second.Body.String())
	}
	var firstEnvelope, secondEnvelope responseEnvelope
	if err := json.Unmarshal(first.Body.Bytes(), &firstEnvelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondEnvelope); err != nil {
		t.Fatal(err)
	}
	if !firstEnvelope.Verified || !secondEnvelope.Verified || firstEnvelope.Operation != "stories.startLive" {
		t.Fatalf("envelopes = %+v / %+v", firstEnvelope, secondEnvelope)
	}
	if !bytes.Equal(firstEnvelope.Result, secondEnvelope.Result) {
		t.Fatalf("idempotent updates changed: %s != %s", firstEnvelope.Result, secondEnvelope.Result)
	}
	var updates map[string]any
	if err := json.Unmarshal(firstEnvelope.Result, &updates); err != nil || updates["predicate_name"] != "updates" {
		t.Fatalf("updates = %s, %v", firstEnvelope.Result, err)
	}

	conflict := signedFixtureRequest(t, fixture, requestEnvelope{RequestID: "request-3", Operation: "stories.startLive", UserID: 42, Payload: json.RawMessage(`{"random_id":99,"peer":{"predicate_name":"inputPeerSelf"},"caption":"different"}`)})
	var conflictEnvelope responseEnvelope
	if err := json.Unmarshal(conflict.Body.Bytes(), &conflictEnvelope); err != nil {
		t.Fatal(err)
	}
	if conflictEnvelope.Verified || conflictEnvelope.Error == "" {
		t.Fatalf("conflicting retry unexpectedly verified: %+v", conflictEnvelope)
	}
}

func TestBusinessProviderRejectsBadSignatureAndMissingRows(t *testing.T) {
	fixture := &server{key: []byte(businessFixtureTestKey), store: newMemoryStore(), maxAge: 5}
	body := []byte(`{"request_id":"r","operation":"fragment.getCollectibleInfo","user_id":42,"payload":{"collectible":{"username":"name"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Teamgram-Business-Provider-Request-Signature", "sha256="+hex.EncodeToString(make([]byte, sha256.Size)))
	r := httptest.NewRecorder()
	fixture.handle(r, req)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d", r.Code)
	}

	valid := requestEnvelope{RequestID: "r", Operation: "fragment.getCollectibleInfo", UserID: 42, Payload: json.RawMessage(`{"collectible":{"username":"name"}}`)}
	missing := signedFixtureRequest(t, fixture, valid)
	var envelope responseEnvelope
	if err := json.Unmarshal(missing.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Verified || envelope.Error == "" || len(envelope.Result) != 0 {
		t.Fatalf("missing collectible response = %+v", envelope)
	}
}

func TestBusinessProviderDispatchesPaymentOperationsByFingerprint(t *testing.T) {
	store := newMemoryStore()
	payload := json.RawMessage(`{"request":{"gift_id":101}}`)
	fingerprint := sha256.Sum256(payload)
	store.paymentOps["42:payments.getStarGiftUpgradePreview:"+hex.EncodeToString(fingerprint[:])] = []byte(`{"predicate_name":"payments_starGiftUpgradePreview","sample_attributes":[],"prices":[],"next_prices":[]}`)
	fixture := &server{key: []byte(businessFixtureTestKey), store: store, maxAge: 5}
	response := signedFixtureRequest(t, fixture, requestEnvelope{
		RequestID: "payment-request", Operation: "payments.getStarGiftUpgradePreview", UserID: 42, Payload: payload,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("payment response status = %d, body=%s", response.Code, response.Body.String())
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Verified || envelope.Operation != "payments.getStarGiftUpgradePreview" || len(envelope.Result) == 0 {
		t.Fatalf("payment response = %+v", envelope)
	}

	transferPayload := json.RawMessage(`{"stargift":{"predicate_name":"inputSavedStarGiftChat","saved_id":77},"to_id":{"predicate_name":"inputPeerUser","user_id":8,"access_hash":8008}}`)
	transferFingerprint := sha256.Sum256(transferPayload)
	store.paymentOps["42:payments.transferStarGift:"+hex.EncodeToString(transferFingerprint[:])] = []byte(`{"predicate_name":"updates","updates":[],"users":[],"chats":[],"date":1760000000,"seq":0}`)
	transferResponse := signedFixtureRequest(t, fixture, requestEnvelope{
		RequestID: "transfer-request", Operation: "payments.transferStarGift", UserID: 42, Payload: transferPayload,
	})
	if transferResponse.Code != http.StatusOK {
		t.Fatalf("transfer response status = %d, body=%s", transferResponse.Code, transferResponse.Body.String())
	}
	var transferEnvelope responseEnvelope
	if err := json.Unmarshal(transferResponse.Body.Bytes(), &transferEnvelope); err != nil {
		t.Fatal(err)
	}
	if !transferEnvelope.Verified || transferEnvelope.Operation != "payments.transferStarGift" || len(transferEnvelope.Result) == 0 {
		t.Fatalf("transfer response = %+v", transferEnvelope)
	}
}

func TestBusinessProviderWebViewDataEchoMode(t *testing.T) {
	fixture := &server{key: []byte(businessFixtureTestKey), store: newMemoryStore(), maxAge: 5, webViewDataMode: "echo"}
	response := signedFixtureRequest(t, fixture, requestEnvelope{
		RequestID: "webview-data-request", Operation: "messages.sendWebViewData", UserID: 42,
		Payload: json.RawMessage(`{"random_id":99,"button_text":"Open","data":"confirm"}`),
	})
	if response.Code != http.StatusOK {
		t.Fatalf("echo response status = %d, body=%s", response.Code, response.Body.String())
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Verified || string(envelope.Result) == "" {
		t.Fatalf("echo response = %+v", envelope)
	}
	var result map[string]any
	if err := json.Unmarshal(envelope.Result, &result); err != nil || result["predicate_name"] != "updates" {
		t.Fatalf("echo result = %s, err=%v", envelope.Result, err)
	}
}

func TestBusinessProviderDispatchesMiniAppOperationsByFingerprint(t *testing.T) {
	store := newMemoryStore()
	payloads := map[string]json.RawMessage{
		"messages.getBotApp":                json.RawMessage(`{"app":{"predicate_name":"inputBotAppID","id":7}}`),
		"messages.requestAppWebView":        json.RawMessage(`{"scope_key":"mini-app","peer":{"predicate_name":"inputPeerSelf"}}`),
		"messages.sendWebViewData":          json.RawMessage(`{"random_id":99,"button_text":"Open","data":"confirm"}`),
		"messages.sendWebViewResultMessage": json.RawMessage(`{"bot_query_id":"query-1","result":{"predicate_name":"botInlineResult","id":"1","type":"article","send_message":{"predicate_name":"botInlineMessageText","message":"ok"}}}`),
	}
	for operation, payload := range payloads {
		fingerprint := sha256.Sum256(payload)
		store.botOps[fmt.Sprintf("42:%s:%x", operation, fingerprint[:])] = []byte(`{"predicate_name":"updates","updates":[],"users":[],"chats":[],"date":1760000000,"seq":0}`)
	}
	fixture := &server{key: []byte(businessFixtureTestKey), store: store, maxAge: 5}
	for operation, payload := range payloads {
		response := signedFixtureRequest(t, fixture, requestEnvelope{
			RequestID: operation + "-request", Operation: operation, UserID: 42, Payload: payload,
		})
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body=%s", operation, response.Code, response.Body.String())
		}
		var envelope responseEnvelope
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("%s response: %v", operation, err)
		}
		if !envelope.Verified || envelope.Operation != operation || len(envelope.Result) == 0 {
			t.Fatalf("%s envelope = %+v", operation, envelope)
		}
	}
}

func TestBusinessProviderRequiresBearerAuthorization(t *testing.T) {
	fixture := &server{key: []byte(businessFixtureTestKey), authKey: "provider-key", store: newMemoryStore(), maxAge: 5}
	request := requestEnvelope{
		RequestID: "auth-request", Operation: "fragment.getCollectibleInfo", UserID: 42,
		Payload: json.RawMessage(`{"collectible":{"username":"name"}}`),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, authorization := range []string{"", "provider-key", "Bearer wrong", "Bearer  provider-key"} {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Authorization", authorization)
		req.Header.Set("X-Teamgram-Business-Provider-Request-Signature", fixture.sign(body))
		recorder := httptest.NewRecorder()
		fixture.handle(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q status=%d body=%s", authorization, recorder.Code, recorder.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-key")
	req.Header.Set("X-Teamgram-Business-Provider-Request-Signature", fixture.sign(body))
	req.Header.Set("Idempotency-Key", providercontract.IdempotencyKey(request.UserID, request.Operation, request.Payload))
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid authorization status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBusinessProviderRequiresPayloadBoundIdempotencyKey(t *testing.T) {
	fixture := &server{key: []byte(businessFixtureTestKey), store: newMemoryStore(), maxAge: 5}
	request := requestEnvelope{
		RequestID: "idempotency-request", Operation: "fragment.getCollectibleInfo", UserID: 42,
		Payload: json.RawMessage(`{"collectible":{"username":"name"}}`),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]string{
		"missing": "",
		"wrong":   strings.Repeat("0", 64),
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("X-Teamgram-Business-Provider-Request-Signature", fixture.sign(body))
			req.Header.Set("Idempotency-Key", key)
			recorder := httptest.NewRecorder()
			fixture.handle(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("idempotency key %q status=%d body=%s", name, recorder.Code, recorder.Body.String())
			}
		})
	}
	valid := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	valid.Header.Set("X-Teamgram-Business-Provider-Request-Signature", fixture.sign(body))
	valid.Header.Set("Idempotency-Key", providercontract.IdempotencyKey(request.UserID, request.Operation, request.Payload))
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, valid)
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid idempotency key status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBusinessProviderHealthzChecksConfiguredStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "healthy", want: http.StatusOK},
		{name: "database unavailable", err: errors.New("database unavailable"), want: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &server{store: &healthProbeStore{businessStore: newMemoryStore(), err: tc.err}}
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			recorder := httptest.NewRecorder()
			fixture.healthz(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("health status=%d, want %d", recorder.Code, tc.want)
			}
		})
	}
	fixture := &server{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	fixture.healthz(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil store health status=%d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestBusinessProviderSignatureUsesHMACSHA256(t *testing.T) {
	fixture := &server{key: []byte(businessFixtureTestKey)}
	body := []byte("body")
	got := fixture.sign(body)
	mac := hmac.New(sha256.New, []byte(businessFixtureTestKey))
	_, _ = mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want || !verifySignature(fixture.key, got, body) {
		t.Fatalf("signature = %q, want %q", got, want)
	}
	if _, err := io.ReadAll(bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
}
