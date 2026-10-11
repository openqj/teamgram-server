package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

const apifullAIProviderTestSigningKey = "apifull-ai-provider-test-signing-key-0123456789"

func newAPIFullAIProviderCore(t *testing.T, operation string, result any, sign bool) *ApiFullCore {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Operation != operation || request.UserID != 42 || request.RequestID == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			http.Error(w, "bad result", http.StatusInternalServerError)
			return
		}
		body, err := json.Marshal(apifullAIProviderEnvelope{RequestID: request.RequestID, Verified: true, UserID: request.UserID, Result: resultJSON})
		if err != nil {
			http.Error(w, "bad envelope", http.StatusInternalServerError)
			return
		}
		if sign {
			w.Header().Set("X-Teamgram-AI-Provider-Signature", apifullAIProviderSignature(apifullAIProviderTestSigningKey, body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return &ApiFullCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{
			AIProviderEndpoint:       server.URL,
			AIProviderSigningKey:     apifullAIProviderTestSigningKey,
			AIProviderTimeoutSeconds: 2,
		}},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
}

func TestMessagesComposeMessageWithAIUsesSignedProvider(t *testing.T) {
	core := newAPIFullAIProviderCore(t, "compose_message", map[string]any{"result_text": map[string]any{"text": "rewritten"}}, true)
	input := mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "source"}).To_TextWithEntities()
	got, err := core.MessagesComposeMessageWithAI(&mtproto.TLMessagesComposeMessageWithAI{Text: input})
	if err != nil || got == nil || got.GetResultText() == nil || got.GetResultText().GetText() != "rewritten" {
		t.Fatalf("compose = (%v, %v), want signed provider result", got, err)
	}
}

func TestAicomposeGetToneExampleUsesSignedProvider(t *testing.T) {
	core := newAPIFullAIProviderCore(t, "tone_example", map[string]any{
		"from": map[string]any{"text": "plain"},
		"to":   map[string]any{"text": "formal"},
	}, true)
	got, err := core.AicomposeGetToneExample(&mtproto.TLAicomposeGetToneExample{Tone: &mtproto.InputAiComposeTone{Slug: "formal"}, Num: 1})
	if err != nil || got == nil || got.GetFrom() == nil || got.GetTo() == nil || got.GetTo().GetText() != "formal" {
		t.Fatalf("tone example = (%v, %v), want signed provider result", got, err)
	}
}
