package code

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

func TestHTTPEmailProviderDeliversConfiguredRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("X-Provider-Secret"); got != "provider-secret" {
			t.Errorf("provider secret = %q", got)
		}
		var delivery Delivery
		if err := json.NewDecoder(r.Body).Decode(&delivery); err != nil {
			t.Errorf("decode delivery: %v", err)
		}
		if delivery.Channel != ChannelEmail || delivery.Destination != "person@example.com" || delivery.Code != "123456" || delivery.ChallengeID != "challenge" {
			t.Errorf("delivery = %+v", delivery)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	provider := NewEmailProvider(&conf.SmsVerifyCodeConfig{
		EmailProvider:          " HTTP ",
		EmailSendCodeUrl:       server.URL,
		Key:                    "api-key",
		Secret:                 "provider-secret",
		ProviderTimeoutSeconds: 2,
	})
	if err := provider.Ready(); err != nil {
		t.Fatalf("provider ready: %v", err)
	}
	if err := provider.Deliver(context.Background(), Delivery{
		Channel: ChannelEmail, Destination: "person@example.com", Code: "123456", ChallengeID: "challenge", Purpose: "verify",
	}); err != nil {
		t.Fatalf("provider delivery: %v", err)
	}
}

func TestHTTPSMSProviderUsesConfiguredHTTPWhenNoInjectedVerifier(t *testing.T) {
	var received Delivery
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode delivery: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	provider := NewSMSProvider(&conf.SmsVerifyCodeConfig{
		Name: "http", SendCodeUrl: server.URL,
	}, nil)
	if err := provider.Deliver(context.Background(), Delivery{
		Channel: ChannelSMS, Destination: "+14155552671", Code: "12345", ChallengeID: "challenge",
	}); err != nil {
		t.Fatalf("provider delivery: %v", err)
	}
	if received.Channel != ChannelSMS || received.Destination != "+14155552671" || received.Code != "12345" || received.ChallengeID != "challenge" {
		t.Fatalf("delivery = %+v", received)
	}
}

func TestHTTPSMSProviderSelectorOverridesLegacyName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var delivery Delivery
		if err := json.NewDecoder(r.Body).Decode(&delivery); err != nil {
			t.Errorf("decode delivery: %v", err)
		}
		if delivery.Code != "12345" {
			t.Errorf("delivery code = %q", delivery.Code)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	provider := NewSMSProvider(&conf.SmsVerifyCodeConfig{
		Name: "me", SMSProvider: "http", SendCodeUrl: server.URL,
	}, nil)
	if err := provider.Deliver(context.Background(), Delivery{
		Channel: ChannelSMS, Destination: "+14155552671", Code: "12345",
	}); err != nil {
		t.Fatalf("provider delivery: %v", err)
	}
}

func TestLegacyMESSMSProviderPreservesQueryContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("phone"); got != "+1 415&555" {
			t.Errorf("phone query = %q", got)
		}
		if got := r.URL.Query().Get("code"); got != "12 3?" {
			t.Errorf("code query = %q", got)
		}
		_, _ = io.WriteString(w, "accepted")
	}))
	defer server.Close()

	provider := NewSMSProvider(&conf.SmsVerifyCodeConfig{Name: "me", SendCodeUrl: server.URL}, nil)
	if err := provider.Ready(); err != nil {
		t.Fatalf("provider ready: %v", err)
	}
	if err := provider.Deliver(context.Background(), Delivery{
		Channel: ChannelSMS, Destination: "+1 415&555", Code: "12 3?", ChallengeID: "challenge",
	}); err != nil {
		t.Fatalf("provider delivery: %v", err)
	}
}

func TestLegacyMESSMSProviderRequiresValidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://sms.example.test/send"} {
		provider := NewSMSProvider(&conf.SmsVerifyCodeConfig{Name: "me", SendCodeUrl: endpoint}, nil)
		if !errors.Is(provider.Ready(), ErrProviderUnavailable) {
			t.Fatalf("endpoint %q ready = %v, want unavailable", endpoint, provider.Ready())
		}
	}
}

