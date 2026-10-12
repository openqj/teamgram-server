package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testCodeFixture() *fixture {
	return newFixture("provider-key", "provider-secret")
}

func signedDeliveryRequest(t *testing.T, f *fixture, value delivery) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-key")
	req.Header.Set("X-Provider-Secret", "provider-secret")
	req.Header.Set("Idempotency-Key", value.DeliveryID)
	recorder := httptest.NewRecorder()
	f.send(recorder, req)
	return recorder
}

func TestCodeFixtureStoresSMSAndEmailDeliveries(t *testing.T) {
	f := testCodeFixture()
	sms := delivery{
		Channel: "sms", DeliveryID: "sms-delivery", Destination: "+12025550106", Code: "12345",
		ChallengeID: "sms-challenge", Purpose: "auth.login",
	}
	if recorder := signedDeliveryRequest(t, f, sms); recorder.Code != http.StatusAccepted {
		t.Fatalf("SMS status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	mail := delivery{
		Channel: "email", DeliveryID: "email-delivery", Destination: "person@example.test", Code: "654321",
		ChallengeID: "email-challenge", Purpose: "account.verify_email",
	}
	if recorder := signedDeliveryRequest(t, f, mail); recorder.Code != http.StatusAccepted {
		t.Fatalf("email status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/latest?challenge_id=sms-challenge", nil)
	req.Header.Set("Authorization", "Bearer provider-key")
	req.Header.Set("X-Provider-Secret", "provider-secret")
	recorder := httptest.NewRecorder()
	f.latestDelivery(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("latest status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var got deliveryRecord
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Code != sms.Code || got.ChallengeID != sms.ChallengeID || got.Channel != sms.Channel {
		t.Fatalf("latest delivery=%+v, want %+v", got, sms)
	}
}

func TestCodeFixtureEnforcesAuthAndIdempotency(t *testing.T) {
	f := testCodeFixture()
	value := delivery{
		Channel: "sms", DeliveryID: "delivery", Destination: "+12025550106", Code: "12345",
		ChallengeID: "challenge", Purpose: "auth.login",
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	unauthorized.Header.Set("Idempotency-Key", value.DeliveryID)
	recorder := httptest.NewRecorder()
	f.send(recorder, unauthorized)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", recorder.Code)
	}

	if recorder := signedDeliveryRequest(t, f, value); recorder.Code != http.StatusAccepted {
		t.Fatalf("first delivery status=%d", recorder.Code)
	}
	if recorder := signedDeliveryRequest(t, f, value); recorder.Code != http.StatusAccepted {
		t.Fatalf("idempotent retry status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	value.Code = "54321"
	if recorder := signedDeliveryRequest(t, f, value); recorder.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCodeFixtureRejectsMalformedDelivery(t *testing.T) {
	f := testCodeFixture()
	for _, body := range []string{
		`{"channel":"push","delivery_id":"id","destination":"+1","code":"12345","challenge_id":"c","purpose":"auth.login"}`,
		`{"channel":"sms","delivery_id":"id","destination":"+1","code":"","challenge_id":"c","purpose":"auth.login"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer provider-key")
		req.Header.Set("X-Provider-Secret", "provider-secret")
		req.Header.Set("Idempotency-Key", "id")
		recorder := httptest.NewRecorder()
		f.send(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %s status=%d", body, recorder.Code)
		}
	}
}
