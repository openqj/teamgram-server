package svc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIdentityTokenExchangerUsesBoundAccountEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/internal/teamgram/token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Identity-Internal-Token"); got != "internal-secret" {
			t.Fatalf("unexpected internal token: %q", got)
		}
		var request map[string]int64
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["teamgram_user_id"] != 12345 {
			t.Fatalf("unexpected user id: %v", request["teamgram_user_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "identity-token",
			"expires_in":   600,
		})
	}))
	defer server.Close()

	exchanger, err := NewIdentityTokenExchanger(server.URL, "internal-secret", 2)
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

	exchanger, err := NewIdentityTokenExchanger(server.URL, "internal-secret", 2)
	if err != nil {
		t.Fatalf("create exchanger: %v", err)
	}
	_, _, err = exchanger.Exchange(context.Background(), 12345)
	if err != ErrIdentityTokenNotFound {
		t.Fatalf("expected unbound account error, got %v", err)
	}
}
