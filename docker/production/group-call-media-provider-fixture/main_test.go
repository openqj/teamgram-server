package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func signedRequest(t *testing.T, fixture *server, req request, idempotency string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer "+fixture.apiKey)
	httpReq.Header.Set("X-Teamgram-Group-Call-Request-Signature", signature(fixture.signingKey, body))
	if idempotency != "" {
		httpReq.Header.Set("Idempotency-Key", idempotency)
	}
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, httpReq)
	return recorder
}

func TestSignedJoinIsIdempotentAndReleaseAllowsRejoin(t *testing.T) {
	fixture := newServer("api-key", "signing-key-with-at-least-32-bytes", "media.example:1935")
	req := request{RequestID: "r1", Operation: "join_group_call", UserID: 7, CallID: 11, ChannelID: 13}
	first := signedRequest(t, fixture, req, "join:11:7")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	var firstResponse response
	if err := json.Unmarshal(first.Body.Bytes(), &firstResponse); err != nil {
		t.Fatal(err)
	}
	second := signedRequest(t, fixture, request{RequestID: "r2", Operation: req.Operation, UserID: req.UserID, CallID: req.CallID, ChannelID: req.ChannelID}, "join:11:7")
	var secondResponse response
	if err := json.Unmarshal(second.Body.Bytes(), &secondResponse); err != nil {
		t.Fatal(err)
	}
	if firstResponse.MediaSource <= 0 || firstResponse.MediaSource != secondResponse.MediaSource {
		t.Fatalf("idempotent source changed: %#v / %#v", firstResponse, secondResponse)
	}
	leave := signedRequest(t, fixture, request{RequestID: "r3", Operation: "leave_group_call", UserID: req.UserID, CallID: req.CallID, ChannelID: req.ChannelID}, "leave:11:7")
	if leave.Code != http.StatusOK {
		t.Fatalf("leave status = %d, body = %s", leave.Code, leave.Body.String())
	}
	rejoin := signedRequest(t, fixture, request{RequestID: "r4", Operation: req.Operation, UserID: req.UserID, CallID: req.CallID, ChannelID: req.ChannelID}, "join:11:7-retry")
	var rejoinResponse response
	if err := json.Unmarshal(rejoin.Body.Bytes(), &rejoinResponse); err != nil {
		t.Fatal(err)
	}
	if rejoin.Code != http.StatusOK || rejoinResponse.MediaSource <= 0 || rejoinResponse.MediaSource == firstResponse.MediaSource {
		t.Fatalf("rejoin response = %#v, status = %d", rejoinResponse, rejoin.Code)
	}
}

func TestProviderRejectsUnsignedRequest(t *testing.T) {
	fixture := newServer("api-key", "signing-key-with-at-least-32-bytes", "media.example:1935")
	req := request{RequestID: "r1", Operation: "join_group_call", UserID: 7, CallID: 11, ChannelID: 13}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer "+fixture.apiKey)
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, httpReq)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
