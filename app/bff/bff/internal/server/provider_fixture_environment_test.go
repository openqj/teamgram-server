package server

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/bff/internal/config"
)

func TestApplyProviderEnvironmentFixtureMode(t *testing.T) {
	t.Setenv("TEAMGRAM_PROVIDER_FIXTURE_MODE", "true")
	var cfg config.Config
	if err := applyProviderEnvironment(&cfg); err != nil {
		t.Fatalf("applyProviderEnvironment() error = %v", err)
	}
	if !cfg.ProviderFixtureMode {
		t.Fatal("provider fixture mode was not enabled")
	}

	t.Setenv("TEAMGRAM_PROVIDER_FIXTURE_MODE", "invalid")
	if err := applyProviderEnvironment(&cfg); err == nil {
		t.Fatal("applyProviderEnvironment() accepted an invalid fixture mode")
	}
}

func TestApplyProviderEnvironmentLoadsPaymentProvider(t *testing.T) {
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_ENDPOINT", "  https://payment-provider.internal/v1  ")
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_KEY", "payment-api-key")
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_TIMEOUT_SECONDS", "19")

	var cfg config.Config
	if err := applyProviderEnvironment(&cfg); err != nil {
		t.Fatalf("applyProviderEnvironment() error = %v", err)
	}
	if cfg.PaymentProviderEndpoint != "https://payment-provider.internal/v1" {
		t.Fatalf("endpoint = %q", cfg.PaymentProviderEndpoint)
	}
	if cfg.PaymentProviderKey != "payment-api-key" {
		t.Fatalf("provider key = %q", cfg.PaymentProviderKey)
	}
	if cfg.PaymentProviderSigningKey != "01234567890123456789012345678901" {
		t.Fatalf("signing key = %q", cfg.PaymentProviderSigningKey)
	}
	if cfg.PaymentProviderTimeoutSeconds != 19 {
		t.Fatalf("timeout = %d", cfg.PaymentProviderTimeoutSeconds)
	}
}
