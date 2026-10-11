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

const cdnProviderBodyLimit = 1 << 20

type cdnProviderEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result"`
}

func cdnProviderSignature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyCDNProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func secureCDNProviderURL(parsed *url.URL) bool {
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

func (c *FilesCore) callCDNProvider(operation string, payload any, result any) error {
	if c == nil || c.svcCtx == nil {
		return mtproto.ErrCdnMethodInvalid
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.CdnProviderEndpoint)
	signingKey := strings.TrimSpace(c.svcCtx.Config.CdnProviderSigningKey)
	if endpoint == "" || len(signingKey) < 32 {
		return mtproto.ErrCdnMethodInvalid
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureCDNProviderURL(parsed) {
		return mtproto.ErrCdnMethodInvalid
	}
	if c.svcCtx.Config.CdnProviderTimeoutSeconds < 0 || c.svcCtx.Config.CdnProviderTimeoutSeconds > 300 {
		return mtproto.ErrCdnMethodInvalid
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.CdnProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.CdnProviderTimeoutSeconds) * time.Second
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	userID := int64(0)
	if c.MD != nil {
		userID = c.MD.GetUserId()
	}
	request := struct {
		RequestID string          `json:"request_id"`
		Operation string          `json:"operation"`
		UserID    int64           `json:"user_id"`
		Payload   json.RawMessage `json:"payload"`
	}{hex.EncodeToString(nonce[:]), operation, userID, payloadJSON}
	body, err := json.Marshal(request)
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(c.svcCtx.Config.CdnProviderKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("X-Teamgram-CDN-Provider-Request-Signature", cdnProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, cdnProviderBodyLimit+1))
	if err != nil || len(responseBody) > cdnProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrCdnMethodInvalid
	}
	if !verifyCDNProviderSignature(signingKey, resp.Header.Get("X-Teamgram-CDN-Provider-Signature"), responseBody) {
		return mtproto.ErrCdnMethodInvalid
	}
	var envelope cdnProviderEnvelope
	if err = json.Unmarshal(responseBody, &envelope); err != nil || !envelope.Verified || envelope.RequestID != request.RequestID || envelope.Operation != request.Operation || envelope.UserID != request.UserID || envelope.Error != "" || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return mtproto.ErrCdnMethodInvalid
	}
	if err = json.Unmarshal(envelope.Result, result); err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	return nil
}

func (c *FilesCore) cdnProviderConfigured() bool {
	if c == nil || c.svcCtx == nil {
		return false
	}
	return strings.TrimSpace(c.svcCtx.Config.CdnProviderEndpoint) != "" && len(strings.TrimSpace(c.svcCtx.Config.CdnProviderSigningKey)) >= 32
}
