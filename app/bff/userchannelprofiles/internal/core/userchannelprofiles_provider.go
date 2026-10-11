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

const userChannelProfilesProviderBodyLimit = 1 << 20

type birthdaySuggestionProviderRequest struct {
	RequestID string             `json:"request_id"`
	Operation string             `json:"operation"`
	CallerID  int64              `json:"caller_id"`
	TargetID  int64              `json:"target_id"`
	Recipient *mtproto.InputUser `json:"recipient"`
	Birthday  *mtproto.Birthday  `json:"birthday"`
}

type birthdaySuggestionProviderResponse struct {
	RequestID string           `json:"request_id"`
	Operation string           `json:"operation"`
	Verified  bool             `json:"verified"`
	CallerID  int64            `json:"caller_id"`
	TargetID  int64            `json:"target_id"`
	Result    *mtproto.Updates `json:"result"`
}

func userChannelProfilesProviderSignature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyUserChannelProfilesProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func secureUserChannelProfilesProviderURL(parsed *url.URL) bool {
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

func (c *UserChannelProfilesCore) callBirthdaySuggestionProvider(ctx context.Context, payload birthdaySuggestionProviderRequest) (*mtproto.Updates, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.svcCtx == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.UserChannelProfilesProviderEndpoint)
	key := strings.TrimSpace(c.svcCtx.Config.UserChannelProfilesProviderKey)
	signingKey := strings.TrimSpace(c.svcCtx.Config.UserChannelProfilesProviderSigningKey)
	if endpoint == "" || len(signingKey) < 32 {
		return nil, mtproto.ErrMethodNotImpl
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureUserChannelProfilesProviderURL(parsed) {
		return nil, mtproto.ErrInternalServerError
	}
	if c.svcCtx.Config.UserChannelProfilesProviderTimeoutSeconds < 0 || c.svcCtx.Config.UserChannelProfilesProviderTimeoutSeconds > 300 {
		return nil, mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.UserChannelProfilesProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.UserChannelProfilesProviderTimeoutSeconds) * time.Second
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	payload.RequestID = hex.EncodeToString(nonce[:])
	payload.Operation = "users.suggestBirthday"
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("X-Teamgram-UserChannelProfiles-Provider-Request-Signature", userChannelProfilesProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, userChannelProfilesProviderBodyLimit+1))
	if err != nil || len(responseBody) > userChannelProfilesProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mtproto.ErrMethodNotImpl
	}
	if !verifyUserChannelProfilesProviderSignature(signingKey, resp.Header.Get("X-Teamgram-UserChannelProfiles-Provider-Signature"), responseBody) {
		return nil, mtproto.ErrInternalServerError
	}
	var result birthdaySuggestionProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.RequestID != payload.RequestID || result.Operation != payload.Operation || result.CallerID != payload.CallerID || result.TargetID != payload.TargetID || result.Result == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if !validBirthdaySuggestionUpdates(result.Result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result.Result, nil
}

func validBirthdaySuggestionUpdates(result *mtproto.Updates) bool {
	if result == nil {
		return false
	}
	if result.GetPredicateName() == "" {
		return false
	}
	switch result.GetPredicateName() {
	case mtproto.Predicate_updates, mtproto.Predicate_updatesCombined, mtproto.Predicate_updateShort, mtproto.Predicate_updateShortMessage, mtproto.Predicate_updateShortChatMessage, mtproto.Predicate_updateShortSentMessage:
		return true
	default:
		return false
	}
}
