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
	"github.com/teamgram/teamgram-server/pkg/ai/openai"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const apifullAIProviderBodyLimit = 1 << 20

type apifullAIProviderEnvelope struct {
	RequestID string          `json:"request_id"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Result    json.RawMessage `json:"result"`
}

func apifullAIProviderSignature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyAPIFullAIProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func secureAPIFullAIProviderURL(parsed *url.URL) bool {
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

func (c *ApiFullCore) callAPIFullAIProvider(operation string, payload any, result any) error {
	if c == nil || c.svcCtx == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.AIProviderEndpoint)
	key := strings.TrimSpace(c.svcCtx.Config.AIProviderKey)
	signingKey := strings.TrimSpace(c.svcCtx.Config.AIProviderSigningKey)
	if endpoint == "" || len(signingKey) < 32 {
		if endpoint == "" || key == "" || signingKey != "" {
			return mtproto.ErrMethodNotImpl
		}
		model := strings.TrimSpace(c.svcCtx.Config.AIProviderModel)
		if model == "" {
			model = "gpt-6.1-sol"
		}
		if err := openai.Call(c.ctx, openai.Config{Endpoint: endpoint, APIKey: key, Model: model, Timeout: time.Duration(c.svcCtx.Config.AIProviderTimeoutSeconds) * time.Second}, operation, c.MD.GetUserId(), payload, result); err != nil {
			return mtproto.ErrMethodNotImpl
		}
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureAPIFullAIProviderURL(parsed) {
		return mtproto.ErrInternalServerError
	}
	if c.svcCtx.Config.AIProviderTimeoutSeconds < 0 || c.svcCtx.Config.AIProviderTimeoutSeconds > 300 {
		return mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.AIProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.AIProviderTimeoutSeconds) * time.Second
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
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("X-Teamgram-AI-Provider-Request-Signature", apifullAIProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrMethodNotImpl
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, apifullAIProviderBodyLimit+1))
	if err != nil || len(responseBody) > apifullAIProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrMethodNotImpl
	}
	if !verifyAPIFullAIProviderSignature(signingKey, resp.Header.Get("X-Teamgram-AI-Provider-Signature"), responseBody) {
		return mtproto.ErrInternalServerError
	}
	var envelope apifullAIProviderEnvelope
	if err = json.Unmarshal(responseBody, &envelope); err != nil || !envelope.Verified || envelope.RequestID != request.RequestID || envelope.UserID != request.UserID || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return mtproto.ErrInternalServerError
	}
	if err = json.Unmarshal(envelope.Result, result); err != nil {
		return mtproto.ErrInternalServerError
	}
	return nil
}

func apifullAIString(value *wrapperspb.StringValue) string {
	if value == nil {
		return ""
	}
	return value.GetValue()
}

func normalizeAPIFullAIText(value *mtproto.TextWithEntities) (*mtproto.TextWithEntities, error) {
	if value == nil || strings.TrimSpace(value.GetText()) == "" {
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLTextWithEntities(value).To_TextWithEntities(), nil
}
