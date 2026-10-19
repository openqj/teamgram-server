package http

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/apifull/paymentrecovery"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

const sub2APIStarsUsageMaxBodyBytes = 1 << 20

type sub2APIStarsUsagePayload struct {
	EventID   string      `json:"event_id"`
	AccountID string      `json:"account_id"`
	AmountUSD json.Number `json:"amount_usd"`
	Currency  string      `json:"currency"`
}

type identityTeamgramAccountResponse struct {
	AccountID      string `json:"account_id"`
	InstanceID     string `json:"instance_id"`
	TeamgramUserID int64  `json:"teamgram_user_id"`
}

func sub2APIStarsUsageHandler(serverCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if serverCtx == nil {
			http.Error(w, "Stars usage accounting is unavailable", http.StatusServiceUnavailable)
			return
		}
		serviceToken := strings.TrimSpace(serverCtx.Config.Sub2APIServiceToken)
		if len(serviceToken) < 32 || !constantTimeTokenEqual(r.Header.Get("X-Sub2API-Service-Token"), serviceToken) {
			if len(serviceToken) < 32 {
				http.Error(w, "Stars usage webhook is not configured", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "invalid Sub2API service token", http.StatusUnauthorized)
			return
		}
		if r.Body == nil {
			http.Error(w, "invalid Stars usage event", http.StatusBadRequest)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, sub2APIStarsUsageMaxBodyBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid Stars usage body", http.StatusRequestEntityTooLarge)
			return
		}
		var payload sub2APIStarsUsagePayload
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&payload); err != nil {
			http.Error(w, "invalid Stars usage JSON", http.StatusBadRequest)
			return
		}
		if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			http.Error(w, "invalid Stars usage JSON", http.StatusBadRequest)
			return
		}
		payload.EventID = strings.TrimSpace(payload.EventID)
		payload.AccountID = strings.TrimSpace(payload.AccountID)
		payload.Currency = strings.ToUpper(strings.TrimSpace(payload.Currency))
		amount := strings.TrimSpace(payload.AmountUSD.String())
		if payload.EventID == "" || len(payload.EventID) > 191 || !validSub2APIStarsAccountID(payload.AccountID) ||
			payload.Currency != "USD" || !validSub2APIStarsUSD(amount) {
			http.Error(w, "invalid Stars usage event", http.StatusBadRequest)
			return
		}
		userID, err := resolveSub2APIStarsTeamgramUser(r.Context(), serverCtx, payload.AccountID)
		if err != nil {
			http.Error(w, "Teamgram account lookup is unavailable", http.StatusServiceUnavailable)
			return
		}
		stars, err := paymentrecovery.ApplySub2APIStarsUsage(r.Context(), paymentrecovery.Sub2APIStarsUsageEvent{
			EventID: payload.EventID, AccountID: payload.AccountID, UserID: userID,
			AmountUSD: amount, Currency: payload.Currency,
		})
		if err != nil {
			switch {
			case errors.Is(err, paymentrecovery.ErrStarsBalanceExceeded), errors.Is(err, paymentrecovery.ErrStarsUsageConflict):
				http.Error(w, "Stars usage could not be applied; retry the event", http.StatusConflict)
			case errors.Is(err, paymentrecovery.ErrInvalidWebhook):
				http.Error(w, "invalid Stars usage event", http.StatusBadRequest)
			default:
				http.Error(w, "Stars usage accounting is unavailable", http.StatusServiceUnavailable)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "event_id": payload.EventID, "applied_stars_delta": stars})
	}
}

func resolveSub2APIStarsTeamgramUser(ctx context.Context, serverCtx *svc.ServiceContext, accountID string) (int64, error) {
	if serverCtx == nil {
		return 0, errors.New("HTTP server context is unavailable")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(serverCtx.Config.IdentityBaseURL), "/")
	identityToken := strings.TrimSpace(serverCtx.Config.IdentityTeamgramServiceToken)
	instanceID := strings.TrimSpace(serverCtx.Config.IdentityTeamgramInstanceID)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) || identityToken == "" || instanceID == "" {
		return 0, errors.New("Identity reverse lookup is not configured")
	}
	payload, err := json.Marshal(struct {
		AccountID  string `json:"account_id"`
		InstanceID string `json:"instance_id"`
	}{AccountID: accountID, InstanceID: instanceID})
	if err != nil {
		return 0, err
	}
	timeout := 5 * time.Second
	if serverCtx.Config.IdentityRequestTimeoutSeconds > 0 {
		timeout = time.Duration(serverCtx.Config.IdentityRequestTimeoutSeconds) * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(callCtx, http.MethodPost, baseURL+"/v1/internal/teamgram/account", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Identity-Teamgram-Token", identityToken)
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return 0, errors.New("Identity reverse lookup request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return 0, errors.New("Identity reverse lookup rejected the account")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return 0, err
	}
	var result identityTeamgramAccountResponse
	if json.Unmarshal(body, &result) != nil || result.AccountID != accountID || result.InstanceID != instanceID || result.TeamgramUserID <= 0 {
		return 0, errors.New("Identity reverse lookup response is invalid")
	}
	return result.TeamgramUserID, nil
}

func constantTimeTokenEqual(provided, expected string) bool {
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func validSub2APIStarsAccountID(value string) bool {
	if len(value) != len("acct:00000000-0000-0000-0000-000000000000") || !strings.HasPrefix(value, "acct:") {
		return false
	}
	for index, char := range value[len("acct:"):] {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func validSub2APIStarsUSD(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	unsigned := strings.TrimPrefix(value, "-")
	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 18)) {
		return false
	}
	for _, part := range parts {
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	rat, ok := new(big.Rat).SetString(value)
	return ok && rat.Sign() != 0
}
