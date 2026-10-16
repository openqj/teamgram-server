package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type legacyAppWebViewStore struct{}

func (*legacyAppWebViewStore) Get(string) (string, error) { return "", nil }

func (*legacyAppWebViewStore) Set(string, string) error { return nil }

func legacyAppWebViewRequest(botID, accessHash int64) *mtproto.TLMessagesRequestAppWebView8C5A3B3C {
	return &mtproto.TLMessagesRequestAppWebView8C5A3B3C{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		App: mtproto.MakeTLInputBotAppID(&mtproto.InputBotApp{
			Id: accessHash + 1, AccessHash: accessHash + 2, BotId: miniBotInput(botID, accessHash),
		}).To_InputBotApp(),
		Platform: "ios",
	}
}

func legacyAppWebViewCore(botID, accessHash int64, bot bool) *ApiFullCore {
	return &ApiFullCore{
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: &miniBotAppsUserClient{
			users: map[int64]*mtproto.ImmutableUser{botID: miniBotProfile(botID, accessHash, bot)},
		}}},
		MD: &metadata.RpcMetadata{UserId: 42},
	}
}

func requireLegacyAppWebViewPostgres(t *testing.T) {
	t.Helper()
	if !persist.PostgresEnabled() {
		t.Skip("APIFull PostgreSQL is not configured")
	}
}

func TestMessagesRequestAppWebViewLegacyRejectsNilOrMalformedInput(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	for name, request := range map[string]*mtproto.TLMessagesRequestAppWebView8C5A3B3C{
		"nil":     nil,
		"empty":   {},
		"no peer": {App: legacyAppWebViewRequest(7, 8).GetApp()},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := core.MessagesRequestAppWebView8C5A3B3C(request)
			if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("legacy app webview input = (%#v, %v), want INPUT_REQUEST_INVALID", result, err)
			}
		})
	}
}

func TestMessagesRequestAppWebViewLegacyRequiresPostgresBeforeProvider(t *testing.T) {
	previous := persist.Default
	persist.Use(&legacyAppWebViewStore{})
	t.Cleanup(func() { persist.Use(previous) })

	core := newBusinessProviderGapCore(t, "messages.requestAppWebView", map[string]any{
		"predicate_name": mtproto.Predicate_webViewResultUrl,
		"url":            "https://mini.example/legacy",
	})
	result, err := core.MessagesRequestAppWebView8C5A3B3C(legacyAppWebViewRequest(3, 4))
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("legacy app webview without PostgreSQL = (%#v, %v), want METHOD_NOT_IMPL", result, err)
	}
}

func TestMessagesRequestAppWebViewLegacyUsesVerifiedProvider(t *testing.T) {
	requireLegacyAppWebViewPostgres(t)
	const (
		botID      = int64(991001)
		accessHash = int64(771221)
	)
	core := newBusinessProviderGapCore(t, "messages.requestAppWebView", map[string]any{
		"predicate_name": mtproto.Predicate_webViewResultUrl,
		"url":            "https://mini.example/legacy",
	})
	core.svcCtx.Dao = &apifullDao.Dao{UserClient: &miniBotAppsUserClient{
		users: map[int64]*mtproto.ImmutableUser{botID: miniBotProfile(botID, accessHash, true)},
	}}
	request := legacyAppWebViewRequest(botID, accessHash)
	request.GetApp().BotId = miniBotInput(botID, accessHash)
	request.StartParam = wrapperspb.String("launch")
	result, err := core.MessagesRequestAppWebView8C5A3B3C(request)
	if err != nil || result == nil || result.GetPredicateName() != mtproto.Predicate_appWebViewResultUrl || result.GetUrl() != "https://mini.example/legacy" {
		t.Fatalf("legacy app webview = (%#v, %v)", result, err)
	}
	if err = result.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("legacy app webview did not encode: %v", err)
	}
}

func TestMessagesRequestAppWebViewLegacyRejectsWrongProviderConstructor(t *testing.T) {
	requireLegacyAppWebViewPostgres(t)
	core := newBusinessProviderGapCore(t, "messages.requestAppWebView", map[string]any{
		"predicate_name": mtproto.Predicate_appWebViewResultUrl,
		"url":            "https://mini.example/legacy",
	})
	result, err := core.MessagesRequestAppWebView8C5A3B3C(legacyAppWebViewRequest(3, 4))
	if result != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("wrong provider constructor = (%#v, %v), want nil and INTERNAL_SERVER_ERROR", result, err)
	}
}

