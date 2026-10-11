package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
)

const aiProviderTestSigningKey = "ai-provider-test-signing-key-0123456789"

func newAIProviderCore(t *testing.T, operation string, result any, sign bool) *MessagesCore {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.Operation != operation || request.UserID != 42 || request.RequestID == "" {
			http.Error(w, "wrong operation", http.StatusBadRequest)
			return
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			http.Error(w, "bad result", http.StatusInternalServerError)
			return
		}
		body, err := json.Marshal(aiProviderEnvelope{
			RequestID: request.RequestID,
			Verified:  true,
			UserID:    request.UserID,
			Result:    resultJSON,
		})
		if err != nil {
			http.Error(w, "bad envelope", http.StatusInternalServerError)
			return
		}
		if sign {
			w.Header().Set("X-Teamgram-AI-Provider-Signature", aiProviderSignature(aiProviderTestSigningKey, body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return &MessagesCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{
			AIProviderEndpoint:       server.URL,
			AIProviderSigningKey:     aiProviderTestSigningKey,
			AIProviderTimeoutSeconds: 2,
		}},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
}

func TestMessagesSummarizeTextUsesSignedProvider(t *testing.T) {
	core := newAIProviderCore(t, "summarize_text", map[string]any{"text": "summary"}, true)
	got, err := core.MessagesSummarizeText(&mtproto.TLMessagesSummarizeText{Id: 7})
	if err != nil || got == nil || got.GetText() != "summary" || got.GetPredicateName() != mtproto.Predicate_textWithEntities {
		t.Fatalf("summary = (%v, %v), want signed provider result", got, err)
	}
}

func TestMessagesTranslateRichMessageUsesSignedProvider(t *testing.T) {
	core := newAIProviderCore(t, "translate_rich_message", []map[string]any{{}}, true)
	input := mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{Html: "source"}).To_InputRichMessage()
	got, err := core.MessagesTranslateRichMessage(&mtproto.TLMessagesTranslateRichMessage{Text: []*mtproto.InputRichMessage{input}, ToLang: "fr"})
	if err != nil || got == nil || len(got.GetResult()) != 1 || got.GetResult()[0].GetPredicateName() != mtproto.Predicate_richMessage {
		t.Fatalf("translation = (%v, %v), want signed provider result", got, err)
	}
}

func TestMessagesComposeRichMessageWithAIRejectsUnsignedProvider(t *testing.T) {
	core := newAIProviderCore(t, "compose_rich_message", map[string]any{}, false)
	input := mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{Html: "source"}).To_InputRichMessage()
	got, err := core.MessagesComposeRichMessageWithAI(&mtproto.TLMessagesComposeRichMessageWithAI{Text: input})
	if got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("compose = (%v, %v), want unsigned provider rejection", got, err)
	}
}

func TestMessagesSummarizeTextUsesOpenAICompatibleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer openai-test-key" {
			t.Fatalf("path/auth = %s/%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"text":"summary","entities":[]}`}}}})
	}))
	defer server.Close()
	core := &MessagesCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{AIProviderEndpoint: server.URL + "/v1", AIProviderKey: "openai-test-key", AIProviderModel: "gpt-6.1-sol"}},
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	got, err := core.MessagesSummarizeText(&mtproto.TLMessagesSummarizeText{Id: 7})
	if err != nil || got == nil || got.GetText() != "summary" {
		t.Fatalf("summary = (%v, %v), want OpenAI-compatible result", got, err)
	}
}

func TestMessagesTranslateRichMessageUsesOpenAICompatibleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `[{"blocks":[],"photos":[],"documents":[]}]`}}}})
	}))
	defer server.Close()
	core := &MessagesCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{AIProviderEndpoint: server.URL + "/v1", AIProviderKey: "openai-test-key", AIProviderModel: "gpt-6.1-sol"}},
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	input := mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{Html: "source"}).To_InputRichMessage()
	got, err := core.MessagesTranslateRichMessage(&mtproto.TLMessagesTranslateRichMessage{Text: []*mtproto.InputRichMessage{input}, ToLang: "fr"})
	if err != nil || got == nil || len(got.GetResult()) != 1 || got.GetResult()[0].GetPredicateName() != mtproto.Predicate_richMessage {
		t.Fatalf("translation = (%v, %v), want OpenAI-compatible result", got, err)
	}
}
