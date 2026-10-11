package server

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/bff/internal/config"
)

func TestApplyProviderEnvironmentLoadsCDNProvider(t *testing.T) {
	t.Setenv("TEAMGRAM_CDN_PROVIDER_ENDPOINT", "  https://cdn-provider.internal/v1  ")
	t.Setenv("TEAMGRAM_CDN_PROVIDER_KEY", "edge-api-key")
	t.Setenv("TEAMGRAM_CDN_PROVIDER_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("TEAMGRAM_CDN_PROVIDER_TIMEOUT_SECONDS", "17")

	var cfg config.Config
	if err := applyProviderEnvironment(&cfg); err != nil {
		t.Fatalf("applyProviderEnvironment() error = %v", err)
	}
	if cfg.CdnProviderEndpoint != "https://cdn-provider.internal/v1" {
		t.Fatalf("endpoint = %q", cfg.CdnProviderEndpoint)
	}
	if cfg.CdnProviderKey != "edge-api-key" {
		t.Fatalf("provider key = %q", cfg.CdnProviderKey)
	}
	if cfg.CdnProviderSigningKey != "01234567890123456789012345678901" {
		t.Fatalf("signing key = %q", cfg.CdnProviderSigningKey)
	}
	if cfg.CdnProviderTimeoutSeconds != 17 {
		t.Fatalf("timeout = %d", cfg.CdnProviderTimeoutSeconds)
	}
}

func TestApplyProviderEnvironmentRejectsInvalidCDNTimeout(t *testing.T) {
	t.Setenv("TEAMGRAM_CDN_PROVIDER_TIMEOUT_SECONDS", "301")
	var cfg config.Config
	if err := applyProviderEnvironment(&cfg); err == nil {
		t.Fatal("applyProviderEnvironment() accepted timeout above the provider limit")
	}
}
