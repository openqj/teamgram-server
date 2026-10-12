package server

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/bff/internal/config"
	passkeyhelper "github.com/teamgram/teamgram-server/app/bff/passkey"
)

func TestApplyPasskeyEnvironmentLoadsDeploymentPolicy(t *testing.T) {
	t.Setenv("TEAMGRAM_PASSKEY_RP_ID", " passkey.example.com ")
	t.Setenv("TEAMGRAM_PASSKEY_RP_DISPLAY_NAME", " Teamgram WebAuthn ")
	t.Setenv("TEAMGRAM_PASSKEY_ORIGINS", "https://passkey.example.com, https://login.passkey.example.com")
	t.Setenv("TEAMGRAM_PASSKEY_TRUSTED_APPS", "17349:0123456789abcdef0123456789abcdef, 17350:ABCDEF0123456789ABCDEF0123456789")

	var cfg config.Config
	if err := applyPasskeyEnvironment(&cfg); err != nil {
		t.Fatalf("applyPasskeyEnvironment() error = %v", err)
	}
	if cfg.Passkey.RelyingPartyId != "passkey.example.com" {
		t.Fatalf("rp id = %q", cfg.Passkey.RelyingPartyId)
	}
	if cfg.Passkey.RelyingPartyDisplayName != "Teamgram WebAuthn" {
		t.Fatalf("rp display name = %q", cfg.Passkey.RelyingPartyDisplayName)
	}
	if len(cfg.Passkey.Origins) != 2 || cfg.Passkey.Origins[1] != "https://login.passkey.example.com" {
		t.Fatalf("origins = %#v", cfg.Passkey.Origins)
	}
	if len(cfg.Passkey.TrustedApps) != 2 || cfg.Passkey.TrustedApps[0].ApiId != 17349 || cfg.Passkey.TrustedApps[1].ApiHash != "ABCDEF0123456789ABCDEF0123456789" {
		t.Fatalf("trusted apps = %#v", cfg.Passkey.TrustedApps)
	}
}

func TestApplyPasskeyEnvironmentRejectsMalformedTrustedApp(t *testing.T) {
	t.Setenv("TEAMGRAM_PASSKEY_TRUSTED_APPS", "17349:not-a-valid-api-hash")
	var cfg config.Config
	if err := applyPasskeyEnvironment(&cfg); err == nil {
		t.Fatal("applyPasskeyEnvironment() accepted malformed trusted app")
	}
}

func TestApplyPasskeyEnvironmentAllowsDisabledDefaults(t *testing.T) {
	t.Setenv("TEAMGRAM_PASSKEY_RP_ID", "")
	t.Setenv("TEAMGRAM_PASSKEY_RP_DISPLAY_NAME", "")
	t.Setenv("TEAMGRAM_PASSKEY_ORIGINS", "")
	t.Setenv("TEAMGRAM_PASSKEY_TRUSTED_APPS", "")
	cfg := config.Config{}
	cfg.Passkey.Origins = []string{"https://fixture.example.com"}
	cfg.Passkey.TrustedApps = []passkeyhelper.TrustedApp{{ApiId: 1, ApiHash: "0123456789abcdef0123456789abcdef"}}
	if err := applyPasskeyEnvironment(&cfg); err != nil {
		t.Fatalf("applyPasskeyEnvironment() error = %v", err)
	}
	if cfg.Passkey.RelyingPartyId != "" || cfg.Passkey.RelyingPartyDisplayName != "" || cfg.Passkey.Origins != nil || cfg.Passkey.TrustedApps != nil {
		t.Fatalf("empty deployment policy was not cleared: %#v", cfg.Passkey)
	}
}
