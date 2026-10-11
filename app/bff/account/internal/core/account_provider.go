package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const botConnectionProviderBodyLimit = 1 << 20

type botConnectionProviderEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result"`
}

type botConnectionProviderResult struct {
	Confirmed bool          `json:"confirmed"`
	Bool      *mtproto.Bool `json:"bool,omitempty"`
}

func botConnectionProviderConfigured(c *AccountCore) bool {
	return c != nil && c.svcCtx != nil && strings.TrimSpace(c.svcCtx.Config.BotConnectionProviderEndpoint) != "" && len(strings.TrimSpace(c.svcCtx.Config.BotConnectionProviderSigningKey)) >= 32
}

func botConnectionProviderSignature(key string, body []byte) string {
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func verifyBotConnectionProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write(body)
	return hmac.Equal(provided, h.Sum(nil))
}

func secureBotConnectionProviderURL(parsed *url.URL) bool {
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

func (c *AccountCore) callBotConnectionProvider(operation string, payload any, result any) error {
	if c == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	if !botConnectionProviderConfigured(c) {
		return mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.BotConnectionProviderEndpoint)
	key := strings.TrimSpace(c.svcCtx.Config.BotConnectionProviderKey)
	signingKey := strings.TrimSpace(c.svcCtx.Config.BotConnectionProviderSigningKey)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureBotConnectionProviderURL(parsed) {
		return mtproto.ErrInternalServerError
	}
	if c.svcCtx.Config.BotConnectionProviderTimeoutSeconds < 0 || c.svcCtx.Config.BotConnectionProviderTimeoutSeconds > 300 {
		return mtproto.ErrInternalServerError
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return mtproto.ErrInputRequestInvalid
	}
	requestIDBytes := sha256.Sum256(append([]byte(operation+":"+strconv.FormatInt(c.MD.GetUserId(), 10)+":"), payloadJSON...))
	request := struct {
		RequestID string          `json:"request_id"`
		Operation string          `json:"operation"`
		UserID    int64           `json:"user_id"`
		Payload   json.RawMessage `json:"payload"`
	}{hex.EncodeToString(requestIDBytes[:]), operation, c.MD.GetUserId(), payloadJSON}
	body, err := json.Marshal(request)
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.BotConnectionProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.BotConnectionProviderTimeoutSeconds) * time.Second
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Idempotency-Key", request.RequestID)
	req.Header.Set("X-Teamgram-Bot-Connection-Provider-Request-Signature", botConnectionProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrMethodNotImpl
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, botConnectionProviderBodyLimit+1))
	if err != nil || len(responseBody) > botConnectionProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrMethodNotImpl
	}
	if !verifyBotConnectionProviderSignature(signingKey, resp.Header.Get("X-Teamgram-Bot-Connection-Provider-Signature"), responseBody) {
		return mtproto.ErrInternalServerError
	}
	var envelope botConnectionProviderEnvelope
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
