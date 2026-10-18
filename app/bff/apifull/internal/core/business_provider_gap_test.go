package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

const businessProviderGapTestKey = "business-provider-gap-test-signing-key-0123456789"

func TestSecureBusinessProviderURLAllowsFixtureHTTPOnlyWhenConfigured(t *testing.T) {
	fixture, err := url.Parse("http://business-provider-fixture:23190")
	if err != nil {
		t.Fatal(err)
	}
	if secureBusinessProviderURL(fixture, false) {
		t.Fatal("accepted fixture HTTP without the explicit fixture setting")
	}
	if !secureBusinessProviderURL(fixture, true) {
		t.Fatal("rejected fixture HTTP with the explicit fixture setting")
	}

	public, err := url.Parse("https://business.example/provider")
	if err != nil {
		t.Fatal(err)
	}
	if !secureBusinessProviderURL(public, false) {
		t.Fatal("rejected HTTPS business provider")
	}
}

func newBusinessProviderGapCore(t *testing.T, operation string, result any) *ApiFullCore {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.RequestID == "" || request.Operation != operation || request.UserID != 42 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			http.Error(w, "bad result", http.StatusInternalServerError)
			return
		}
		body, err := json.Marshal(businessProviderEnvelope{RequestID: request.RequestID, Operation: operation, Verified: true, UserID: request.UserID, Result: resultJSON})
		if err != nil {
			http.Error(w, "bad envelope", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Teamgram-Business-Provider-Signature", businessProviderSignature(businessProviderGapTestKey, body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return &ApiFullCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{
			BusinessProviderEndpoint:       server.URL,
			BusinessProviderSigningKey:     businessProviderGapTestKey,
			BusinessProviderTimeoutSeconds: 2,
		}},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
}

func TestAffiliateAndBotBusinessMethodsUseSignedProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "bots.updateStarRefProgram", map[string]any{"predicate_name": mtproto.Predicate_starRefProgram, "bot_id": 7, "commission_permille": 125})
	program, err := core.BotsUpdateStarRefProgram(&mtproto.TLBotsUpdateStarRefProgram{
		Bot:                &mtproto.InputUser{UserId: 7, AccessHash: 9},
		CommissionPermille: 125,
	})
	if err != nil || program == nil || program.GetBotId() != 7 || program.GetCommissionPermille() != 125 {
		t.Fatalf("affiliate provider result = (%v, %v)", program, err)
	}

	core = newBusinessProviderGapCore(t, "bots.sendCustomRequest", map[string]any{"data": `{"ok":true}`})
	data, err := core.BotsSendCustomRequest(&mtproto.TLBotsSendCustomRequest{
		CustomMethod: "getStatus",
		Params:       &mtproto.DataJSON{Data: `{}`},
	})
	if err != nil || data == nil || data.GetData() != `{"ok":true}` {
		t.Fatalf("custom request provider result = (%v, %v)", data, err)
	}
}

func TestBusinessProviderUsesStableIdempotencyKey(t *testing.T) {
	var idempotencyKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		idempotencyKeys = append(idempotencyKeys, r.Header.Get("Idempotency-Key"))
		result, _ := json.Marshal(map[string]any{"ok": true})
		body, _ := json.Marshal(businessProviderEnvelope{RequestID: request.RequestID, Operation: request.Operation, Verified: true, UserID: request.UserID, Result: result})
		w.Header().Set("X-Teamgram-Business-Provider-Signature", businessProviderSignature(businessProviderGapTestKey, body))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	core := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{
			BusinessProviderEndpoint:   server.URL,
			BusinessProviderSigningKey: businessProviderGapTestKey,
		}},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
	for i := 0; i < 2; i++ {
		var result map[string]any
		if err := core.callBusinessProvider("bots.setJoinChatResults", map[string]any{"value": true}, &result); err != nil {
			t.Fatalf("business provider call %d: %v", i, err)
		}
	}
	if len(idempotencyKeys) != 2 || len(idempotencyKeys[0]) != 64 || idempotencyKeys[0] != idempotencyKeys[1] {
		t.Fatalf("idempotency keys = %#v, want same 64-character key", idempotencyKeys)
	}
}

