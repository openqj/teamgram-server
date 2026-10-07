package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestPasskeyUserHandleUsesClientDcFormat(t *testing.T) {
	user := &passkeyUser{id: 42, dcID: 2}
	if got := string(user.WebAuthnID()); got != "2:42" {
		t.Fatalf("WebAuthnID() = %q, want 2:42", got)
	}
	if got, ok := userHandleID([]byte("2:42")); !ok || got != 42 {
		t.Fatalf("userHandleID() = (%d, %v), want (42, true)", got, ok)
	}
	if got, ok := userHandleDcID([]byte("2:42")); !ok || got != 2 {
		t.Fatalf("userHandleDcID() = (%d, %v), want (2, true)", got, ok)
	}
	for _, handle := range []string{"0:42", "2:0", "invalid:42", "2:42:extra", "+2:42", "02:42", "2:042"} {
		if _, ok := userHandleID([]byte(handle)); ok {
			t.Fatalf("userHandleID(%q) unexpectedly accepted", handle)
		}
	}
	for _, handle := range [][]byte{
		{0, 0, 0, 0, 0, 0, 0, 42},
		[]byte("42"),
		[]byte("AgAAAAAAAAA"),
	} {
		if _, ok := userHandleID(handle); ok {
			t.Fatalf("legacy user handle %q unexpectedly accepted", handle)
		}
	}
}

func TestPasskeyFinishRejectsUnpairedMigrationAuthKey(t *testing.T) {
	c := &PasskeyCore{
		svcCtx: &svc.ServiceContext{
			Config: config.Config{DcId: 2, Provider: config.ProviderConfig{
				RelyingPartyId: "login.example.com", RelyingPartyDisplayName: "Example",
				Origins: []string{"https://login.example.com"},
			}},
			Dao: &dao.Dao{},
		},
	}
	_, err := c.AuthFinishPasskeyLogin(&mtproto.TLAuthFinishPasskeyLogin{
		Credential:    &mtproto.InputPasskeyCredential{Id: "credential"},
		FromAuthKeyId: wrapperspb.Int64(42),
	})
	if !errors.Is(err, mtproto.ErrAuthKeyInvalid) {
		t.Fatalf("unpaired from_auth_key_id error = %v, want AUTH_KEY_INVALID", err)
	}
	_, err = c.AuthFinishPasskeyLogin(&mtproto.TLAuthFinishPasskeyLogin{
		Credential:    &mtproto.InputPasskeyCredential{Id: "credential"},
		FromDcId:      wrapperspb.Int32(2),
		FromAuthKeyId: wrapperspb.Int64(-42),
	})
	if !errors.Is(err, mtproto.ErrAuthKeyInvalid) {
		t.Fatalf("negative from_auth_key_id error = %v, want AUTH_KEY_INVALID", err)
	}
}

func TestPasskeyProviderConfigurationRejectsLocalOrUntrustedValues(t *testing.T) {
	valid := config.ProviderConfig{
		RelyingPartyId:          "login.example.com",
		RelyingPartyDisplayName: "Example",
		Origins:                 []string{"https://login.example.com"},
		TrustedApps: []config.TrustedApp{{
			ApiId: 17349, ApiHash: "344583e45741c457fe1862106095a5eb",
		}},
	}
	if !passkeyProviderConfigured(valid) {
		t.Fatal("valid HTTPS relying-party configuration was rejected")
	}
	if !passkeyTrustedApp(valid, 17349, "344583e45741c457fe1862106095a5eb") {
		t.Fatal("configured app credential was rejected")
	}

	for _, provider := range []config.ProviderConfig{
		{RelyingPartyId: "localhost", RelyingPartyDisplayName: "Local", Origins: []string{"https://localhost"}},
		{RelyingPartyId: "127.0.0.1", RelyingPartyDisplayName: "Local", Origins: []string{"https://127.0.0.1"}},
		{RelyingPartyId: "example.com", RelyingPartyDisplayName: "Example", Origins: []string{"http://example.com"}},
		{RelyingPartyId: "example.com", RelyingPartyDisplayName: "Example", Origins: []string{"https://attacker.example.net"}},
		{RelyingPartyId: "example.com", RelyingPartyDisplayName: "Example", Origins: []string{"https://example.com/login"}},
		{RelyingPartyId: "example.com", RelyingPartyDisplayName: "Example", Origins: []string{"https://user@example.com"}},
		{RelyingPartyId: "example.com", RelyingPartyDisplayName: "Example", Origins: []string{"https://example.com?rp=other"}},
		{RelyingPartyId: "example.com", RelyingPartyDisplayName: "Example", Origins: []string{"https://example.com#rp"}},
		{RelyingPartyId: "com", RelyingPartyDisplayName: "Example", Origins: []string{"https://example.com"}},
		{RelyingPartyId: "co.uk", RelyingPartyDisplayName: "Example", Origins: []string{"https://example.co.uk"}},
		{RelyingPartyId: "example.com:443", RelyingPartyDisplayName: "Example", Origins: []string{"https://example.com"}},
		{RelyingPartyId: "example.com", RelyingPartyDisplayName: "   ", Origins: []string{"https://example.com"}},
	} {
		if passkeyProviderConfigured(provider) {
			t.Fatalf("unsafe provider configuration accepted: %+v", provider)
		}
	}
	if passkeyTrustedApp(valid, 17349, "00000000000000000000000000000000") {
		t.Fatal("unregistered app hash was accepted")
	}
}

