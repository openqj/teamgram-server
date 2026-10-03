package me

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

func TestSendSmsVerifyCodeEncodesQueryAndReturnsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("phone"); got != "+1 555&123" {
			t.Errorf("phone query = %q, want %q", got, "+1 555&123")
		}
		if got := r.URL.Query().Get("code"); got != "12 3?" {
			t.Errorf("code query = %q, want %q", got, "12 3?")
		}
		if got := r.URL.Query().Get("existing"); got != "keep" {
			t.Errorf("existing query = %q, want keep", got)
		}
		_, _ = w.Write([]byte(" accepted \n"))
	}))
	defer server.Close()

	provider := New(&conf.SmsVerifyCodeConfig{SendCodeUrl: server.URL + "/code?existing=keep"})
	got, err := provider.SendSmsVerifyCode(context.Background(), "+1 555&123", "12 3?", "hash")
	if err != nil {
		t.Fatalf("SendSmsVerifyCode() error = %v", err)
	}
	if got != "accepted" {
		t.Fatalf("SendSmsVerifyCode() = %q, want accepted", got)
	}
}

func TestSendSmsVerifyCodeFailsClosed(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	tests := []struct {
		name       string
		endpoint   string
		statusCode int
		body       string
		want       error
	}{
		{name: "missing config", want: ErrProviderUnavailable},
		{name: "network error", endpoint: closedURL, want: ErrDeliveryFailed},
		{name: "provider error", statusCode: http.StatusBadGateway, body: "failed", want: ErrDeliveryFailed},
		{name: "empty response", statusCode: http.StatusNoContent, want: ErrDeliveryFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoint := test.endpoint
			if endpoint == "" && test.name != "missing config" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(test.statusCode)
					_, _ = w.Write([]byte(test.body))
				}))
				defer server.Close()
				endpoint = server.URL
			}

			var provider *meVerifyCode
			if test.name == "missing config" {
				provider = New(nil)
			} else {
				provider = New(&conf.SmsVerifyCodeConfig{SendCodeUrl: endpoint})
			}
			if _, err := provider.SendSmsVerifyCode(context.Background(), "phone", "12345", "hash"); !errors.Is(err, test.want) {
				t.Fatalf("SendSmsVerifyCode() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSendSmsVerifyCodeHonorsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := New(&conf.SmsVerifyCodeConfig{SendCodeUrl: server.URL})
	if _, err := provider.SendSmsVerifyCode(ctx, "phone", "12345", "hash"); !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("SendSmsVerifyCode() error = %v, want delivery failure", err)
	}
}
