package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/providercontract"
)

const businessProviderBodyLimit = 1 << 20

func (c *ApiFullCore) businessProviderConfigured() bool {
	return c != nil && c.svcCtx != nil && strings.TrimSpace(c.svcCtx.Config.BusinessProviderEndpoint) != "" && len(strings.TrimSpace(c.svcCtx.Config.BusinessProviderSigningKey)) >= 32
}

type businessProviderEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result"`
}

func businessProviderSignature(key string, body []byte) string {
	return providercontract.Signature(key, body)
}

func businessProviderIdempotencyKey(userID int64, operation string, payload []byte) string {
	return providercontract.IdempotencyKey(userID, operation, payload)
}

func verifyBusinessProviderSignature(key, signature string, body []byte) bool {
	return providercontract.VerifySignature(key, signature, body)
}

func secureBusinessProviderURL(parsed *url.URL, allowInsecureHTTP bool) bool {
	if parsed == nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	if allowInsecureHTTP {
		return true
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// callBusinessProvider is the common boundary for business capabilities that
// are owned by a separate service. A signed response is required even for
// read-only operations so a stale or unauthenticated response cannot be
// surfaced as a Layer 229 success.
func (c *ApiFullCore) callBusinessProvider(operation string, payload any, result any) error {
	if c == nil || c.svcCtx == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.BusinessProviderEndpoint)
	key := strings.TrimSpace(c.svcCtx.Config.BusinessProviderKey)
	signingKey := strings.TrimSpace(c.svcCtx.Config.BusinessProviderSigningKey)
	if endpoint == "" || len(signingKey) < 32 {
		return mtproto.ErrMethodNotImpl
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureBusinessProviderURL(parsed, c.svcCtx.Config.AllowInsecureProviderHTTP) {
		return mtproto.ErrInternalServerError
	}
	if c.svcCtx.Config.BusinessProviderTimeoutSeconds < 0 || c.svcCtx.Config.BusinessProviderTimeoutSeconds > 300 {
		return mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.BusinessProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.BusinessProviderTimeoutSeconds) * time.Second
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return mtproto.ErrInternalServerError
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return mtproto.ErrInputRequestInvalid
	}
	request := struct {
		RequestID string          `json:"request_id"`
		Operation string          `json:"operation"`
		UserID    int64           `json:"user_id"`
		Payload   json.RawMessage `json:"payload"`
	}{hex.EncodeToString(nonce[:]), operation, c.MD.GetUserId(), payloadJSON}
	body, err := json.Marshal(request)
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	providercontract.SetRequestHeaders(req, body, key,
		businessProviderIdempotencyKey(c.MD.GetUserId(), operation, payloadJSON),
		"X-Teamgram-Business-Provider-Request-Signature", signingKey)
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrMethodNotImpl
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, businessProviderBodyLimit+1))
	if err != nil || len(responseBody) > businessProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrMethodNotImpl
	}
	if !verifyBusinessProviderSignature(signingKey, resp.Header.Get("X-Teamgram-Business-Provider-Signature"), responseBody) {
		return mtproto.ErrInternalServerError
	}
	var envelope businessProviderEnvelope
	if err = json.Unmarshal(responseBody, &envelope); err != nil || !envelope.Verified || envelope.RequestID != request.RequestID || envelope.Operation != request.Operation || envelope.UserID != request.UserID {
		return mtproto.ErrInternalServerError
	}
	if envelope.Error != "" || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return mtproto.ErrMethodNotImpl
	}
	if err = json.Unmarshal(envelope.Result, result); err != nil {
		return mtproto.ErrInternalServerError
	}
	return nil
}
