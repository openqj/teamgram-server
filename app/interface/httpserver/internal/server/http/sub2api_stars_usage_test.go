package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

func TestSub2APIStarsUsageHandlerAuthenticatesBeforeProcessing(t *testing.T) {
	called := false
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer identity.Close()
	serverCtx := &svc.ServiceContext{Config: config.Config{
		Sub2APIServiceToken: "01234567890123456789012345678901",
		IdentityBaseURL:     identity.URL,
	}}
	request := httptest.NewRequest(http.MethodPost, "/v1/internal/sub2api/stars/usage", nil)
	response := httptest.NewRecorder()
	sub2APIStarsUsageHandler(serverCtx)(response, request)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("unauthenticated callback = status %d, identity called %v", response.Code, called)
	}
}

func TestResolveSub2APIStarsTeamgramUserChecksReverseIdentityContract(t *testing.T) {
	const accountID = "acct:550e8400-e29b-41d4-a716-446655440000"
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/internal/teamgram/account" {
			t.Fatalf("Identity request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Identity-Teamgram-Token") != "identity-secret" {
			t.Fatalf("Identity token = %q", r.Header.Get("X-Identity-Teamgram-Token"))
		}
		var request struct {
			AccountID  string `json:"account_id"`
			InstanceID string `json:"instance_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode Identity request: %v", err)
		}
		if request.AccountID != accountID || request.InstanceID != "instance-a" {
			t.Fatalf("Identity request = %+v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"account_id": accountID, "instance_id": "instance-a", "teamgram_user_id": int64(42),
		})
	}))
	defer identity.Close()

	serverCtx := &svc.ServiceContext{Config: config.Config{
		IdentityBaseURL:              identity.URL,
		IdentityTeamgramServiceToken: "identity-secret",
		IdentityTeamgramInstanceID:   "instance-a",
	}}
	userID, err := resolveSub2APIStarsTeamgramUser(context.Background(), serverCtx, accountID)
	if err != nil || userID != 42 {
		t.Fatalf("reverse lookup = (%d, %v), want (42, nil)", userID, err)
	}
}

func TestSub2APIStarsUsageValidationRejectsNonCanonicalAccountAndAmount(t *testing.T) {
	for _, accountID := range []string{"", "acct:550E8400-e29b-41d4-a716-446655440000", "acct:550e8400-e29b-41d4-a716-44665544000z"} {
		if validSub2APIStarsAccountID(accountID) {
			t.Errorf("account id %q unexpectedly accepted", accountID)
		}
	}
	for _, amount := range []string{"", "0", "-0", "1e-3", "1.1234567890123456789", "NaN"} {
		if validSub2APIStarsUSD(amount) {
			t.Errorf("amount %q unexpectedly accepted", amount)
		}
	}
	if !validSub2APIStarsUSD("0.000000000000000001") {
		t.Fatal("small positive USD decimal was rejected")
	}
	if !validSub2APIStarsUSD("-0.000000000000000001") {
		t.Fatal("small negative USD delta was rejected")
	}
}
