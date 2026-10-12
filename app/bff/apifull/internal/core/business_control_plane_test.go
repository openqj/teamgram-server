package core

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBusinessControlPlanePublishedTLJSONDecodesToCanonicalTypes(t *testing.T) {
	var updates mtproto.Updates
	if err := json.Unmarshal([]byte(`{"predicate_name":"updates","updates":[],"users":[],"chats":[],"date":1730000000,"seq":9}`), &updates); err != nil {
		t.Fatal(err)
	}
	if err := validateUpdatesResult(&updates); err != nil {
		t.Fatalf("canonical updates rejected: %v", err)
	}

	var botApp mtproto.Messages_BotApp
	if err := json.Unmarshal([]byte(`{"predicate_name":"messages_botApp","app":{"predicate_name":"botApp","id":7,"access_hash":77,"short_name":"shop","title":"Shop","description":"Published app"}}`), &botApp); err != nil {
		t.Fatal(err)
	}
	if botApp.GetPredicateName() != mtproto.Predicate_messages_botApp || botApp.GetApp() == nil || botApp.GetApp().GetId() != 7 || botApp.GetApp().GetAccessHash() != 77 {
		t.Fatalf("decoded bot app = %+v", &botApp)
	}

	var callback mtproto.Messages_BotCallbackAnswer
	if err := json.Unmarshal([]byte(`{"predicate_name":"messages_botCallbackAnswer","alert":true,"message":{"value":"Approved"},"cache_time":30}`), &callback); err != nil {
		t.Fatal(err)
	}
	if callback.GetPredicateName() != mtproto.Predicate_messages_botCallbackAnswer || callback.GetCacheTime() != 30 {
		t.Fatalf("decoded callback = %+v", &callback)
	}

	var webView mtproto.WebViewResult
	if err := json.Unmarshal([]byte(`{"predicate_name":"webViewResultUrl","constructor":202659196,"query_id_INT64":880003,"url":"https://mini.example/app"}`), &webView); err != nil {
		t.Fatal(err)
	}
	if webView.GetPredicateName() != mtproto.Predicate_webViewResultUrl || webView.GetConstructor() != mtproto.TLConstructor_CRC32_webViewResultUrl_c14557c || webView.GetQueryId_INT64() != 880003 || webView.GetUrl() != "https://mini.example/app" {
		t.Fatalf("decoded web view = %+v", &webView)
	}
}

func TestWebViewResultQueryIDRequiresCanonicalPositiveID(t *testing.T) {
	if got, present, err := webViewResultQueryID(&mtproto.WebViewResult{QueryId_FLAGINT64: wrapperspb.Int64(991)}); err != nil || !present || got != 991 {
		t.Fatalf("flag query id = %d, present=%v, %v", got, present, err)
	}
	if got, present, err := webViewResultQueryID(&mtproto.WebViewResult{QueryId_INT64: 992}); err != nil || !present || got != 992 {
		t.Fatalf("plain query id = %d, present=%v, %v", got, present, err)
	}
	if got, present, err := webViewResultQueryID(&mtproto.WebViewResult{}); err != nil || present || got != 0 {
		t.Fatalf("optional flag query id = %d, present=%v, %v", got, present, err)
	}
	if got, present, err := webViewResultQueryID(&mtproto.WebViewResult{
		Constructor: mtproto.TLConstructor_CRC32_webViewResultUrl_c14557c,
	}); err != mtproto.ErrInternalServerError || present || got != 0 {
		t.Fatalf("fixed query id = %d, present=%v, %v", got, present, err)
	}
	for name, result := range map[string]*mtproto.WebViewResult{
		"nil":       nil,
		"zero flag": {QueryId_FLAGINT64: wrapperspb.Int64(0)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := webViewResultQueryID(result); err != mtproto.ErrInternalServerError {
				t.Fatalf("query id error = %v, want internal error", err)
			}
		})
	}
}

func TestBusinessControlPlanePopularAppBotsLocalPath(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL control-plane schema unavailable: %v", err)
	}
	result := json.RawMessage(`{"predicate_name":"bots_popularAppBots","users":[]}`)
	if err := persist.PutBusinessControlRecord(persist.BusinessControlRecord{
		Operation: "bots.getPopularAppBots", ScopeKey: "offset::limit:20", CallerUserID: 42, Result: result,
	}); err != nil {
		t.Fatal(err)
	}
	core := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{}},
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	got, err := core.BotsGetPopularAppBots(&mtproto.TLBotsGetPopularAppBots{Offset: "", Limit: 20})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_bots_popularAppBots {
		t.Fatalf("local popular app bots = (%v, %v)", got, err)
	}
}
