package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallOpenAICompatibleJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("request path/auth = %s/%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var req completionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 2 || !strings.Contains(req.Messages[1].Content, `"operation":"summarize_text"`) {
			t.Fatalf("request = %#v, err=%v", req, err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"text":"summary","entities":[]}`}}}})
	}))
	defer server.Close()
	var result struct {
		Text string `json:"text"`
	}
	err := Call(context.Background(), Config{Endpoint: server.URL + "/v1", APIKey: "test-key", Model: "gpt-6.1-sol"}, "summarize_text", 42, map[string]any{"id": 7}, &result)
	if err != nil || result.Text != "summary" {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
}

func TestCallRejectsUnsupportedOperation(t *testing.T) {
	err := Call(context.Background(), Config{Endpoint: "https://example.test/v1", APIKey: "test-key", Model: "gpt-6.1-sol"}, "unknown", 42, nil, &struct{}{})
	if err == nil {
		t.Fatal("expected unsupported operation error")
	}
}
