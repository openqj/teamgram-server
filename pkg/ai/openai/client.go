// Package openai implements the narrow OpenAI-compatible adapter used by the
// Teamgram AI methods. It deliberately returns decoded JSON only; TL
// constructor validation remains in the calling BFF.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const bodyLimit = 1 << 20

type Config struct {
	Endpoint string
	APIKey   string
	Model    string
	Timeout  time.Duration
}

type completionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float32   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func secureURL(parsed *url.URL) bool {
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

func endpointURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !secureURL(u) {
		return nil, errors.New("invalid AI provider endpoint")
	}
	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(path, "/chat/completions") {
		if path == "" {
			path = "/v1"
		}
		u.Path = path + "/chat/completions"
	}
	return u, nil
}

func prompt(operation string, userID int64, payload any) (string, error) {
	b, err := json.Marshal(struct {
		Operation string `json:"operation"`
		UserID    int64  `json:"user_id"`
		Payload   any    `json:"payload"`
	}{operation, userID, payload})
	if err != nil {
		return "", err
	}
	schema := map[string]string{
		"summarize_text":         `{"text":"string","entities":[]}`,
		"translate_rich_message": `[ {"text":"string","entities":[]} ]`,
		"compose_rich_message":   `{"text":"string","entities":[]}`,
		"compose_message":        `{"result_text":{"text":"string","entities":[]},"diff_text":null}`,
		"tone_example":           `{"from":{"text":"string","entities":[]},"to":{"text":"string","entities":[]}}`,
	}[operation]
	if schema == "" {
		return "", errors.New("unsupported AI operation")
	}
	return fmt.Sprintf("You are the Teamgram Layer 229 AI provider. Return ONLY valid JSON, with no markdown or commentary. The JSON must match this schema exactly: %s. Preserve the requested meaning and do not invent entities. Request: %s", schema, b), nil
}

func contentBytes(raw json.RawMessage) ([]byte, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, errors.New("empty AI content")
		}
		return []byte(text), nil
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("empty AI content")
	}
	return raw, nil
}

func Call(ctx context.Context, cfg Config, operation string, userID int64, payload any, result any) error {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" || userID <= 0 || result == nil {
		return errors.New("AI provider is not configured")
	}
	u, err := endpointURL(cfg.Endpoint)
	if err != nil {
		return err
	}
	text, err := prompt(operation, userID, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(completionRequest{Model: cfg.Model, Messages: []message{{Role: "system", Content: "Return machine-readable JSON only."}, {Role: "user", Content: text}}, Temperature: 0, MaxTokens: 2048})
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil || len(responseBody) > bodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return errors.New("AI provider request failed")
	}
	var completion completionResponse
	if err = json.Unmarshal(responseBody, &completion); err != nil || len(completion.Choices) == 0 {
		return errors.New("AI provider response is malformed")
	}
	content, err := contentBytes(completion.Choices[0].Message.Content)
	if err != nil || !json.Valid(content) {
		return errors.New("AI provider content is not JSON")
	}
	if err = json.Unmarshal(content, result); err != nil {
		return errors.New("AI provider result does not match operation schema")
	}
	return nil
}
