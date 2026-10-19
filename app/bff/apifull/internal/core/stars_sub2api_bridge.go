// Copyright 2026 Teamgram Authors
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

const starsSub2APIMaxResponseBytes = 1 << 20

type starsSub2APIConfig struct {
	identityEndpoint string
	identityToken    string
	instanceID       string
	balanceBaseURL   string
	serviceToken     string
	timeout          time.Duration
}

type identityTeamgramTokenResponse struct {
	AccountID string `json:"account_id"`
}

type sub2APIBalanceResponse struct {
	AccountID     string  `json:"account_id"`
	Sub2APIUserID int64   `json:"sub2api_user_id"`
	Balance       float64 `json:"balance"`
	Currency      string  `json:"currency"`
}

type sub2APIResponseEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// RunStarsSub2APIMirror retries durable signed ledger deltas. Every event is
// processed independently so one unavailable account cannot hold the batch.
func RunStarsSub2APIMirror(ctx context.Context, svcCtx *svc.ServiceContext) {
	interval := 30 * time.Second
	batchSize := 50
	if svcCtx != nil {
		if svcCtx.Config.PaymentReconciliationIntervalSeconds > 0 {
			interval = time.Duration(svcCtx.Config.PaymentReconciliationIntervalSeconds) * time.Second
		}
		if svcCtx.Config.PaymentReconciliationBatchSize > 0 {
			batchSize = svcCtx.Config.PaymentReconciliationBatchSize
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var unavailable bool
	for {
		err := processStarsSub2APIMirror(ctx, svcCtx, batchSize)
		if err != nil && !unavailable {
			logx.WithContext(ctx).Errorf("Stars Sub2API mirror unavailable: %v", err)
			unavailable = true
		} else if err == nil && unavailable {
			logx.WithContext(ctx).Info("Stars Sub2API mirror available")
			unavailable = false
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processStarsSub2APIMirror(ctx context.Context, svcCtx *svc.ServiceContext, batchSize int) error {
	if svcCtx == nil {
		return nil
	}
	cfg, enabled, err := parseStarsSub2APIConfig(svcCtx.Config)
	if err != nil || !enabled {
		return err
	}
	events, err := domain.ClaimStarsMirrorOutbox(ctx, batchSize)
	if err != nil {
		return err
	}
	for _, event := range events {
		callCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
		deliveryErr := deliverStarsSub2APIMirror(callCtx, cfg, event)
		cancel()
		if deliveryErr != nil {
			logx.WithContext(ctx).Errorf("Stars mirror event %s failed: %v", event.EventID, deliveryErr)
			retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
			if retryErr := domain.RetryStarsMirrorOutbox(retryCtx, event, deliveryErr); retryErr != nil {
				logx.WithContext(ctx).Errorf("Stars mirror event %s retry scheduling failed: %v", event.EventID, retryErr)
			}
			retryCancel()
			continue
		}
		completeCtx, completeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if completeErr := domain.CompleteStarsMirrorOutbox(completeCtx, event.EventID, event.ClaimToken); completeErr != nil {
			logx.WithContext(ctx).Errorf("Stars mirror event %s completion failed: %v", event.EventID, completeErr)
		}
		completeCancel()
	}
	return nil
}

func deliverStarsSub2APIMirror(ctx context.Context, cfg starsSub2APIConfig, event domain.StarsMirrorOutboxEvent) error {
	amountValue, amountOK := new(big.Rat).SetString(strings.TrimSpace(event.AmountUSD))
	if event.EventID == "" || event.UserID <= 0 || event.DeltaStars == 0 || !amountOK || amountValue.Sign() <= 0 {
		return errors.New("invalid Stars mirror event or exchange rate")
	}
	accountID, err := resolveStarsIdentityAccount(ctx, cfg, event.UserID)
	if err != nil {
		return err
	}
	operation := "add"
	if event.DeltaStars < 0 {
		operation = "subtract"
	}
	body, err := json.Marshal(struct {
		Amount    json.RawMessage `json:"amount"`
		Operation string          `json:"operation"`
		Notes     string          `json:"notes"`
	}{json.RawMessage(event.AmountUSD), operation, "Teamgram Stars mirror"})
	if err != nil {
		return err
	}
	endpoint := cfg.balanceBaseURL + "/api/v1/internal/aimemory/accounts/" + url.PathEscape(accountID) + "/balance"
	responseBody, err := callStarsSub2API(ctx, cfg, http.MethodPost, endpoint, body, map[string]string{
		"X-AIMemory-Service-Token":      cfg.serviceToken,
		"Idempotency-Key":               event.EventID,
		"X-Balance-Mirror-Origin-Event": event.EventID,
	})
	if err != nil {
		return err
	}
	var envelope sub2APIResponseEnvelope
	if json.Unmarshal(responseBody, &envelope) != nil || envelope.Code != 0 || len(envelope.Data) == 0 {
		return errors.New("Sub2API balance response is invalid")
	}
	var response sub2APIBalanceResponse
	if json.Unmarshal(envelope.Data, &response) != nil ||
		response.AccountID != accountID || response.Sub2APIUserID <= 0 ||
		response.Currency != "USD" {
		return errors.New("Sub2API balance response is invalid")
	}
	return nil
}

func parseStarsSub2APIConfig(raw config.Config) (starsSub2APIConfig, bool, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(raw.Sub2APIInternalBaseURL), "/")
	serviceToken := strings.TrimSpace(raw.Sub2APIServiceToken)
	identityBaseURL := strings.TrimRight(strings.TrimSpace(raw.IdentityBaseURL), "/")
	identityToken := strings.TrimSpace(raw.IdentityTeamgramServiceToken)
	instanceID := strings.TrimSpace(raw.IdentityTeamgramInstanceID)
	configured := baseURL != "" || serviceToken != ""
	if !configured {
		return starsSub2APIConfig{}, false, nil
	}
	if baseURL == "" || len(serviceToken) < 32 || identityBaseURL == "" || identityToken == "" || instanceID == "" {
		return starsSub2APIConfig{}, true, errors.New("Stars Sub2API mirror configuration is incomplete")
	}
	if !validStarsSub2APIURL(baseURL) || !validStarsSub2APIURL(identityBaseURL) {
		return starsSub2APIConfig{}, true, errors.New("Stars Sub2API mirror URL is invalid")
	}
	if raw.Sub2APITimeoutSeconds < 0 || raw.Sub2APITimeoutSeconds > 300 {
		return starsSub2APIConfig{}, true, errors.New("Sub2API timeout must be between 0 and 300 seconds")
	}
	timeout := 5 * time.Second
	if raw.Sub2APITimeoutSeconds > 0 {
		timeout = time.Duration(raw.Sub2APITimeoutSeconds) * time.Second
	}
	return starsSub2APIConfig{
		identityEndpoint: identityBaseURL + "/v1/internal/teamgram/token",
		identityToken:    identityToken,
		instanceID:       instanceID,
		balanceBaseURL:   baseURL,
		serviceToken:     serviceToken,
		timeout:          timeout,
	}, true, nil
}

func resolveStarsIdentityAccount(ctx context.Context, cfg starsSub2APIConfig, userID int64) (string, error) {
	payload, err := json.Marshal(struct {
		TeamgramUserID int64  `json:"teamgram_user_id"`
		InstanceID     string `json:"instance_id"`
	}{TeamgramUserID: userID, InstanceID: cfg.instanceID})
	if err != nil {
		return "", err
	}
	responseBody, err := callStarsSub2API(ctx, cfg, http.MethodPost, cfg.identityEndpoint, payload, map[string]string{
		"X-Identity-Teamgram-Token": cfg.identityToken,
	})
	if err != nil {
		return "", err
	}
	var response identityTeamgramTokenResponse
	if json.Unmarshal(responseBody, &response) != nil || !canonicalStarsIdentityAccountID(response.AccountID) {
		return "", errors.New("Identity Teamgram account response is invalid")
	}
	return response.AccountID, nil
}

func callStarsSub2API(ctx context.Context, cfg starsSub2APIConfig, method, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid Stars mirror request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	client := &http.Client{
		Timeout: cfg.timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("Stars mirror request failed")
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, starsSub2APIMaxResponseBytes+1))
	if readErr != nil || len(data) > starsSub2APIMaxResponseBytes {
		return nil, errors.New("Stars mirror response could not be read")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.New("Stars mirror endpoint rejected the event")
	}
	return data, nil
}

func validStarsSub2APIURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" &&
		(strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
}

func canonicalStarsIdentityAccountID(raw string) bool {
	if len(raw) != len("acct:00000000-0000-0000-0000-000000000000") || !strings.HasPrefix(raw, "acct:") {
		return false
	}
	value := raw[len("acct:"):]
	for index, char := range value {
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
