package svc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrIdentityTokenNotFound = errors.New("Teamgram account is not bound to a product account")
	ErrIdentityTokenService  = errors.New("Identity token exchange is unavailable")
)

type AIMemoryTokenExchanger interface {
	Exchange(context.Context, int64) (string, int64, error)
}

type identityTokenExchanger struct {
	endpoint string
	token    string
	client   *http.Client
}

type identityTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
	ExpiresIn   int64  `json:"expires_in"`
}

func NewIdentityTokenExchanger(baseURL, serviceToken string, timeoutSeconds int) (AIMemoryTokenExchanger, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	serviceToken = strings.TrimSpace(serviceToken)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("Identity base URL is invalid")
	}
	if serviceToken == "" {
		return nil, fmt.Errorf("Identity service token is required")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	return &identityTokenExchanger{
		endpoint: baseURL + "/v1/internal/teamgram/token",
		token:    serviceToken,
		client:   &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}, nil
}

func (e *identityTokenExchanger) Exchange(ctx context.Context, userID int64) (string, int64, error) {
	if userID <= 0 {
		return "", 0, ErrIdentityTokenNotFound
	}
	body, err := json.Marshal(map[string]int64{"teamgram_user_id": userID})
	if err != nil {
		return "", 0, fmt.Errorf("marshal Identity token request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("create Identity token request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Identity-Internal-Token", e.token)
	response, err := e.client.Do(request)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %v", ErrIdentityTokenService, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", 0, fmt.Errorf("%w: read response: %v", ErrIdentityTokenService, err)
	}
	if response.StatusCode == http.StatusNotFound {
		return "", 0, ErrIdentityTokenNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", 0, fmt.Errorf("%w: Identity returned HTTP %d", ErrIdentityTokenService, response.StatusCode)
	}
	var tokenResponse identityTokenResponse
	if err := json.Unmarshal(data, &tokenResponse); err != nil || strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return "", 0, fmt.Errorf("%w: invalid Identity token response", ErrIdentityTokenService)
	}
	if tokenResponse.ExpiresAt <= 0 && tokenResponse.ExpiresIn > 0 {
		tokenResponse.ExpiresAt = time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second).Unix()
	}
	if tokenResponse.ExpiresAt <= time.Now().Unix() {
		return "", 0, fmt.Errorf("%w: Identity token is already expired", ErrIdentityTokenService)
	}
	return tokenResponse.AccessToken, tokenResponse.ExpiresAt, nil
}