func TestPasskeyWebAuthnOptionsUseConfiguredRP(t *testing.T) {
	c := &PasskeyCore{svcCtx: &svc.ServiceContext{
		Config: config.Config{DcId: 2, Provider: config.ProviderConfig{
			RelyingPartyId: "login.example.com", RelyingPartyDisplayName: "Example",
			Origins: []string{"https://login.example.com"},
		}},
	}}
	w, err := c.webAuthn()
	if err != nil {
		t.Fatalf("webAuthn() error = %v", err)
	}
	creation, _, err := w.BeginRegistration(&passkeyUser{id: 42, dcID: 2, name: "user", displayName: "User"})
	if err != nil {
		t.Fatalf("BeginRegistration() error = %v", err)
	}
	if got := creation.Response.RelyingParty.ID; got != "login.example.com" {
		t.Fatalf("registration RP ID = %q, want login.example.com", got)
	}
	login, _, err := w.BeginDiscoverableLogin()
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin() error = %v", err)
	}
	if got := login.Response.RelyingPartyID; got != "login.example.com" {
		t.Fatalf("login RP ID = %q, want login.example.com", got)
	}
}

func TestPasskeyRequiresConfiguredDc(t *testing.T) {
	c := &PasskeyCore{
		MD: &metadata.RpcMetadata{UserId: 1},
		svcCtx: &svc.ServiceContext{Config: config.Config{
			Provider: config.ProviderConfig{
				RelyingPartyId: "login.example.com", RelyingPartyDisplayName: "Example",
				Origins: []string{"https://login.example.com"},
			},
		}},
	}
	if err := c.requireProvider(); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("Passkey provider without local DcId error = %v, want METHOD_NOT_IMPL", err)
	}
	c.svcCtx.Config.DcId = 2
	if err := c.requireProvider(); err != nil {
		t.Fatalf("configured Passkey provider rejected: %v", err)
	}
}

func TestPasskeyDataJSONIsTLWrapped(t *testing.T) {
	data := passkeyDataJSON(`{"publicKey":{"challenge":"test"}}`)
	if data.GetPredicateName() != "dataJSON" {
		t.Fatalf("predicate = %q, want dataJSON", data.GetPredicateName())
	}
	options := mtproto.MakeTLAuthPasskeyLoginOptions(&mtproto.Auth_PasskeyLoginOptions{Options: data})
	if err := options.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("auth.passkeyLoginOptions encoding failed: %v", err)
	}
}

func TestPasskeyCredentialIDsMustMatch(t *testing.T) {
	base := &mtproto.InputPasskeyCredential{
		Id:    "AQ",
		RawId: "Ag",
		Response: &mtproto.InputPasskeyResponse{
			ClientData:        &mtproto.DataJSON{Data: "{}"},
			AuthenticatorData: []byte{1},
			Signature:         []byte{2},
		},
	}
	if _, err := credentialResponsePayload(base, false); !errors.Is(err, mtproto.ErrAuthTokenInvalid) {
		t.Fatalf("mismatched passkey IDs error = %v, want AUTH_TOKEN_INVALID", err)
	}
	base.RawId = base.Id
	if _, err := credentialResponsePayload(base, false); err != nil {
		t.Fatalf("matching passkey IDs rejected: %v", err)
	}
	base.RawId = ""
	if _, err := credentialResponsePayload(base, false); !errors.Is(err, mtproto.ErrAuthTokenInvalid) {
		t.Fatalf("missing raw_id error = %v, want AUTH_TOKEN_INVALID", err)
	}
}

func TestPasskeyAccountMethodsFailClosedWithoutProvider(t *testing.T) {
	c := &PasskeyCore{MD: &metadata.RpcMetadata{UserId: 1}}
	credential := &mtproto.InputPasskeyCredential{Id: "unverified-credential"}

	if got, err := c.AccountInitPasskeyRegistration(&mtproto.TLAccountInitPasskeyRegistration{}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AccountInitPasskeyRegistration() = (%#v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
	if got, err := c.AccountRegisterPasskey(&mtproto.TLAccountRegisterPasskey{Credential: credential}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AccountRegisterPasskey() = (%#v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
	if got, err := c.AccountGetPasskeys(&mtproto.TLAccountGetPasskeys{}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AccountGetPasskeys() = (%#v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
	if got, err := c.AccountDeletePasskey(&mtproto.TLAccountDeletePasskey{Id: credential.GetId()}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AccountDeletePasskey() = (%#v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestPasskeySourceDcIsLocalOnly(t *testing.T) {
	if err := validatePasskeySourceDc(1, 1); err != nil {
		t.Fatalf("local source DC rejected: %v", err)
	}
	for _, sourceDC := range []int32{0, 2, -1} {
		if err := validatePasskeySourceDc(1, sourceDC); !errors.Is(err, mtproto.ErrDcIdInvalid) {
			t.Fatalf("source DC %d error = %v, want DC_ID_INVALID", sourceDC, err)
		}
	}
	if err := validatePasskeySourceDc(0, 1); !errors.Is(err, mtproto.ErrDcIdInvalid) {
		t.Fatalf("unknown local DC error = %v, want DC_ID_INVALID", err)
	}
}

func TestAuthFinishPasskeyLoginFailsClosedWithoutVerifier(t *testing.T) {
	credential := &mtproto.InputPasskeyCredential{Id: "unverified-credential"}
	c := &PasskeyCore{}
	if got, err := c.AuthFinishPasskeyLogin(&mtproto.TLAuthFinishPasskeyLogin{Credential: credential}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AuthFinishPasskeyLogin() = (%#v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}

	if got, err := c.AuthFinishPasskeyLogin(nil); got != nil || !errors.Is(err, mtproto.ErrAuthTokenInvalid) {
		t.Fatalf("AuthFinishPasskeyLogin(nil) = (%#v, %v), want (nil, AUTH_TOKEN_INVALID)", got, err)
	}
}
