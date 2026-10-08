// Copyright 2026 Teamgram Authors
// All rights reserved.
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

package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const authProviderBodyLimit = 1 << 20

type authProviderRequest struct {
	RequestID    string `json:"request_id"`
	Operation    string `json:"operation"`
	APIID        int32  `json:"api_id"`
	APIHash      string `json:"api_hash"`
	BotAuthToken string `json:"bot_auth_token,omitempty"`
	WebAuthToken string `json:"web_auth_token,omitempty"`
}

type authProviderResponse struct {
	RequestID string `json:"request_id"`
	Verified  bool   `json:"verified"`
	Operation string `json:"operation"`
	APIID     int32  `json:"api_id"`
	UserID    int64  `json:"user_id"`
}

func (c *AuthorizationCore) authProvider(ctx context.Context, request authProviderRequest) (authProviderResponse, error) {
	var result authProviderResponse
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.svcCtx == nil {
		return result, mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.AuthProviderEndpoint)
	if endpoint == "" {
		return result, mtproto.ErrMethodNotImpl
	}
	if strings.TrimSpace(c.svcCtx.Config.AuthProviderKey) == "" || len(c.svcCtx.Config.AuthProviderSigningKey) < 32 {
		return result, mtproto.ErrMethodNotImpl
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureAuthProviderURL(parsed) {
		return result, mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.AuthProviderTimeoutSeconds < 0 || c.svcCtx.Config.AuthProviderTimeoutSeconds > 300 {
		return result, mtproto.ErrInternalServerError
	}
	if c.svcCtx.Config.AuthProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.AuthProviderTimeoutSeconds) * time.Second
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return result, mtproto.ErrInternalServerError
	}
	request.RequestID = hex.EncodeToString(nonce[:])
	body, err := json.Marshal(request)
	if err != nil {
		return result, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result, mtproto.ErrInternalServerError
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.svcCtx.Config.AuthProviderKey)
	req.Header.Set("X-Teamgram-Auth-Provider-Request-Signature", authProviderSignature(c.svcCtx.Config.AuthProviderSigningKey, body))
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, mtproto.ErrInternalServerError
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, authProviderBodyLimit+1))
	if err != nil || len(responseBody) > authProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, mtproto.ErrInternalServerError
	}
	if !verifyAuthProviderSignature(c.svcCtx.Config.AuthProviderSigningKey, resp.Header.Get("X-Teamgram-Auth-Provider-Signature"), responseBody) {
		return result, mtproto.ErrInternalServerError
	}
	if err = json.Unmarshal(responseBody, &result); err != nil || result.RequestID != request.RequestID || !result.Verified || result.Operation != request.Operation || result.APIID != request.APIID || result.UserID <= 0 {
		return authProviderResponse{}, mtproto.ErrAccessTokenInvalid
	}
	return result, nil
}

func secureAuthProviderURL(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func authProviderSignature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyAuthProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || key == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func (c *AuthorizationCore) bindProviderUser(user *mtproto.ImmutableUser) (*mtproto.Auth_Authorization, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.AuthsessionClient == nil || c.MD == nil || c.MD.GetPermAuthKeyId() == 0 || user == nil || user.GetUser() == nil || user.Deleted() {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	bindHash, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionBindAuthKeyUser(c.ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: c.MD.GetPermAuthKeyId(),
		UserId:    user.GetUser().GetId(),
	})
	if err != nil {
		return nil, err
	}
	if bindHash == nil || bindHash.GetV() == 0 {
		return nil, mtproto.ErrInternalServerError
	}
	return authAuthorization(user), nil
}

func (c *AuthorizationCore) loadBotByProviderToken(token string, expectedID int64) (*mtproto.ImmutableUser, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	user, err := c.svcCtx.Dao.UserGetImmutableUserByToken(c.ctx, &userpb.TLUserGetImmutableUserByToken{Token: token})
	if err != nil {
		return nil, err
	}
	if user == nil || user.GetUser() == nil || user.Deleted() || !user.IsBot() || (expectedID > 0 && user.GetUser().GetId() != expectedID) {
		return nil, mtproto.ErrAccessTokenInvalid
	}
	return user, nil
}
