package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestDeliverStarsSub2APIMirrorUsesIdentityAndBalanceOriginKey(t *testing.T) {
	const accountID = "acct:550e8400-e29b-41d4-a716-446655440000"
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/internal/teamgram/token" {
			t.Fatalf("identity request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Identity-Teamgram-Token") != "identity-secret" {
			t.Fatalf("identity token = %q", r.Header.Get("X-Identity-Teamgram-Token"))
		}
		var request struct {
			TeamgramUserID int64  `json:"teamgram_user_id"`
			InstanceID     string `json:"instance_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode identity request: %v", err)
		}
		if request.TeamgramUserID != 42 || request.InstanceID != "default" {
			t.Fatalf("identity request = %+v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"account_id": accountID})
	}))
	defer identity.Close()

	var operations []string
	balance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/balance") {
			t.Fatalf("balance request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-AIMemory-Service-Token") != strings.Repeat("s", 32) {
			t.Fatalf("balance service token = %q", r.Header.Get("X-AIMemory-Service-Token"))
		}
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey == "" || r.Header.Get("X-Balance-Mirror-Origin-Event") != idempotencyKey {
			t.Fatalf("mirror event headers = idempotency:%q origin:%q", idempotencyKey, r.Header.Get("X-Balance-Mirror-Origin-Event"))
		}
		var request struct {
			Amount    json.Number `json:"amount"`
			Operation string      `json:"operation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode balance request: %v", err)
		}
		operations = append(operations, request.Operation+":"+request.Amount.String())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data": map[string]any{
				"account_id": accountID, "sub2api_user_id": 9001, "balance": -0.5, "currency": "USD",
			},
		})
	}))
	defer balance.Close()

	cfg, enabled, err := parseStarsSub2APIConfig(config.Config{
		Sub2APIInternalBaseURL:       balance.URL,
		Sub2APIServiceToken:          strings.Repeat("s", 32),
		Sub2APITimeoutSeconds:        2,
		IdentityBaseURL:              identity.URL,
		IdentityTeamgramServiceToken: "identity-secret",
		IdentityTeamgramInstanceID:   "default",
	})
	if err != nil || !enabled {
		t.Fatalf("parse mirror config = enabled:%v err:%v", enabled, err)
	}
	for _, event := range []domain.StarsMirrorOutboxEvent{
		{EventID: "event-credit", UserID: 42, DeltaStars: 25, AmountUSD: "3.75"},
		{EventID: "event-debit", UserID: 42, DeltaStars: -4, AmountUSD: "0.6"},
	} {
		if err = deliverStarsSub2APIMirror(context.Background(), cfg, event); err != nil {
			t.Fatalf("deliver %s: %v", event.EventID, err)
		}
	}
	if got, want := strings.Join(operations, ","), "add:3.75,subtract:0.6"; got != want {
		t.Fatalf("balance operations = %q, want %q", got, want)
	}
}

func TestStarsSub2APIConfigRequiresCredentialsAndUsesFixedRate(t *testing.T) {
	if _, enabled, err := parseStarsSub2APIConfig(config.Config{}); enabled || err != nil {
		t.Fatalf("empty config = enabled:%v err:%v, want disabled", enabled, err)
	}
	identityOnly := config.Config{IdentityBaseURL: "https://identity.example", IdentityTeamgramServiceToken: "identity-token", IdentityTeamgramInstanceID: "default"}
	if _, enabled, err := parseStarsSub2APIConfig(identityOnly); enabled || err != nil {
		t.Fatalf("identity-only config = enabled:%v err:%v, want the optional Sub2API mirror disabled", enabled, err)
	}
	partial := config.Config{Sub2APIInternalBaseURL: "http://sub2api.test"}
	if _, enabled, err := parseStarsSub2APIConfig(partial); !enabled || err == nil {
		t.Fatalf("partial config = enabled:%v err:%v, want an error", enabled, err)
	}
	partial.Sub2APIServiceToken = strings.Repeat("s", 32)
	partial.IdentityBaseURL = "http://identity.test"
	partial.IdentityTeamgramServiceToken = "identity-token"
	partial.IdentityTeamgramInstanceID = "default"
	if _, enabled, err := parseStarsSub2APIConfig(partial); !enabled || err != nil {
		t.Fatalf("complete config without a runtime rate = enabled:%v err:%v, want fixed-rate mirror enabled", enabled, err)
	}
}
