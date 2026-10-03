package code

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

func TestEmailProviderUsesLegacyNameSelectorWithConfiguredEndpoint(t *testing.T) {
	var delivered bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered = r.Method == http.MethodPost
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider := NewEmailProvider(&conf.SmsVerifyCodeConfig{
		Name:             " HTTP ",
		EmailSendCodeUrl: server.URL,
	})
	if err := provider.Ready(); err != nil {
		t.Fatalf("provider ready: %v", err)
	}
	if err := provider.Deliver(context.Background(), Delivery{
		Channel: ChannelEmail, Destination: "person@example.com", Code: "123456",
	}); err != nil {
		t.Fatalf("provider delivery: %v", err)
	}
	if !delivered {
		t.Fatal("email provider did not use the configured HTTP endpoint")
	}
}
