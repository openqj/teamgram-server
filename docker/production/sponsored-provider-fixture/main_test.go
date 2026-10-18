package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/providercontract"
)

const sponsoredFixtureTestKey = "sponsored-provider-fixture-test-signing-key-0123456789"

func signedRequest(t *testing.T, fixture *server, request requestEnvelope) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+fixture.authKey)
	req.Header.Set("Idempotency-Key", sponsoredRequestID(request.Operation, request.UserID, request.Payload))
	req.Header.Set(requestHeader, fixture.sign(body))
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, req)
	return recorder
}

func TestSponsoredProviderDispatchesStableLayer229Results(t *testing.T) {
	fixture := &server{key: sponsoredFixtureTestKey, authKey: "provider-key", query: "fixture", peerID: 900000000001, maxAge: time.Second}
	peersPayload := json.RawMessage(`{"q":"fixture"}`)
	peersRequest := requestEnvelope{
		RequestID: sponsoredRequestID("contacts.getSponsoredPeers", 42, peersPayload),
		Operation: "contacts.getSponsoredPeers", UserID: 42, Payload: peersPayload,
	}
	firstPeers := signedRequest(t, fixture, peersRequest)
	secondPeers := signedRequest(t, fixture, peersRequest)
	if firstPeers.Code != http.StatusOK || secondPeers.Code != http.StatusOK {
		t.Fatalf("peer responses = %d/%d", firstPeers.Code, secondPeers.Code)
	}
	var firstEnvelope, secondEnvelope providercontract.ResponseEnvelope
	if err := json.Unmarshal(firstPeers.Body.Bytes(), &firstEnvelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(secondPeers.Body.Bytes(), &secondEnvelope); err != nil {
		t.Fatal(err)
	}
	if !firstEnvelope.Verified || firstEnvelope.Error != "" || !bytes.Equal(firstEnvelope.Result, secondEnvelope.Result) {
		t.Fatalf("peer envelopes = %+v / %+v", firstEnvelope, secondEnvelope)
	}
	var peers map[string]any
	if err := json.Unmarshal(firstEnvelope.Result, &peers); err != nil {
		t.Fatal(err)
	}
	if peers["predicate_name"] != "contacts_sponsoredPeers" {
		t.Fatalf("peer predicate = %v", peers["predicate_name"])
	}
	var peersTL mtproto.Contacts_SponsoredPeers
	if err := json.Unmarshal(firstEnvelope.Result, &peersTL); err != nil {
		t.Fatal(err)
	}
	if peersTL.GetPredicateName() != mtproto.Predicate_contacts_sponsoredPeers || len(peersTL.GetPeers()) != 1 || peersTL.GetPeers()[0].GetPeer().GetUserId() != 900000000001 {
		t.Fatalf("decoded peer result = %v", peersTL)
	}

	messagesPayload := json.RawMessage(`{"peer":{"predicate_name":"inputPeerSelf"}}`)
	messagesRequest := requestEnvelope{
		RequestID: sponsoredRequestID("messages.getSponsoredMessages", 42, messagesPayload),
		Operation: "messages.getSponsoredMessages", UserID: 42, Payload: messagesPayload,
	}
	messagesResponse := signedRequest(t, fixture, messagesRequest)
	if messagesResponse.Code != http.StatusOK {
		t.Fatalf("message response status = %d", messagesResponse.Code)
	}
	var messagesEnvelope providercontract.ResponseEnvelope
	if err := json.Unmarshal(messagesResponse.Body.Bytes(), &messagesEnvelope); err != nil {
		t.Fatal(err)
	}
	var messages map[string]any
	if err := json.Unmarshal(messagesEnvelope.Result, &messages); err != nil {
		t.Fatal(err)
	}
	if messages["predicate_name"] != "messages_sponsoredMessages" {
		t.Fatalf("message predicate = %v", messages["predicate_name"])
	}
	var messagesTL mtproto.Messages_SponsoredMessages
	if err := json.Unmarshal(messagesEnvelope.Result, &messagesTL); err != nil {
		t.Fatal(err)
	}
	if messagesTL.GetPredicateName() != mtproto.Predicate_messages_sponsoredMessages || len(messagesTL.GetMessages()) != 1 || messagesTL.GetMessages()[0].GetMessage() == "" {
		t.Fatalf("decoded message result = %v", messagesTL)
	}
	encoded, _ := json.Marshal(messages)
	if !strings.Contains(string(encoded), "signed sponsored fixture for user 42") {
		t.Fatalf("message result is not bound to user: %s", encoded)
	}

	messageList := messages["messages"].([]any)
	randomID := messageList[0].(map[string]any)["random_id"].(string)
	if _, err := base64.StdEncoding.DecodeString(randomID); err != nil {
		t.Fatalf("random_id is not base64: %v", err)
	}
	otherPayload := json.RawMessage(`{"peer":{"predicate_name":"inputPeerUser","user_id":7,"access_hash":1}}`)
	otherRequest := requestEnvelope{
		RequestID: sponsoredRequestID("messages.getSponsoredMessages", 42, otherPayload),
		Operation: "messages.getSponsoredMessages", UserID: 42, Payload: otherPayload,
	}
	otherResponse := signedRequest(t, fixture, otherRequest)
	var otherEnvelope providercontract.ResponseEnvelope
	if err := json.Unmarshal(otherResponse.Body.Bytes(), &otherEnvelope); err != nil {
		t.Fatal(err)
	}
	var otherMessages struct {
		Messages []struct {
			RandomID string `json:"random_id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(otherEnvelope.Result, &otherMessages); err != nil {
		t.Fatal(err)
	}
	otherRandomID := ""
	if len(otherMessages.Messages) == 1 {
		otherRandomID = otherMessages.Messages[0].RandomID
	}
	if otherRandomID == "" || otherRandomID == randomID {
		t.Fatalf("peer payload did not change random_id: %q/%q", randomID, otherRandomID)
	}
}

func TestSponsoredProviderRejectsInvalidAuthenticationAndIdempotency(t *testing.T) {
	fixture := &server{key: sponsoredFixtureTestKey, authKey: "provider-key", query: "fixture", peerID: 900000000001, maxAge: time.Second}
	payload := json.RawMessage(`{"q":"fixture"}`)
	request := requestEnvelope{
		RequestID: sponsoredRequestID("contacts.getSponsoredPeers", 42, payload),
		Operation: "contacts.getSponsoredPeers", UserID: 42, Payload: payload,
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	badSignature := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	badSignature.Header.Set("Authorization", "Bearer provider-key")
	badSignature.Header.Set("Idempotency-Key", request.RequestID)
	badSignature.Header.Set(requestHeader, "sha256="+strings.Repeat("0", sha256.Size*2))
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, badSignature)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d", recorder.Code)
	}

	wrongID := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	wrongID.Header.Set("Authorization", "Bearer provider-key")
	wrongID.Header.Set("Idempotency-Key", "wrong")
	wrongID.Header.Set(requestHeader, fixture.sign(body))
	recorder = httptest.NewRecorder()
	fixture.handle(recorder, wrongID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("wrong idempotency status = %d", recorder.Code)
	}
	var envelope providercontract.ResponseEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Verified || envelope.Error == "" || len(envelope.Result) != 0 {
		t.Fatalf("wrong idempotency response = %+v", envelope)
	}
}

func TestSponsoredProviderHealthz(t *testing.T) {
	fixture := &server{}
	recorder := httptest.NewRecorder()
	fixture.healthz(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d", recorder.Code)
	}
}
