package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestWebViewProviderRejectsNonCanonicalUpdates(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.sendWebViewData", map[string]any{
		"predicate_name": "message",
	})
	result, err := core.MessagesSendWebViewData(&mtproto.TLMessagesSendWebViewData{
		Bot:        &mtproto.InputUser{UserId: 7, AccessHash: 9},
		RandomId:   1,
		ButtonText: "Open",
		Data:       "payload",
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("non-canonical updates = (%v, %v), want internal error", result, err)
	}
}

func TestWebViewProviderRejectsMalformedBotBeforeProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.sendWebViewData", map[string]any{
		"predicate_name": "updates", "date": 1,
	})
	result, err := core.MessagesSendWebViewData(&mtproto.TLMessagesSendWebViewData{
		Bot:        &mtproto.InputUser{UserId: 7},
		RandomId:   1,
		ButtonText: "Open",
		Data:       "payload",
	})
	if result != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("malformed bot = (%v, %v), want USER_ID_INVALID", result, err)
	}
}

func TestInlineEditRejectsMalformedMessageIDBeforeProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.editInlineBotMessage", map[string]any{"value": true})
	result, err := core.MessagesEditInlineBotMessage(&mtproto.TLMessagesEditInlineBotMessage{
		Id:      &mtproto.InputBotInlineMessageID{DcId: 2, AccessHash: 9},
		Message: wrapperspb.String("edited"),
	})
	if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("malformed inline message id = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
}

func TestWebViewProviderRejectsWrongResultConstructor(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.sendWebViewResultMessage", map[string]any{
		"predicate_name": "updates",
	})
	result, err := core.MessagesSendWebViewResultMessage(&mtproto.TLMessagesSendWebViewResultMessage{
		BotQueryId: "query-1",
		Result:     &mtproto.InputBotInlineResult{Id: "result-1", Type: "article"},
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("wrong WebView result constructor = (%v, %v), want internal error", result, err)
	}
}

func TestWebViewProviderRejectsNonCanonicalWebViewResult(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.requestChatJoinWebView", map[string]any{
		"predicate_name": "simpleWebViewResultUrl",
		"url":            "https://mini.example/join",
	})
	result, err := core.MessagesRequestChatJoinWebView(&mtproto.TLMessagesRequestChatJoinWebView{
		QueryId:  9,
		Platform: "android",
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("wrong WebView result constructor = (%v, %v), want internal error", result, err)
	}
}

func TestBotProviderRejectsMissingBooleanValue(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.setBotCallbackAnswer", map[string]any{})
	result, err := core.MessagesSetBotCallbackAnswer(&mtproto.TLMessagesSetBotCallbackAnswer{
		QueryId: 9,
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing callback boolean = (%v, %v), want internal error", result, err)
	}

	core = newBusinessProviderGapCore(t, "bots.answerWebhookJSONQuery", map[string]any{})
	result, err = core.BotsAnswerWebhookJSONQuery(&mtproto.TLBotsAnswerWebhookJSONQuery{
		QueryId: 9,
		Data:    &mtproto.DataJSON{Data: `{}`},
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing webhook boolean = (%v, %v), want internal error", result, err)
	}
}

func TestBotProviderRejectsInvalidDataJSON(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	if result, err := core.BotsSendCustomRequest(&mtproto.TLBotsSendCustomRequest{
		CustomMethod: "status",
		Params:       &mtproto.DataJSON{Data: "not-json"},
	}); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("invalid custom request JSON = (%v, %v), want input error", result, err)
	}
	if result, err := core.BotsInvokeWebViewCustomMethod(&mtproto.TLBotsInvokeWebViewCustomMethod{
		Bot:          &mtproto.InputUser{UserId: 7, AccessHash: 9},
		CustomMethod: "status",
		Params:       &mtproto.DataJSON{Data: "not-json"},
	}); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("invalid WebView custom JSON = (%v, %v), want input error", result, err)
	}
}
