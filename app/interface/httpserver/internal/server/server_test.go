package server

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/config"
)

func TestApplyPaymentProviderEnvironment(t *testing.T) {
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_NAME", " provider-test ")
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_SIGNING_KEY", " webhook-secret ")
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_MAX_BODY_BYTES", "2048")

	c := config.Config{}
	if err := applyPaymentProviderEnvironment(&c); err != nil {
		t.Fatalf("applyPaymentProviderEnvironment() error = %v", err)
	}
	if c.PaymentProviderName != "provider-test" {
		t.Fatalf("PaymentProviderName = %q, want provider-test", c.PaymentProviderName)
	}
	if c.PaymentProviderWebhookSigningKey != "webhook-secret" {
		t.Fatalf("PaymentProviderWebhookSigningKey = %q, want webhook-secret", c.PaymentProviderWebhookSigningKey)
	}
	if c.PaymentProviderWebhookMaxBodyBytes != 2048 {
		t.Fatalf("PaymentProviderWebhookMaxBodyBytes = %d, want 2048", c.PaymentProviderWebhookMaxBodyBytes)
	}
}

func TestApplyPaymentProviderEnvironmentRejectsInvalidBodyLimit(t *testing.T) {
	t.Setenv("TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_MAX_BODY_BYTES", "not-a-number")
	if err := applyPaymentProviderEnvironment(&config.Config{}); err == nil {
		t.Fatal("applyPaymentProviderEnvironment() error = nil, want invalid body limit error")
	}
}

func TestApplyIdentityEnvironment(t *testing.T) {
	t.Setenv("TEAMGRAM_CORS_ORIGINS", " http://localhost:1235,https://web.example.com ")
	t.Setenv("TEAMGRAM_IDENTITY_BASE_URL", " https://identity.internal ")
	t.Setenv("TEAMGRAM_IDENTITY_TEAMGRAM_SERVICE_TOKEN", " teamgram-secret ")
	t.Setenv("TEAMGRAM_IDENTITY_INSTANCE_ID", " instance-a ")
	t.Setenv("TEAMGRAM_SUB2API_SERVICE_TOKEN", " sub2api-secret ")
	t.Setenv("TEAMGRAM_IDENTITY_REQUEST_TIMEOUT_SECONDS", "7")

	c := config.Config{}
	if err := applyIdentityEnvironment(&c); err != nil {
		t.Fatalf("applyIdentityEnvironment() error = %v", err)
	}
	if c.IdentityBaseURL != "https://identity.internal" {
		t.Fatalf("IdentityBaseURL = %q", c.IdentityBaseURL)
	}
	if c.IdentityTeamgramServiceToken != "teamgram-secret" {
		t.Fatalf("IdentityTeamgramServiceToken = %q", c.IdentityTeamgramServiceToken)
	}
	if c.IdentityTeamgramInstanceID != "instance-a" {
		t.Fatalf("IdentityTeamgramInstanceID = %q", c.IdentityTeamgramInstanceID)
	}
	if c.Sub2APIServiceToken != "sub2api-secret" {
		t.Fatalf("Sub2API service token = %q", c.Sub2APIServiceToken)
	}
	if c.IdentityRequestTimeoutSeconds != 7 {
		t.Fatalf("IdentityRequestTimeoutSeconds = %d", c.IdentityRequestTimeoutSeconds)
	}
	if got := splitCorsOrigins(c.CorsOrigins); len(got) != 2 || got[0] != "http://localhost:1235" || got[1] != "https://web.example.com" {
		t.Fatalf("CorsOrigins = %#v", got)
	}
}

func TestApplyIdentityEnvironmentRejectsInvalidTimeout(t *testing.T) {
	t.Setenv("TEAMGRAM_IDENTITY_REQUEST_TIMEOUT_SECONDS", "not-a-number")
	if err := applyIdentityEnvironment(&config.Config{}); err == nil {
		t.Fatal("applyIdentityEnvironment() error = nil, want invalid timeout error")
	}
}

func TestSplitCorsOriginsTrimsAndDropsEmptyValues(t *testing.T) {
	got := splitCorsOrigins(" http://localhost:1235, ,https://web.example.com ")
	want := []string{"http://localhost:1235", "https://web.example.com"}
	if len(got) != len(want) {
		t.Fatalf("origins = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("origins = %#v, want %#v", got, want)
		}
	}
}
