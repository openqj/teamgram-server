package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const cdnFixtureTestKey = "cdn-provider-fixture-test-signing-key-0123456789"

func signedCDNRequest(t *testing.T, fixture *server, request requestEnvelope) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Teamgram-CDN-Provider-Request-Signature", fixture.sign(body))
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, req)
	return recorder
}

func TestCDNFixtureReadinessRequiresPostgresStore(t *testing.T) {
	fixture := &server{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	fixture.healthz(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing PostgreSQL store health status=%d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestCDNFixtureRequiresBearerAuthorization(t *testing.T) {
	fixture := &server{key: []byte(cdnFixtureTestKey), authKey: "provider-key", store: newMemoryStore(), timeout: 5}
	request := requestEnvelope{RequestID: "auth-request", Operation: "help.getCdnConfig", UserID: 0, Payload: json.RawMessage(`{}`)}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, authorization := range []string{"", "provider-key", "Bearer wrong", "Bearer  provider-key"} {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Authorization", authorization)
		req.Header.Set("X-Teamgram-CDN-Provider-Request-Signature", fixture.sign(body))
		recorder := httptest.NewRecorder()
		fixture.handle(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q status=%d body=%s", authorization, recorder.Code, recorder.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-key")
	req.Header.Set("X-Teamgram-CDN-Provider-Request-Signature", fixture.sign(body))
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid authorization status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCDNFixtureSeedRequiresBearerAuthorization(t *testing.T) {
	store := newMemoryStore()
	fixture := &server{key: []byte(cdnFixtureTestKey), authKey: "provider-key", store: store, timeout: 5}
	body := []byte(`{"source_key":"seed-source"}`)
	for _, authorization := range []string{"", "provider-key", "Bearer wrong", "Bearer  provider-key"} {
		req := httptest.NewRequest(http.MethodPut, "/seed", bytes.NewReader(body))
		req.Header.Set("Authorization", authorization)
		recorder := httptest.NewRecorder()
		fixture.handle(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q status=%d body=%s", authorization, recorder.Code, recorder.Body.String())
		}
	}
	if _, err := store.BySource(nil, "seed-source"); err == nil {
		t.Fatal("unauthorized seed unexpectedly wrote an object")
	}

	req := httptest.NewRequest(http.MethodPut, "/seed", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-key")
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("valid seed status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := store.BySource(nil, "seed-source"); err != nil {
		t.Fatalf("authorized seed did not persist object: %v", err)
	}
}

func TestCDNFixtureRedirectChunksHashesAndReupload(t *testing.T) {
	store := newMemoryStore()
	fixture := &server{key: []byte(cdnFixtureTestKey), store: store, timeout: 5}
	location := json.RawMessage(`{"predicate_name":"inputDocumentFileLocation","id":42,"access_hash":99}`)
	object := deriveObject(sourceKey(location))
	store.objects[sourceKey(location)] = object
	store.objects[hex.EncodeToString(object.Token)] = object
	store.objects["__cdn_config__"] = &cdnObject{Data: []byte(`{"predicate_name":"cdnConfig","public_keys":[]}`)}
	redirectPayload, _ := json.Marshal(map[string]any{"location": json.RawMessage(location), "offset": 0, "limit": 1024})
	redirect := signedCDNRequest(t, fixture, requestEnvelope{RequestID: "redirect", Operation: "upload.getFile", UserID: 0, Payload: redirectPayload})
	if redirect.Code != http.StatusOK {
		t.Fatalf("redirect status=%d body=%s", redirect.Code, redirect.Body.String())
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(redirect.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Verified {
		t.Fatalf("redirect envelope = %+v", envelope)
	}
	var result struct {
		Predicate string            `json:"predicate_name"`
		Token     []byte            `json:"file_token"`
		Key       []byte            `json:"encryption_key"`
		IV        []byte            `json:"encryption_iv"`
		Hashes    []json.RawMessage `json:"file_hashes"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Predicate != "upload_fileCdnRedirect" || len(result.Token) == 0 || len(result.Key) != 32 || len(result.IV) != 16 || len(result.Hashes) == 0 {
		t.Fatalf("redirect result = %+v", result)
	}
	tokenJSON, _ := json.Marshal(result.Token)
	chunkPayload, _ := json.Marshal(map[string]any{"file_token": json.RawMessage(tokenJSON), "offset": 0, "limit": 8})
	chunk := signedCDNRequest(t, fixture, requestEnvelope{RequestID: "chunk", Operation: "upload.getCdnFile", UserID: 0, Payload: chunkPayload})
	if chunk.Code != http.StatusOK {
		t.Fatalf("chunk status=%d body=%s", chunk.Code, chunk.Body.String())
	}
	var chunkEnvelope responseEnvelope
	if err := json.Unmarshal(chunk.Body.Bytes(), &chunkEnvelope); err != nil {
		t.Fatal(err)
	}
	var chunkResult struct {
		Predicate string `json:"predicate_name"`
		Bytes     []byte `json:"bytes"`
	}
	if err := json.Unmarshal(chunkEnvelope.Result, &chunkResult); err != nil {
		t.Fatal(err)
	}
	if chunkResult.Predicate != "upload_cdnFile" || len(chunkResult.Bytes) != 8 {
		t.Fatalf("chunk result = %+v", chunkResult)
	}

	hashesPayload, _ := json.Marshal(map[string]any{"file_token": json.RawMessage(tokenJSON), "offset": 0})
	hashes := signedCDNRequest(t, fixture, requestEnvelope{RequestID: "hashes", Operation: "upload.getCdnFileHashes", UserID: 0, Payload: hashesPayload})
	if hashes.Code != http.StatusOK {
		t.Fatalf("hashes status=%d body=%s", hashes.Code, hashes.Body.String())
	}
	var hashesEnvelope responseEnvelope
	if err := json.Unmarshal(hashes.Body.Bytes(), &hashesEnvelope); err != nil {
		t.Fatal(err)
	}
	var hashesResult struct {
		Datas []json.RawMessage `json:"datas"`
	}
	if err := json.Unmarshal(hashesEnvelope.Result, &hashesResult); err != nil || len(hashesResult.Datas) == 0 {
		t.Fatalf("hashes result = %s, %v", hashesEnvelope.Result, err)
	}

	requestTokenPayload, _ := json.Marshal(map[string]any{"file_token": json.RawMessage(tokenJSON), "request_token": requestToken(fixture.key, result.Token)})
	reuploaded := signedCDNRequest(t, fixture, requestEnvelope{RequestID: "reupload", Operation: "upload.reuploadCdnFile", UserID: 0, Payload: requestTokenPayload})
	if reuploaded.Code != http.StatusOK {
		t.Fatalf("reupload status=%d body=%s", reuploaded.Code, reuploaded.Body.String())
	}
	if !bytes.Contains(reuploaded.Body.Bytes(), []byte(`"datas"`)) {
		t.Fatalf("reupload body=%s", reuploaded.Body.String())
	}
}

func TestCDNFixtureRejectsUnknownTokenAndBadSignature(t *testing.T) {
	fixture := &server{key: []byte(cdnFixtureTestKey), store: newMemoryStore(), timeout: 5}
	unknownToken, _ := json.Marshal(map[string]any{"file_token": []byte("unknown"), "offset": 0, "limit": 1})
	response := signedCDNRequest(t, fixture, requestEnvelope{RequestID: "unknown", Operation: "upload.getCdnFile", Payload: unknownToken})
	var envelope responseEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Verified || envelope.Error == "" {
		t.Fatalf("unknown token response = %+v", envelope)
	}

	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"request_id":"bad","operation":"help.getCdnConfig","user_id":0,"payload":{}}`)))
	request.Header.Set("X-Teamgram-CDN-Provider-Request-Signature", "sha256=00")
	recorder := httptest.NewRecorder()
	fixture.handle(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status=%d", recorder.Code)
	}
}
