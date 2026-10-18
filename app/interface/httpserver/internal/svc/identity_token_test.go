package svc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdentityTokenExchangerUsesBoundAccountEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/internal/teamgram/token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Identity-Teamgram-Token"); got != "teamgram-secret" {
			t.Fatalf("unexpected internal token: %q", got)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if int64(request["teamgram_user_id"].(float64)) != 12345 || request["instance_id"] != "instance-a" {
			t.Fatalf("unexpected user id: %v", request["teamgram_user_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "identity-token",
			"expires_in":   600,
		})
	}))
	defer server.Close()

	exchanger, err := NewIdentityTokenExchanger(server.URL, "teamgram-secret", "instance-a", 2)
	if err != nil {
		t.Fatalf("create exchanger: %v", err)
	}
	accessToken, expiresAt, err := exchanger.Exchange(context.Background(), 12345)
	if err != nil {
		t.Fatalf("exchange token: %v", err)
	}
	if accessToken != "identity-token" {
		t.Fatalf("unexpected access token: %q", accessToken)
	}
	if expiresAt <= time.Now().Unix() {
		t.Fatalf("expected future expiration, got %d", expiresAt)
	}
}

func TestIdentityTokenExchangerFailsClosedForUnboundAccount(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	exchanger, err := NewIdentityTokenExchanger(server.URL, "teamgram-secret", "instance-a", 2)
	if err != nil {
		t.Fatalf("create exchanger: %v", err)
	}
	_, _, err = exchanger.Exchange(context.Background(), 12345)
	if err != ErrIdentityTokenNotFound {
		t.Fatalf("expected unbound account error, got %v", err)
	}
}

func BenchmarkIdentityTokenExchangerExchange(b *testing.B) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"identity-token","expires_in":600}`))
	}))
	defer server.Close()

	exchanger, err := NewIdentityTokenExchanger(server.URL, "teamgram-secret", "instance-a", 2)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, _, err := exchanger.Exchange(context.Background(), 12345); err != nil {
				b.Error(err)
			}
		}
	})
	b.StopTimer()
	b.ReportMetric(float64(requests.Load()), "requests")
}
