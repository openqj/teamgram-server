package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/config"
)

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
	} {
		if passkeyProviderConfigured(provider) {
			t.Fatalf("unsafe provider configuration accepted: %+v", provider)
		}
	}
	if passkeyTrustedApp(valid, 17349, "00000000000000000000000000000000") {
		t.Fatal("unregistered app hash was accepted")
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
