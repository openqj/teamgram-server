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

const sponsoredProviderBodyLimit = 1 << 20

type sponsoredProviderEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result"`
}

func sponsoredProviderSignature(key string, body []byte) string {
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// sponsoredProviderRequestID is stable for one logical RPC request. Provider
// retries must reuse this id so a timeout cannot create duplicate ad events or
// inventory reservations.
func sponsoredProviderRequestID(operation string, userID int64, payload []byte) string {
	digest := sha256.Sum256(append([]byte(operation+":"+strconv.FormatInt(userID, 10)+":"), payload...))
	return hex.EncodeToString(digest[:])
}

func verifySponsoredProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write(body)
	return hmac.Equal(provided, h.Sum(nil))
}

func secureSponsoredProviderURL(parsed *url.URL) bool {
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

func (c *SponsoredMessagesCore) callSponsoredProvider(operation string, payload any, result any) error {
	uid, err := c.requireSponsoredUser()
	if err != nil {
		return err
	}
	if c.svcCtx == nil {
		return mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.SponsoredProviderEndpoint)
	key := strings.TrimSpace(c.svcCtx.Config.SponsoredProviderKey)
	signingKey := strings.TrimSpace(c.svcCtx.Config.SponsoredProviderSigningKey)
	if endpoint == "" || len(signingKey) < 32 {
		return mtproto.ErrMethodNotImpl
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureSponsoredProviderURL(parsed) {
		return mtproto.ErrInternalServerError
	}
	if c.svcCtx.Config.SponsoredProviderTimeoutSeconds < 0 || c.svcCtx.Config.SponsoredProviderTimeoutSeconds > 300 {
		return mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.SponsoredProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.SponsoredProviderTimeoutSeconds) * time.Second
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
	}{sponsoredProviderRequestID(operation, uid, payloadJSON), operation, uid, payloadJSON}
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
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", request.RequestID)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("X-Teamgram-Sponsored-Provider-Request-Signature", sponsoredProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrMethodNotImpl
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, sponsoredProviderBodyLimit+1))
	if err != nil || len(responseBody) > sponsoredProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrMethodNotImpl
	}
	if !verifySponsoredProviderSignature(signingKey, resp.Header.Get("X-Teamgram-Sponsored-Provider-Signature"), responseBody) {
		return mtproto.ErrInternalServerError
	}
	var envelope sponsoredProviderEnvelope
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