func TestHTTPMissingCodeReportProviderDeliversReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var delivery Delivery
		if err := json.NewDecoder(r.Body).Decode(&delivery); err != nil {
			t.Errorf("decode report: %v", err)
		}
		if delivery.Channel != ChannelSMS || delivery.Destination != "+14155552671" || delivery.ChallengeID != "challenge" || delivery.Purpose != "auth.reportMissingCode" || delivery.MNC != "310" || delivery.Code != "" {
			t.Errorf("report = %+v", delivery)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	provider := NewMissingCodeReportProvider(&conf.SmsVerifyCodeConfig{ReportMissingCodeUrl: server.URL})
	if err := provider.Ready(); err != nil {
		t.Fatalf("provider ready: %v", err)
	}
	if err := provider.Deliver(context.Background(), Delivery{
		Channel: ChannelSMS, Destination: "+14155552671", ChallengeID: "challenge", Purpose: "auth.reportMissingCode", MNC: "310",
	}); err != nil {
		t.Fatalf("provider report: %v", err)
	}
}

func TestMissingCodeReportProviderRequiresExplicitEndpoint(t *testing.T) {
	provider := NewMissingCodeReportProvider(&conf.SmsVerifyCodeConfig{SendCodeUrl: "http://sms.example.test/send"})
	if !errors.Is(provider.Ready(), ErrProviderUnavailable) {
		t.Fatalf("provider ready = %v, want unavailable", provider.Ready())
	}
}

func TestHTTPProviderFailsClosedForMissingInvalidAndProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config *conf.SmsVerifyCodeConfig
		status int
		want   error
	}{
		{name: "missing endpoint", config: &conf.SmsVerifyCodeConfig{EmailProvider: "http"}, want: ErrProviderUnavailable},
		{name: "invalid scheme", config: &conf.SmsVerifyCodeConfig{EmailProvider: "http", EmailSendCodeUrl: "ftp://mail.example.test/send"}, want: ErrProviderUnavailable},
		{name: "provider status", status: http.StatusBadGateway, want: ErrDeliveryFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := tc.config
			if tc.status != 0 {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
				}))
				defer server.Close()
				config = &conf.SmsVerifyCodeConfig{EmailProvider: "http", EmailSendCodeUrl: server.URL}
			}
			provider := NewEmailProvider(config)
			err := provider.Ready()
			if tc.status != 0 {
				if err != nil {
					t.Fatalf("provider ready: %v", err)
				}
				err = provider.Deliver(context.Background(), Delivery{Channel: ChannelEmail, Destination: "person@example.com", Code: "123456"})
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestHTTPProviderRetriesServerFailures(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	provider := NewEmailProvider(&conf.SmsVerifyCodeConfig{
		EmailProvider: "http", EmailSendCodeUrl: server.URL, ProviderRetryCount: 1,
	})
	if err := provider.Deliver(context.Background(), Delivery{Channel: ChannelEmail, Destination: "person@example.com", Code: "123456"}); err != nil {
		t.Fatalf("provider delivery = %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestHTTPProviderRetriesNetworkFailures(t *testing.T) {
	var requests atomic.Int32
	provider := NewEmailProvider(&conf.SmsVerifyCodeConfig{
		// The provider policy permits HTTP only for loopback test endpoints.
		// The custom transport below prevents any network request.
		EmailProvider: "http", EmailSendCodeUrl: "http://127.0.0.1:9/send", ProviderRetryCount: 1,
	})
	httpProvider := provider.(*httpProvider)
	httpProvider.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return nil, errors.New("temporary network failure")
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	if err := provider.Deliver(context.Background(), Delivery{Channel: ChannelEmail, Destination: "person@example.com", Code: "123456"}); err != nil {
		t.Fatalf("provider delivery = %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestHTTPProviderRetryCountIsBounded(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewEmailProvider(&conf.SmsVerifyCodeConfig{
		EmailProvider: "http", EmailSendCodeUrl: server.URL, ProviderRetryCount: 2,
	})
	err := provider.Deliver(context.Background(), Delivery{Channel: ChannelEmail, Destination: "person@example.com", Code: "123456"})
	if !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("provider error = %v, want delivery failure", err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests = %d, want initial request plus two retries", got)
	}
}

func TestHTTPProviderDoesNotRetryClientFailuresOrRedirects(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusFound} {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(status)
		}))
		provider := NewEmailProvider(&conf.SmsVerifyCodeConfig{
			EmailProvider: "http", EmailSendCodeUrl: server.URL, ProviderRetryCount: 3,
		})
		err := provider.Deliver(context.Background(), Delivery{Channel: ChannelEmail, Destination: "person@example.com", Code: "123456"})
		server.Close()
		if !errors.Is(err, ErrDeliveryFailed) {
			t.Fatalf("status %d error = %v, want delivery failure", status, err)
		}
		if got := requests.Load(); got != 1 {
			t.Fatalf("status %d requests = %d, want 1", status, got)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
