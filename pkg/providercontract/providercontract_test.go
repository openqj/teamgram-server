package providercontract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSignatureAndRequestHeaders(t *testing.T) {
	body := []byte(`{"request_id":"r"}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://provider.example.test", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	SetRequestHeaders(req, body, " bearer ", " key ", "X-Teamgram-Provider-Signature", " signing-key ")
	if req.Header.Get("Authorization") != "Bearer bearer" || req.Header.Get("Idempotency-Key") != "key" {
		t.Fatalf("headers = %#v", req.Header)
	}
	if !VerifySignature("signing-key", req.Header.Get("X-Teamgram-Provider-Signature"), body) {
		t.Fatal("request signature did not verify")
	}
	if VerifySignature("signing-key", req.Header.Get("X-Teamgram-Provider-Signature"), []byte(`{"request_id":"other"}`)) {
		t.Fatal("signature accepted a different body")
	}
}

func TestEnvelopeAndIdempotency(t *testing.T) {
	payload := json.RawMessage(`{"amount":1}`)
	request := RequestEnvelope{RequestID: "r", Operation: "payments.settle", UserID: 42, Payload: payload}
	body, err := json.Marshal(ResponseEnvelope{RequestID: "r", Operation: request.Operation, Verified: true, UserID: 42, Result: json.RawMessage(`{"accepted":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ValidateResponse(body, request)
	if err != nil || string(response.Result) != `{"accepted":true}` {
		t.Fatalf("response = %#v, err = %v", response, err)
	}
	if IdempotencyKey(42, request.Operation, payload) != IdempotencyKey(42, request.Operation, payload) {
		t.Fatal("idempotency key is not stable")
	}
	if IdempotencyKey(42, request.Operation, payload) == IdempotencyKey(43, request.Operation, payload) {
		t.Fatal("idempotency key is not caller-bound")
	}
}

func TestVerifyResponseAndDeliveryState(t *testing.T) {
	body := []byte(`{"ok":true}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Teamgram-Provider-Signature", Signature("secret", body))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !VerifyResponse(resp, body, "X-Teamgram-Provider-Signature", "secret") {
		t.Fatal("response signature did not verify")
	}
	if !CanTransition("pending", "inflight") || !CanTransition("inflight", "complete") || !CanTransition("inflight", "pending") || CanTransition("complete", "pending") {
		t.Fatal("invalid delivery state transition accepted")
	}
}
