package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func newCDNCore() *FilesCore {
	return &FilesCore{Logger: logx.WithContext(context.Background())}
}

func TestUploadGetCdnFileValidatesInputBeforeFailingClosed(t *testing.T) {
	tests := []struct {
		name string
		in   *mtproto.TLUploadGetCdnFile
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "empty file token", in: &mtproto.TLUploadGetCdnFile{Limit: 1}, want: mtproto.ErrFileTokenInvalid},
		{name: "negative offset", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Offset_INT64: -1, Limit: 1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative legacy offset", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Offset_INT32: -1, Limit: 1}, want: mtproto.ErrOffsetInvalid},
		{name: "zero limit", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}}, want: mtproto.ErrLimitInvalid},
		{name: "provider unavailable", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Limit: 1}, want: mtproto.ErrCdnMethodInvalid},
	}

	core := newCDNCore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.UploadGetCdnFile(tt.in)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("UploadGetCdnFile() = (%v, %v), want (%v)", got, err, tt.want)
			}
		})
	}
}

func TestUploadGetCdnFileHashesValidatesInputBeforeFailingClosed(t *testing.T) {
	tests := []struct {
		name string
		in   *mtproto.TLUploadGetCdnFileHashes
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "empty file token", in: &mtproto.TLUploadGetCdnFileHashes{}, want: mtproto.ErrFileTokenInvalid},
		{name: "negative offset", in: &mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}, Offset_INT64: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative legacy offset", in: &mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}, Offset_INT32: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "provider unavailable", in: &mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}}, want: mtproto.ErrCdnMethodInvalid},
	}

	core := newCDNCore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.UploadGetCdnFileHashes(tt.in)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("UploadGetCdnFileHashes() = (%v, %v), want (%v)", got, err, tt.want)
			}
		})
	}
}

func TestUploadReuploadCdnFileValidatesInputBeforeFailingClosed(t *testing.T) {
	tests := []struct {
		name string
		in   *mtproto.TLUploadReuploadCdnFile
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "empty file token", in: &mtproto.TLUploadReuploadCdnFile{RequestToken: []byte{2}}, want: mtproto.ErrFileTokenInvalid},
		{name: "empty request token", in: &mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}}, want: mtproto.ErrTokenInvalid},
		{name: "provider unavailable", in: &mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}, RequestToken: []byte{2}}, want: mtproto.ErrCdnMethodInvalid},
	}

	core := newCDNCore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.UploadReuploadCdnFile(tt.in)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("UploadReuploadCdnFile() = (%v, %v), want (%v)", got, err, tt.want)
			}
		})
	}
}

func TestMessagesGetDocumentByHashRejectsNilRequest(t *testing.T) {
	core := &FilesCore{}
	if got, err := core.MessagesGetDocumentByHash(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("MessagesGetDocumentByHash() = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}

func TestCDNMethodsDoNotPanicWithoutLogger(t *testing.T) {
	if got, err := (*FilesCore)(nil).UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Limit: 1}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("UploadGetCdnFile() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
	if got, err := (&FilesCore{}).UploadGetCdnFileHashes(&mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("UploadGetCdnFileHashes() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
	if got, err := (&FilesCore{}).UploadReuploadCdnFile(&mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}, RequestToken: []byte{2}}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("UploadReuploadCdnFile() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
}

func TestHelpGetCdnConfigFailsClosedWithoutProvider(t *testing.T) {
	core := newCDNCore()
	if got, err := core.HelpGetCdnConfig(&mtproto.TLHelpGetCdnConfig{}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("HelpGetCdnConfig() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
}

func TestCDNProviderReturnsVerifiedResults(t *testing.T) {
	const key = "cdn-provider-test-signing-key-0123456789"
	var reuploadIdempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RequestID == "" || req.UserID != 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var result any
		switch req.Operation {
		case "upload.getCdnFile":
			result = map[string]any{"predicate_name": "upload_cdnFile", "bytes": "AQI="}
		case "upload.getCdnFileHashes", "upload.reuploadCdnFile":
			if req.Operation == "upload.reuploadCdnFile" {
				reuploadIdempotencyKey = r.Header.Get("Idempotency-Key")
			}
			result = map[string]any{"datas": []any{map[string]any{
				"predicate_name": "fileHash",
				"offset_INT64":   0,
				"limit":          2,
				"hash":           "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
			}}}
		case "help.getCdnConfig":
			result = map[string]any{"predicate_name": "cdnConfig", "public_keys": []any{map[string]any{
				"predicate_name": "cdnPublicKey", "dc_id": 2, "public_key": "cdn-test-public-key",
			}}}
		default:
			http.Error(w, "unknown operation", http.StatusBadRequest)
			return
		}
		resultJSON, _ := json.Marshal(result)
		body, _ := json.Marshal(cdnProviderEnvelope{RequestID: req.RequestID, Operation: req.Operation, Verified: true, UserID: req.UserID, Result: resultJSON})
		w.Header().Set("X-Teamgram-CDN-Provider-Signature", cdnProviderSignature(key, body))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	core := &FilesCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{},
		Logger: logx.WithContext(context.Background()),
		svcCtx: &svc.ServiceContext{Config: config.Config{CdnProviderEndpoint: server.URL, CdnProviderSigningKey: key, CdnProviderTimeoutSeconds: 2, AllowInsecureProviderHTTP: true}},
	}
	file, err := core.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Limit: 2})
	if err != nil || file == nil || string(file.GetBytes()) != string([]byte{1, 2}) {
		t.Fatalf("cdn file = (%v, %v)", file, err)
	}
	hashes, err := core.UploadGetCdnFileHashes(&mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}})
	if err != nil || hashes == nil || len(hashes.GetDatas()) != 1 {
		t.Fatalf("cdn hashes = (%v, %v)", hashes, err)
	}
	reuploaded, err := core.UploadReuploadCdnFile(&mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}, RequestToken: []byte{2}})
	if err != nil || reuploaded == nil || len(reuploaded.GetDatas()) != 1 {
		t.Fatalf("cdn reupload hashes = (%v, %v)", reuploaded, err)
	}
	if len(reuploadIdempotencyKey) != 64 {
		t.Fatalf("cdn reupload idempotency key = %q, want 64-character key", reuploadIdempotencyKey)
	}
	cdn, err := core.HelpGetCdnConfig(&mtproto.TLHelpGetCdnConfig{})
	if err != nil || cdn == nil || cdn.GetPredicateName() == "" {
		t.Fatalf("cdn config = (%v, %v)", cdn, err)
	}
}

