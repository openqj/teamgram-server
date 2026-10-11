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
)

const passportProviderBodyLimit = 1 << 20

type passportAuthorizationProviderRequest struct {
	RequestID   string                              `json:"request_id"`
	Operation   string                              `json:"operation"`
	UserID      int64                               `json:"user_id"`
	BotID       int64                               `json:"bot_id"`
	Scope       string                              `json:"scope"`
	PublicKey   string                              `json:"public_key"`
	ValueHashes []*mtproto.SecureValueHash          `json:"value_hashes"`
	Credentials *mtproto.SecureCredentialsEncrypted `json:"credentials"`
}

type passportAuthorizationProviderResponse struct {
	RequestID string `json:"request_id"`
	Operation string `json:"operation"`
	Verified  bool   `json:"verified"`
	UserID    int64  `json:"user_id"`
	BotID     int64  `json:"bot_id"`
	Scope     string `json:"scope"`
	PublicKey string `json:"public_key"`
	Result    *bool  `json:"result"`
}

func passportProviderSignature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyPassportProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func securePassportProviderURL(parsed *url.URL) bool {
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

func (c *PassportCore) callPassportAuthorizationProvider(ctx context.Context, payload passportAuthorizationProviderRequest) (passportAuthorizationProviderResponse, error) {
	var result passportAuthorizationProviderResponse
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.svcCtx == nil {
		return result, mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.PassportProviderEndpoint)
	key := strings.TrimSpace(c.svcCtx.Config.PassportProviderKey)
	signingKey := strings.TrimSpace(c.svcCtx.Config.PassportProviderSigningKey)
	if endpoint == "" || len(signingKey) < 32 {
		return result, mtproto.ErrMethodNotImpl
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !securePassportProviderURL(parsed) {
		return result, mtproto.ErrInternalServerError
	}
	if c.svcCtx.Config.PassportProviderTimeoutSeconds < 0 || c.svcCtx.Config.PassportProviderTimeoutSeconds > 300 {
		return result, mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.PassportProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.PassportProviderTimeoutSeconds) * time.Second
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return result, mtproto.ErrInternalServerError
	}
	payload.RequestID = hex.EncodeToString(nonce[:])
	payload.Operation = "account.acceptAuthorization"
	body, err := json.Marshal(payload)
	if err != nil {
		return result, mtproto.ErrInputRequestInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result, mtproto.ErrInternalServerError
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("X-Teamgram-Passport-Provider-Request-Signature", passportProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return result, mtproto.ErrMethodNotImpl
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, passportProviderBodyLimit+1))
	if err != nil || len(responseBody) > passportProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, mtproto.ErrMethodNotImpl
	}
	if !verifyPassportProviderSignature(signingKey, resp.Header.Get("X-Teamgram-Passport-Provider-Signature"), responseBody) {
		return result, mtproto.ErrInternalServerError
	}
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.RequestID != payload.RequestID || result.Operation != payload.Operation || result.UserID != payload.UserID || result.BotID != payload.BotID || result.Scope != payload.Scope || result.PublicKey != payload.PublicKey || result.Result == nil {
		return passportAuthorizationProviderResponse{}, mtproto.ErrInternalServerError
	}
	return result, nil
}
