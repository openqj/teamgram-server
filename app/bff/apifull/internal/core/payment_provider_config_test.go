package core

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestConfiguredPaymentProviderRejectsUnsafeEndpointAndTimeout(t *testing.T) {
	server := httptest.NewServer(nil)
	t.Cleanup(server.Close)

	for name, cfg := range map[string]config.Config{
		"missing signing key": {
			PaymentProviderEndpoint: server.URL,
		},
		"query": {
			PaymentProviderEndpoint:   server.URL + "?route=settle",
			PaymentProviderSigningKey: paymentProviderTestSigningKey,
		},
		"fragment": {
			PaymentProviderEndpoint:   server.URL + "#settle",
			PaymentProviderSigningKey: paymentProviderTestSigningKey,
		},
		"negative timeout": {
			PaymentProviderEndpoint:       server.URL,
			PaymentProviderSigningKey:     paymentProviderTestSigningKey,
			PaymentProviderTimeoutSeconds: -1,
		},
		"excessive timeout": {
			PaymentProviderEndpoint:       server.URL,
			PaymentProviderSigningKey:     paymentProviderTestSigningKey,
			PaymentProviderTimeoutSeconds: 301,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := &ApiFullCore{svcCtx: &svc.ServiceContext{Config: cfg}}
			if _, _, _, err := c.configuredPaymentProvider(); !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
				t.Fatalf("configuredPaymentProvider() error = %v, want ErrPaymentProviderInvalid", err)
			}
		})
	}
}

func TestConfiguredPaymentProviderAllowsFixtureHTTPOnlyWhenExplicitlyEnabled(t *testing.T) {
	for name, cfg := range map[string]config.Config{
		"production": {
			PaymentProviderEndpoint:   "http://payment-provider-fixture:18080",
			PaymentProviderSigningKey: paymentProviderTestSigningKey,
		},
		"fixture": {
			PaymentProviderEndpoint:   "http://payment-provider-fixture:18080",
			PaymentProviderSigningKey: paymentProviderTestSigningKey,
			AllowInsecureProviderHTTP: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := &ApiFullCore{svcCtx: &svc.ServiceContext{Config: cfg}}
			_, _, _, err := c.configuredPaymentProvider()
			if name == "production" {
				if !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
					t.Fatalf("configuredPaymentProvider() error = %v, want ErrPaymentProviderInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("configuredPaymentProvider() error = %v", err)
			}
		})
	}
}
