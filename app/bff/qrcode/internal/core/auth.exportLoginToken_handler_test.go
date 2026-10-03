package core

import (
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
)

func TestValidateQRAppCredentials(t *testing.T) {
	tests := []struct {
		name    string
		apiID   int32
		apiHash string
		wantErr bool
	}{
		{name: "valid shape", apiID: 1, apiHash: "0123456789abcdef0123456789abcdef"},
		{name: "uppercase hex", apiID: 1, apiHash: "0123456789ABCDEF0123456789ABCDEF"},
		{name: "zero api id", apiID: 0, apiHash: "0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "negative api id", apiID: -1, apiHash: "0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "empty api hash", apiID: 1, apiHash: "", wantErr: true},
		{name: "wrong hash length", apiID: 1, apiHash: "0123456789abcdef", wantErr: true},
		{name: "non hex hash", apiID: 1, apiHash: "0123456789abcdeg0123456789abcdef", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQRAppCredentials(tt.apiID, tt.apiHash)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateQRAppCredentials() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err != nil && mtproto.NewRpcError(err).GetErrorMessage() != "API_ID_INVALID" {
				t.Fatalf("unexpected API credential error: %v", err)
			}
		})
	}
}

func TestCanonicalQRAppHash(t *testing.T) {
	got, err := canonicalQRAppHash(1, "0123456789ABCDEF0123456789ABCDEF")
	if err != nil {
		t.Fatalf("canonicalQRAppHash() error = %v", err)
	}
	if want := "0123456789abcdef0123456789abcdef"; got != want {
		t.Fatalf("canonicalQRAppHash() = %q, want %q", got, want)
	}

	if _, err = canonicalQRAppHash(0, "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("canonicalQRAppHash() accepted invalid api id")
	}
}

func TestAuthExportLoginTokenRejectsNilOrUnavailableCore(t *testing.T) {
	if got, err := (&QrCodeCore{}).AuthExportLoginToken(nil); got != nil || err != mtproto.ErrApiIdInvalid {
		t.Fatalf("AuthExportLoginToken(nil) = (%#v, %v), want (nil, API_ID_INVALID)", got, err)
	}
	request := &mtproto.TLAuthExportLoginToken{ApiId: 1, ApiHash: "0123456789abcdef0123456789abcdef"}
	if got, err := (&QrCodeCore{}).AuthExportLoginToken(request); got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("AuthExportLoginToken(unavailable) = (%#v, %v), want (nil, AUTH_KEY_UNREGISTERED)", got, err)
	}
}

func TestNormalizeQRExceptIDs(t *testing.T) {
	got, err := normalizeQRExceptIDs([]int64{42, 7, 42})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{7, 42}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeQRExceptIDs() = %v, want %v", got, want)
	}

	if _, err = normalizeQRExceptIDs([]int64{42, 0}); err == nil || mtproto.NewRpcError(err).GetErrorMessage() != "USER_ID_INVALID" {
		t.Fatalf("normalizeQRExceptIDs() invalid user error = %v, want USER_ID_INVALID", err)
	}
	if got, err = normalizeQRExceptIDs(nil); err != nil || !reflect.DeepEqual(got, []int64{}) {
		t.Fatalf("normalizeQRExceptIDs(nil) = %v, %v, want empty slice", got, err)
	}
}

func TestCheckQRLoginExceptIDs(t *testing.T) {
	qrCode := &model.QRCodeTransaction{ExceptIDs: []int64{7, 42}}
	if err := checkQRLoginExceptIDs(qrCode, 42); err == nil || mtproto.NewRpcError(err).GetErrorMessage() != "AUTH_TOKEN_INVALID" {
		t.Fatalf("excluded account error = %v, want AUTH_TOKEN_INVALID", err)
	}
	if err := checkQRLoginExceptIDs(qrCode, 43); err != nil {
		t.Fatalf("allowed account error = %v, want nil", err)
	}
}

func TestVerifyQRLoginAuthKeyBinding(t *testing.T) {
	if err := verifyQRLoginAuthKeyBinding(42, &mtproto.Int64{V: 42}); err != nil {
		t.Fatalf("matching binding error = %v, want nil", err)
	}

	for _, tt := range []struct {
		name     string
		userID   int64
		response *mtproto.Int64
	}{
		{name: "missing response", userID: 42},
		{name: "unbound key", userID: 42, response: &mtproto.Int64{}},
		{name: "different user", userID: 42, response: &mtproto.Int64{V: 43}},
		{name: "invalid accepted user", userID: 0, response: &mtproto.Int64{V: 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyQRLoginAuthKeyBinding(tt.userID, tt.response)
			if err == nil || mtproto.NewRpcError(err).GetErrorMessage() != "AUTH_TOKEN_INVALID" {
				t.Fatalf("binding error = %v, want AUTH_TOKEN_INVALID", err)
			}
		})
	}
}

func TestValidateQRPermAuthKeyIDAcceptsSignedMTProtoIDs(t *testing.T) {
	for _, authKeyID := range []int64{1, -1} {
		if err := validateQRPermAuthKeyID(authKeyID); err != nil {
			t.Fatalf("validateQRPermAuthKeyID(%d) error = %v, want nil", authKeyID, err)
		}
	}

	if err := validateQRPermAuthKeyID(0); err == nil || mtproto.NewRpcError(err).GetErrorMessage() != "AUTH_KEY_INVALID" {
		t.Fatalf("validateQRPermAuthKeyID(0) error = %v, want AUTH_KEY_INVALID", err)
	}
}
