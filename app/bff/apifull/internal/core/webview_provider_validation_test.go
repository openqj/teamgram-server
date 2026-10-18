package core

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMiniAppProviderAuthenticatesBotBeforeExternalCall(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "provider must not be called", http.StatusInternalServerError)
	}))
	defer server.Close()
	core := &ApiFullCore{
		svcCtx: &svc.ServiceContext{
			Config: config.Config{BusinessProviderEndpoint: server.URL, BusinessProviderSigningKey: businessProviderGapTestKey},
			Dao: &apifullDao.Dao{UserClient: &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
				7: miniBotProfile(7, 9, true),
			}}},
		},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
	badBot := miniBotInput(7, 10)
	checks := []struct {
		name string
		call func() error
	}{
		{"sendWebViewData", func() error {
			_, err := core.MessagesSendWebViewData(&mtproto.TLMessagesSendWebViewData{Bot: badBot, RandomId: 1, ButtonText: "Open", Data: "payload"})
			return err
		}},
		{"invokeWebViewCustomMethod", func() error {
			_, err := core.BotsInvokeWebViewCustomMethod(&mtproto.TLBotsInvokeWebViewCustomMethod{Bot: badBot, CustomMethod: "status", Params: &mtproto.DataJSON{Data: `{}`}})
			return err
		}},
		{"checkDownloadFileParams", func() error {
			_, err := core.BotsCheckDownloadFileParams(&mtproto.TLBotsCheckDownloadFileParams{Bot: badBot, FileName: "file.txt", Url: "https://mini.example/file.txt"})
			return err
		}},
		{"getBotApp", func() error {
			_, err := core.MessagesGetBotApp(&mtproto.TLMessagesGetBotApp{App: mtproto.MakeTLInputBotAppShortName(&mtproto.InputBotApp{BotId: badBot, ShortName: "demo"}).To_InputBotApp(), Hash: 1})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, mtproto.ErrUserIdInvalid) {
				t.Fatalf("error = %v, want USER_ID_INVALID", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("provider calls = %d, want 0", calls)
	}
}

func TestSendWebViewResultMessageRequiresBotCallerBeforeProvider(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "provider must not be called", http.StatusInternalServerError)
	}))
	defer server.Close()
	core := &ApiFullCore{
		svcCtx: &svc.ServiceContext{
			Config: config.Config{BusinessProviderEndpoint: server.URL, BusinessProviderSigningKey: businessProviderGapTestKey},
			Dao: &apifullDao.Dao{UserClient: &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
				42: miniBotProfile(42, 42, false),
			}}},
		},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
	result, err := core.MessagesSendWebViewResultMessage(&mtproto.TLMessagesSendWebViewResultMessage{
		BotQueryId: "query-1",
		Result:     &mtproto.InputBotInlineResult{Id: "result-1", Type: "article"},
	})
	if result != nil || !errors.Is(err, mtproto.ErrBotMethodInvalid) {
		t.Fatalf("result/error = (%v, %v), want BOT_METHOD_INVALID", result, err)
	}
	if calls != 0 {
		t.Fatalf("provider calls = %d, want 0", calls)
	}
}

func TestWebViewProviderRejectsNonCanonicalUpdates(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.sendWebViewData", map[string]any{
		"predicate_name": "message",
	})
	result, err := core.MessagesSendWebViewData(&mtproto.TLMessagesSendWebViewData{
		Bot:        miniBotInput(7, 9),
		RandomId:   1,
		ButtonText: "Open",
		Data:       "payload",
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("non-canonical updates = (%v, %v), want internal error", result, err)
	}
}

func TestWebViewResultOptionalQueryIDPreservesLayer229Semantics(t *testing.T) {
	optional := &mtproto.WebViewResult{
		PredicateName: mtproto.Predicate_webViewResultUrl,
		Url:           "https://mini.example/app",
	}
	if queryID, present, err := webViewResultQueryID(optional); err != nil || present || queryID != 0 {
		t.Fatalf("optional query id = (%d, %v, %v), want absent without error", queryID, present, err)
	}

	fixed := &mtproto.WebViewResult{
		PredicateName: mtproto.Predicate_webViewResultUrl,
		Constructor:   mtproto.TLConstructor_CRC32_webViewResultUrl_c14557c,
		Url:           "https://mini.example/app",
	}
	if queryID, present, err := webViewResultQueryID(fixed); queryID != 0 || present || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("fixed query id = (%d, %v, %v), want internal error", queryID, present, err)
	}
	modernWithLegacyID := &mtproto.WebViewResult{
		PredicateName: mtproto.Predicate_webViewResultUrl,
		Constructor:   mtproto.TLConstructor_CRC32_webViewResultUrl_4d22ff98,
		QueryId_INT64: 993,
	}
	if queryID, present, err := webViewResultQueryID(modernWithLegacyID); queryID != 0 || present || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("modern result with legacy query id = (%d, %v, %v), want internal error", queryID, present, err)
	}
	legacyWithFlaggedID := &mtproto.WebViewResult{
		PredicateName:     mtproto.Predicate_webViewResultUrl,
		Constructor:       mtproto.TLConstructor_CRC32_webViewResultUrl_c14557c,
		QueryId_FLAGINT64: wrapperspb.Int64(994),
	}
	if queryID, present, err := webViewResultQueryID(legacyWithFlaggedID); queryID != 0 || present || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("legacy result with flagged query id = (%d, %v, %v), want internal error", queryID, present, err)
	}
	unknown := &mtproto.WebViewResult{PredicateName: mtproto.Predicate_webViewResultUrl, Constructor: mtproto.TLConstructor(12345)}
	if queryID, present, err := webViewResultQueryID(unknown); queryID != 0 || present || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("unknown result constructor = (%d, %v, %v), want internal error", queryID, present, err)
	}
}

func TestMiniAppThemeParamsValidation(t *testing.T) {
	for name, params := range map[string]*mtproto.DataJSON{
		"nil":       nil,
		"object":    {Data: `{"bg_color":"#ffffff","unknown":"future"}`},
		"empty obj": {Data: `{}`},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateMiniAppThemeParams(params); err != nil {
				t.Fatalf("theme params rejected: %v", err)
			}
		})
	}
	for name, params := range map[string]*mtproto.DataJSON{
		"empty":        {Data: ""},
		"malformed":    {Data: `{"bg_color"`},
		"array":        {Data: `[]`},
		"scalar":       {Data: `"#ffffff"`},
		"non-string":   {Data: `{"bg_color":null}`},
		"nested-value": {Data: `{"colors":{"bg":"#fff"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateMiniAppThemeParams(params); err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("theme params error = %v, want INPUT_REQUEST_INVALID", err)
			}
		})
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
		Bot:          miniBotInput(7, 9),
		CustomMethod: "status",
		Params:       &mtproto.DataJSON{Data: "not-json"},
	}); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("invalid WebView custom JSON = (%v, %v), want input error", result, err)
	}
}