func TestCDNResultValidationRejectsMalformedValues(t *testing.T) {
	if validCDNFileResult(&mtproto.Upload_CdnFile{PredicateName: mtproto.Predicate_upload_cdnFile, Bytes: []byte{1}, RequestToken: []byte{2}}) {
		t.Fatal("cdn file accepted both bytes and request token")
	}
	if validCDNFileResult(&mtproto.Upload_CdnFile{PredicateName: "unknown", Bytes: []byte{1}}) {
		t.Fatal("cdn file accepted unknown constructor")
	}
	if validCDNFileHashVector(&mtproto.Vector_FileHash{Datas: []*mtproto.FileHash{{
		PredicateName: mtproto.Predicate_fileHash,
		Limit:         1,
		Hash:          []byte{1},
	}}}) {
		t.Fatal("cdn file hash accepted a non-SHA-256 hash")
	}
}

func TestUploadGetFileUsesVerifiedCDNRedirect(t *testing.T) {
	const key = "cdn-redirect-provider-signing-key-0123456789"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Operation != "upload.getFile" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resultJSON, _ := json.Marshal(map[string]any{
			"predicate_name": mtproto.Predicate_upload_fileCdnRedirect,
			"dc_id":          2,
			"file_token":     "AQI=",
			"encryption_key": "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
			"encryption_iv":  "AAECAwQFBgcICQoLDA0ODw==",
			"file_hashes": []any{map[string]any{
				"predicate_name": "fileHash",
				"offset_INT64":   0,
				"limit":          2,
				"hash":           "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
			}},
		})
		body, _ := json.Marshal(cdnProviderEnvelope{RequestID: req.RequestID, Operation: req.Operation, Verified: true, UserID: req.UserID, Result: resultJSON})
		w.Header().Set("X-Teamgram-CDN-Provider-Signature", cdnProviderSignature(key, body))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	core := newGetFileCore()
	core.svcCtx.Config = config.Config{CdnProviderEndpoint: server.URL, CdnProviderSigningKey: key, CdnProviderTimeoutSeconds: 2, AllowInsecureProviderHTTP: true}
	location := mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{Id: 9}).To_InputFileLocation()
	file, err := core.UploadGetFile(&mtproto.TLUploadGetFile{CdnSupported: true, Location: location, Limit: 2})
	if err != nil || file == nil || file.GetPredicateName() != mtproto.Predicate_upload_fileCdnRedirect || string(file.GetFileToken()) != string([]byte{1, 2}) {
		t.Fatalf("cdn redirect = (%v, %v)", file, err)
	}
}

func TestSecureCDNProviderURLRequiresFixtureModeForHTTP(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		fixture    bool
		wantSecure bool
	}{
		{name: "https production", endpoint: "https://cdn.example.com/v1", wantSecure: true},
		{name: "public HTTP production", endpoint: "http://cdn.example.com/v1", wantSecure: false},
		{name: "public HTTP fixture", endpoint: "http://cdn.example.com/v1", fixture: true, wantSecure: false},
		{name: "loopback HTTP production", endpoint: "http://127.0.0.1:23191", wantSecure: false},
		{name: "loopback HTTP fixture", endpoint: "http://127.0.0.1:23191", fixture: true, wantSecure: true},
		{name: "compose fixture production", endpoint: "http://cdn-provider-fixture:23191", wantSecure: false},
		{name: "compose fixture enabled", endpoint: "http://cdn-provider-fixture:23191", fixture: true, wantSecure: true},
		{name: "named fixture domain", endpoint: "http://cdn-provider-fixture.test:23191", fixture: true, wantSecure: true},
		{name: "fixture suffix spoof", endpoint: "http://cdn-provider-fixture.attacker.test.evil:23191", fixture: true, wantSecure: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := url.Parse(tt.endpoint)
			if err != nil {
				t.Fatal(err)
			}
			if got := secureCDNProviderURL(parsed, tt.fixture); got != tt.wantSecure {
				t.Fatalf("secureCDNProviderURL(%q, %v) = %v, want %v", tt.endpoint, tt.fixture, got, tt.wantSecure)
			}
		})
	}
}

func TestCanonicalCDNLocationOmitsWireConstructor(t *testing.T) {
	location := mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
		Constructor: mtproto.TLConstructor_CRC32_inputDocumentFileLocation,
		Id:          42,
		AccessHash:  99,
	}).To_InputFileLocation()
	got := canonicalCDNLocation(location)
	if got["predicate_name"] != mtproto.Predicate_inputDocumentFileLocation || got["id"] != int64(42) || got["access_hash"] != int64(99) {
		t.Fatalf("canonical location = %#v", got)
	}
	if _, found := got["constructor"]; found {
		t.Fatalf("canonical location leaked constructor: %#v", got)
	}
}
