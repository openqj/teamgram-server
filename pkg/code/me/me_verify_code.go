// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package me

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

var (
	_smsURL                = "http://127.0.0.1:8181/code?phone=%s&code=%s"
	ErrProviderUnavailable = errors.New("verification-code provider unavailable")
	ErrDeliveryFailed      = errors.New("verification-code delivery failed")
)

func New(c *conf.SmsVerifyCodeConfig) *meVerifyCode {
	timeout := 5 * time.Second
	if c != nil && c.ProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.ProviderTimeoutSeconds) * time.Second
	}
	return &meVerifyCode{
		code: c,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

type meVerifyCode struct {
	code   *conf.SmsVerifyCodeConfig
	client *http.Client
}

func (m *meVerifyCode) SendSmsVerifyCode(ctx context.Context, phoneNumber, code, codeHash string) (string, error) {
	if m == nil || m.code == nil || strings.TrimSpace(m.code.SendCodeUrl) == "" {
		return "", ErrProviderUnavailable
	}

	endpoint, err := url.Parse(strings.TrimSpace(m.code.SendCodeUrl))
	if err != nil || endpoint.Host == "" || endpoint.User != nil || !secureEndpoint(endpoint) || !loopbackEndpoint(endpoint) {
		return "", fmt.Errorf("%w: invalid endpoint", ErrDeliveryFailed)
	}
	query := endpoint.Query()
	query.Set("phone", phoneNumber)
	query.Set("code", code)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("%w: create request: %v", ErrDeliveryFailed, err)
	}
	client := m.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDeliveryFailed, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("%w: read response: %v", ErrDeliveryFailed, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%w: provider returned HTTP %d", ErrDeliveryFailed, resp.StatusCode)
	}
	result := strings.TrimSpace(hack.String(body))
	if result == "" {
		return "", fmt.Errorf("%w: provider returned an empty response", ErrDeliveryFailed)
	}
	return result, nil
}

func secureEndpoint(endpoint *url.URL) bool {
	if endpoint == nil {
		return false
	}
	if strings.EqualFold(endpoint.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(endpoint.Scheme, "http") {
		return false
	}
	return strings.EqualFold(endpoint.Scheme, "http") && loopbackEndpoint(endpoint)
}

func loopbackEndpoint(endpoint *url.URL) bool {
	if endpoint == nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(endpoint.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (m *meVerifyCode) VerifySmsCode(ctx context.Context, codeHash, code, extraData string) error {
	if len(code) != 5 {
		return fmt.Errorf("code invalid")
	}

	//
	if code != extraData {
		return fmt.Errorf("code invalid")
	}

	// ...
	return nil
}
