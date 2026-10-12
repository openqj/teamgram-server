package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestCallStarsRevenueProviderAuthenticatesRequest(t *testing.T) {
	const signingKey = "local-payment-provider-signing-key-20261012"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read request", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-provider" ||
			!verifyPaymentProviderSignature(signingKey, r.Header.Get("X-Teamgram-Payment-Provider-Request-Signature"), body) {
			http.Error(w, "request authentication failed", http.StatusUnauthorized)
			return
		}
		var request struct {
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if json.Unmarshal(body, &request) != nil || request.Operation != "get_stars_revenue_stats" || request.UserID != 902101 {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		responseBody := []byte(`{"verified":true,"user_id":902101}`)
		mac := hmac.New(sha256.New, []byte(signingKey))
		_, _ = mac.Write(responseBody)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Teamgram-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	c := &ApiFullCore{svcCtx: &svc.ServiceContext{Config: config.Config{
		PaymentProviderEndpoint:   server.URL,
		PaymentProviderKey:        "fixture-provider",
		PaymentProviderSigningKey: signingKey,
	}}}
	var response struct {
		Verified bool  `json:"verified"`
		UserID   int64 `json:"user_id"`
	}
	if err := c.callStarsRevenueProvider(context.Background(), struct {
		Operation string `json:"operation"`
		UserID    int64  `json:"user_id"`
	}{"get_stars_revenue_stats", 902101}, &response); err != nil {
		t.Fatalf("callStarsRevenueProvider() error = %v", err)
	}
	if !response.Verified || response.UserID != 902101 {
		t.Fatalf("provider response = %+v", response)
	}
}

func TestPaymentProviderRequestSignatureChangesWithBody(t *testing.T) {
	const signingKey = "local-payment-provider-signing-key-20261012"
	one := []byte(`{"operation":"settle_stars","amount":199}`)
	two := []byte(`{"operation":"settle_stars","amount":200}`)
	if bytes.Equal([]byte(paymentProviderRequestSignature(signingKey, one)), []byte(paymentProviderRequestSignature(signingKey, two))) {
		t.Fatal("different request bodies produced the same provider signature")
	}
}
