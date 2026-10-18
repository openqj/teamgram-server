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

const (
	businessProviderMaxAttempts = 2
	businessProviderRetryDelay  = 50 * time.Millisecond
)

// Mini App calls may be retried after a transient provider failure. The
// provider contract carries a stable Idempotency-Key, so a retry cannot turn
// a repeated WebView action into a second provider mutation when the provider
// has already committed the first attempt.
func retryableMiniAppProviderOperation(operation string) bool {
	switch operation {
	case "messages.getBotApp",
		"messages.requestAppWebView",
		"messages.requestChatJoinWebView",
		"messages.sendWebViewData",
		"messages.sendWebViewResultMessage",
		"bots.getPopularAppBots",
		"bots.checkDownloadFileParams",
		"bots.invokeWebViewCustomMethod":
		return true
	default:
		return false
	}
}

func retryableBusinessProviderStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests ||
		(status >= http.StatusInternalServerError && status < 600)
}

func (c *ApiFullCore) logBusinessProviderFailure(operation string, userID int64, attempt, maxAttempts int, status int, err error) {
	if c == nil || c.Logger == nil {
		return
	}
	if err != nil {
		c.Logger.Errorf("business provider operation=%s user_id=%d attempt=%d/%d transport_error=%v", operation, userID, attempt, maxAttempts, err)
		return
	}
	c.Logger.Errorf("business provider operation=%s user_id=%d attempt=%d/%d http_status=%d", operation, userID, attempt, maxAttempts, status)
}

func (c *ApiFullCore) logBusinessProviderRetry(operation string, userID int64, attempt, maxAttempts, status int, err error) {
	if c == nil || c.Logger == nil {
		return
	}
	if err != nil {
		c.Logger.Infof("business provider retry operation=%s user_id=%d attempt=%d/%d transport_error=%v", operation, userID, attempt, maxAttempts, err)
		return
	}
	c.Logger.Infof("business provider retry operation=%s user_id=%d attempt=%d/%d http_status=%d", operation, userID, attempt, maxAttempts, status)
}

func waitBusinessProviderRetry(ctx context.Context) error {
	timer := time.NewTimer(businessProviderRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

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
	if c == nil || c.MD == nil {
		return mtproto.ErrMethodNotImpl
	}
	return c.callBusinessProviderAs(c.MD.GetUserId(), operation, payload, result)
}

// callBusinessProviderAs keeps provider identity aligned with the account that
// owns a business-scoped mutation. Native calls use the authenticated caller;
// invokeWithBusinessConnection calls can explicitly use the trusted owner.
func (c *ApiFullCore) callBusinessProviderAs(userID int64, operation string, payload any, result any) error {
	if c == nil || c.svcCtx == nil || userID <= 0 {
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
	}{hex.EncodeToString(nonce[:]), operation, userID, payloadJSON}
	body, err := json.Marshal(request)
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	idempotencyKey := businessProviderIdempotencyKey(userID, operation, payloadJSON)
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	maxAttempts := 1
	if retryableMiniAppProviderOperation(operation) {
		maxAttempts = businessProviderMaxAttempts
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			return mtproto.ErrInternalServerError
		}
		providercontract.SetRequestHeaders(req, body, key, idempotencyKey,
			"X-Teamgram-Business-Provider-Request-Signature", signingKey)
		resp, requestErr := client.Do(req)
		if requestErr != nil {
			if attempt < maxAttempts && ctx.Err() == nil {
				c.logBusinessProviderRetry(operation, userID, attempt, maxAttempts, 0, requestErr)
				if waitErr := waitBusinessProviderRetry(ctx); waitErr != nil {
					return mtproto.ErrMethodNotImpl
				}
				continue
			}
			c.logBusinessProviderFailure(operation, userID, attempt, maxAttempts, 0, requestErr)
			return mtproto.ErrMethodNotImpl
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, businessProviderBodyLimit+1))
		status := resp.StatusCode
		_ = resp.Body.Close()
		if readErr != nil {
			if attempt < maxAttempts && ctx.Err() == nil {
				c.logBusinessProviderRetry(operation, userID, attempt, maxAttempts, status, readErr)
				if waitErr := waitBusinessProviderRetry(ctx); waitErr != nil {
					return mtproto.ErrMethodNotImpl
				}
				continue
			}
			c.logBusinessProviderFailure(operation, userID, attempt, maxAttempts, status, readErr)
			return mtproto.ErrMethodNotImpl
		}
		if len(responseBody) > businessProviderBodyLimit {
			c.logBusinessProviderFailure(operation, userID, attempt, maxAttempts, status, nil)
			return mtproto.ErrMethodNotImpl
		}
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			if attempt < maxAttempts && retryableBusinessProviderStatus(status) && ctx.Err() == nil {
				c.logBusinessProviderRetry(operation, userID, attempt, maxAttempts, status, nil)
				if waitErr := waitBusinessProviderRetry(ctx); waitErr != nil {
					return mtproto.ErrMethodNotImpl
				}
				continue
			}
			c.logBusinessProviderFailure(operation, userID, attempt, maxAttempts, status, nil)
			return mtproto.ErrMethodNotImpl
		}
		if !verifyBusinessProviderSignature(signingKey, resp.Header.Get("X-Teamgram-Business-Provider-Signature"), responseBody) {
			return mtproto.ErrInternalServerError
		}
		var envelope businessProviderEnvelope
		if err = json.Unmarshal(responseBody, &envelope); err != nil || envelope.RequestID != request.RequestID || envelope.Operation != request.Operation || envelope.UserID != request.UserID {
			return mtproto.ErrInternalServerError
		}
		// A signed provider error is an authoritative capability miss (for example,
		// an operation that has not been provisioned for this caller), not a
		// malformed response. Preserve the canonical unavailable-method error while
		// keeping an unsigned or otherwise incomplete success response fail-closed.
		if envelope.Error != "" {
			return mtproto.ErrMethodNotImpl
		}
		if !envelope.Verified || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
			return mtproto.ErrInternalServerError
		}
		if err = json.Unmarshal(envelope.Result, result); err != nil {
			return mtproto.ErrInternalServerError
		}
		if attempt > 1 && c.Logger != nil {
			c.Logger.Infof("business provider operation=%s user_id=%d completed_after_retry attempt=%d/%d", operation, userID, attempt, maxAttempts)
		}
		return nil
	}
	return mtproto.ErrMethodNotImpl
}