func TestMessagesRequestAppWebViewLegacyRejectsInvalidProviderURL(t *testing.T) {
	requireLegacyAppWebViewPostgres(t)
	core := newBusinessProviderGapCore(t, "messages.requestAppWebView", map[string]any{
		"predicate_name": mtproto.Predicate_webViewResultUrl,
		"url":            "http://mini.example/legacy",
	})
	core.svcCtx.Dao = &apifullDao.Dao{UserClient: &miniBotAppsUserClient{
		users: map[int64]*mtproto.ImmutableUser{3: miniBotProfile(3, 4, true)},
	}}
	result, err := core.MessagesRequestAppWebView8C5A3B3C(legacyAppWebViewRequest(3, 4))
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("invalid provider URL = (%#v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}

func TestMessagesRequestAppWebViewLegacyRejectsWrongBotIdentity(t *testing.T) {
	requireLegacyAppWebViewPostgres(t)
	const (
		botID      = int64(991002)
		accessHash = int64(771222)
	)
	for _, test := range []struct {
		name       string
		inputHash  int64
		profileBot bool
		wantErr    error
	}{
		{name: "wrong access hash", inputHash: accessHash + 1, profileBot: true, wantErr: mtproto.ErrUserIdInvalid},
		{name: "non bot", inputHash: accessHash, profileBot: false, wantErr: mtproto.ErrBotInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := newBusinessProviderGapCore(t, "messages.requestAppWebView", map[string]any{
				"predicate_name": mtproto.Predicate_webViewResultUrl,
				"url":            "https://mini.example/legacy",
			})
			core.svcCtx.Dao = &apifullDao.Dao{UserClient: &miniBotAppsUserClient{
				users: map[int64]*mtproto.ImmutableUser{botID: miniBotProfile(botID, accessHash, test.profileBot)},
			}}
			request := legacyAppWebViewRequest(botID, test.inputHash)
			result, err := core.MessagesRequestAppWebView8C5A3B3C(request)
			if result != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("wrong bot identity = (%#v, %v), want %v", result, err, test.wantErr)
			}
		})
	}
}

func TestMessagesRequestAppWebViewLegacyUsesCallerScopedPostgresResult(t *testing.T) {
	requireLegacyAppWebViewPostgres(t)
	const (
		botID      = int64(991003)
		accessHash = int64(771223)
	)
	core := legacyAppWebViewCore(botID, accessHash, true)
	request := legacyAppWebViewRequest(botID, accessHash)
	appKey := fmt.Sprintf("bot:%d:id:%d", botID, request.GetApp().GetId())
	scopeKey := miniAppWebViewScopeKey(appKey, 42, request.GetPeer(), request.GetStartParam(), request.GetPlatform())
	resultJSON := json.RawMessage(`{"predicate_name":"webViewResultUrl","query_id_INT64":991003,"url":"https://mini.example/local"}`)
	if err := persist.PutBusinessControlRecord(persist.BusinessControlRecord{
		Operation: "messages.requestAppWebView", ScopeKey: scopeKey, CallerUserID: 42, Result: resultJSON,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = persist.PutBusinessControlRecord(persist.BusinessControlRecord{
			Operation: "messages.requestAppWebView", ScopeKey: scopeKey, CallerUserID: 42,
			State: "revoked", Result: resultJSON,
		})
	})

	result, err := core.MessagesRequestAppWebView8C5A3B3C(request)
	if err != nil || result == nil || result.GetPredicateName() != mtproto.Predicate_appWebViewResultUrl || result.GetUrl() != "https://mini.example/local" {
		t.Fatalf("local legacy app webview = (%#v, %v)", result, err)
	}
	if result.GetUrl() == "" || result.GetPredicateName() != mtproto.Predicate_appWebViewResultUrl {
		t.Fatalf("legacy result lost URL-only constructor = %#v", result)
	}
}

func TestPersistAppWebViewResultPositiveQueryIDFailsWithoutPostgres(t *testing.T) {
	previous := persist.Default
	persist.Use(&legacyAppWebViewStore{})
	t.Cleanup(func() { persist.Use(previous) })

	err := persistAppWebViewResult(42, 7, &mtproto.WebViewResult{
		PredicateName:     mtproto.Predicate_webViewResultUrl,
		QueryId_FLAGINT64: wrapperspb.Int64(991004),
		Url:               "https://mini.example/legacy",
	}, legacyAppWebViewRequest(7, 8))
	if !errors.Is(err, persist.ErrMiniBotStateUnavailable) {
		t.Fatalf("positive query ID persistence error = %v, want %v", err, persist.ErrMiniBotStateUnavailable)
	}
}