func TestBusinessProviderSignedErrorMapsToMethodNotImpl(t *testing.T) {
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
		body, err := json.Marshal(businessProviderEnvelope{
			RequestID: request.RequestID,
			Operation: request.Operation,
			UserID:    request.UserID,
			Error:     "operation not provisioned",
		})
		if err != nil {
			http.Error(w, "bad envelope", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Teamgram-Business-Provider-Signature", businessProviderSignature(businessProviderGapTestKey, body))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	core := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{
			BusinessProviderEndpoint:   server.URL,
			BusinessProviderSigningKey: businessProviderGapTestKey,
		}},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
	var result map[string]any
	if err := core.callBusinessProvider("messages.getPaidReactionPrivacy", map[string]any{}, &result); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("signed provider error = %v, want METHOD_NOT_IMPL", err)
	}
}

func TestBusinessProviderGapRejectsMalformedResult(t *testing.T) {
	core := newBusinessProviderGapCore(t, "bots.updateStarRefProgram", map[string]any{"bot_id": 0, "commission_permille": 100})
	result, err := core.BotsUpdateStarRefProgram(&mtproto.TLBotsUpdateStarRefProgram{
		Bot:                &mtproto.InputUser{UserId: 7, AccessHash: 9},
		CommissionPermille: 100,
	})
	if result != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("malformed provider result = (%v, %v), want internal error", result, err)
	}
}

func TestMiniAppProviderMethodsUseSignedResponses(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.sendWebViewData", map[string]any{"predicate_name": "updates", "date": 1})
	updates, err := core.MessagesSendWebViewData(&mtproto.TLMessagesSendWebViewData{
		Bot: &mtproto.InputUser{UserId: 7, AccessHash: 9}, RandomId: 1, ButtonText: "Open", Data: "payload",
	})
	if err != nil || updates == nil || updates.GetPredicateName() == "" {
		t.Fatalf("send webview data = (%v, %v)", updates, err)
	}

	core = newBusinessProviderGapCore(t, "messages.requestChatJoinWebView", map[string]any{"predicate_name": "webViewResultUrl", "url": "https://mini.example/join", "query_id_INT64": 9})
	webview, err := core.MessagesRequestChatJoinWebView(&mtproto.TLMessagesRequestChatJoinWebView{QueryId: 9, Platform: "android"})
	if err != nil || webview == nil || webview.GetUrl() != "https://mini.example/join" {
		t.Fatalf("chat join webview = (%v, %v)", webview, err)
	}

	core = newBusinessProviderGapCore(t, "bots.invokeWebViewCustomMethod", map[string]any{"data": `{"ok":true}`})
	data, err := core.BotsInvokeWebViewCustomMethod(&mtproto.TLBotsInvokeWebViewCustomMethod{
		Bot: &mtproto.InputUser{UserId: 7, AccessHash: 9}, CustomMethod: "status", Params: &mtproto.DataJSON{Data: `{}`},
	})
	if err != nil || data == nil || data.GetData() == "" {
		t.Fatalf("invoke webview method = (%v, %v)", data, err)
	}
}

func TestPopularAppBotsRejectsMalformedProviderUsers(t *testing.T) {
	core := newBusinessProviderGapCore(t, "bots.getPopularAppBots", map[string]any{
		"predicate_name": mtproto.Predicate_bots_popularAppBots,
		"users":          []any{nil},
	})
	result, err := core.BotsGetPopularAppBots(&mtproto.TLBotsGetPopularAppBots{Limit: 20})
	if result != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("malformed popular app bots = (%v, %v), want nil and internal error", result, err)
	}
}

func TestPrepaidGiveawayUsesSignedProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "payments.launchPrepaidGiveaway", map[string]any{
		"predicate_name": "updates", "date": 1700000000,
	})
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 77, AccessHash: 88}).To_InputPeer()
	purpose := mtproto.MakeTLInputStorePaymentPremiumGiveaway(&mtproto.InputStorePaymentPurpose{
		Amount: 299, Currency: "USD", UntilDate: 1700000100,
	}).To_InputStorePaymentPurpose()
	updates, err := core.PaymentsLaunchPrepaidGiveaway(&mtproto.TLPaymentsLaunchPrepaidGiveaway{
		Peer: peer, GiveawayId: 123, Purpose: purpose,
	})
	if err != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("prepaid giveaway provider result = (%v, %v)", updates, err)
	}
}

func TestPrepaidGiveawayRejectsInvalidInputBeforeProvider(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 77}).To_InputPeer()
	purpose := mtproto.MakeTLInputStorePaymentPremiumGiveaway(&mtproto.InputStorePaymentPurpose{}).To_InputStorePaymentPurpose()
	for name, input := range map[string]*mtproto.TLPaymentsLaunchPrepaidGiveaway{
		"missing purpose":     {Peer: peer, GiveawayId: 123},
		"invalid giveaway id": {Peer: peer, GiveawayId: 0, Purpose: purpose},
		"wrong purpose":       {Peer: peer, GiveawayId: 123, Purpose: mtproto.MakeTLInputStorePaymentStarsTopup(nil).To_InputStorePaymentPurpose()},
	} {
		t.Run(name, func(t *testing.T) {
			updates, err := core.PaymentsLaunchPrepaidGiveaway(input)
			if updates != nil || err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("result = (%v, %v), want input request invalid", updates, err)
			}
		})
	}
}

func TestPrepaidGiveawayRejectsUnknownUpdatesConstructor(t *testing.T) {
	core := newBusinessProviderGapCore(t, "payments.launchPrepaidGiveaway", map[string]any{
		"predicate_name": "notUpdates", "date": 1700000000,
	})
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 77, AccessHash: 88}).To_InputPeer()
	purpose := mtproto.MakeTLInputStorePaymentPremiumGiveaway(nil).To_InputStorePaymentPurpose()
	updates, err := core.PaymentsLaunchPrepaidGiveaway(&mtproto.TLPaymentsLaunchPrepaidGiveaway{
		Peer: peer, GiveawayId: 123, Purpose: purpose,
	})
	if updates != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("unknown updates constructor = (%v, %v), want internal error", updates, err)
	}
}

func TestInlineProviderMethodsUseSignedResponses(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.getBotCallbackAnswer", map[string]any{
		"predicate_name": "messages_botCallbackAnswer", "cache_time": 5,
	})
	answer, err := core.MessagesGetBotCallbackAnswer(&mtproto.TLMessagesGetBotCallbackAnswer{
		Peer: &mtproto.InputPeer{UserId: 7}, MsgId: 1, Data: []byte{1},
	})
	if err != nil || answer == nil || answer.GetCacheTime() != 5 {
		t.Fatalf("callback answer = (%v, %v)", answer, err)
	}

	core = newBusinessProviderGapCore(t, "messages.sendBotRequestedPeer", map[string]any{"predicate_name": "updates", "date": 1})
	updates, err := core.MessagesSendBotRequestedPeer(&mtproto.TLMessagesSendBotRequestedPeer{
		Peer: &mtproto.InputPeer{UserId: 7}, RequestedPeer: &mtproto.InputPeer{UserId: 9},
	})
	if err != nil || updates == nil || updates.GetPredicateName() == "" {
		t.Fatalf("requested peer = (%v, %v)", updates, err)
	}

	core = newBusinessProviderGapCore(t, "messages.setBotGuestChatResult", map[string]any{"predicate_name": "inputBotInlineMessageID", "dc_id": 2, "id_INT64": 9001, "access_hash": 9002})
	result, err := core.MessagesSetBotGuestChatResult(&mtproto.TLMessagesSetBotGuestChatResult{
		QueryId: 1, Result: &mtproto.InputBotInlineResult{Id: "r", Type: "article"},
	})
	if err != nil || result == nil || result.GetPredicateName() == "" {
		t.Fatalf("guest result = (%v, %v)", result, err)
	}
}
