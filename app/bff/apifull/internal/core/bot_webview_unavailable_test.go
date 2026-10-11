package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

type botWebViewStoreProbe struct {
	reads  int
	writes int
}

func (s *botWebViewStoreProbe) Get(string) (string, error) {
	s.reads++
	return "", nil
}

func (s *botWebViewStoreProbe) Set(string, string) error {
	s.writes++
	return nil
}

func TestBotAndWebViewMethodsFailClosedWithoutProvider(t *testing.T) {
	oldStore := persist.Default
	store := &botWebViewStoreProbe{}
	persist.Use(store)
	t.Cleanup(func() { persist.Use(oldStore) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81018001}}
	calls := []struct {
		name string
		call func() error
	}{
		{"inline query", func() error { _, err := c.MessagesGetInlineBotResults(nil); return err }},
		{"inline result submission", func() error { _, err := c.MessagesSetInlineBotResults(nil); return err }},
		{"inline result send", func() error { _, err := c.MessagesSendInlineBotResult(nil); return err }},
		{"inline message edit", func() error { _, err := c.MessagesEditInlineBotMessage(nil); return err }},
		{"callback answer query", func() error { _, err := c.MessagesGetBotCallbackAnswer(nil); return err }},
		{"inline callback", func() error { _, err := c.MessagesSetBotCallbackAnswer(nil); return err }},
		{"requested peer", func() error { _, err := c.MessagesSendBotRequestedPeer(nil); return err }},
		{"guest chat result", func() error { _, err := c.MessagesSetBotGuestChatResult(nil); return err }},
		{"webview request", func() error { _, err := c.MessagesRequestWebView(nil); return err }},
		{"webview prolong", func() error { _, err := c.MessagesProlongWebView(nil); return err }},
		{"simple webview", func() error { _, err := c.MessagesRequestSimpleWebView413A3E73(nil); return err }},
		{"simple webview legacy", func() error { _, err := c.MessagesRequestSimpleWebView1A46500A(nil); return err }},
		{"simple webview URL", func() error { _, err := c.MessagesRequestSimpleWebView299BEC8E(nil); return err }},
		{"simple webview no URL", func() error { _, err := c.MessagesRequestSimpleWebView6ABB2F73(nil); return err }},
		{"webview result message", func() error { _, err := c.MessagesSendWebViewResultMessage(nil); return err }},
		{"webview data", func() error { _, err := c.MessagesSendWebViewData(nil); return err }},
		{"bot app lookup", func() error { _, err := c.MessagesGetBotApp(nil); return err }},
		{"app webview", func() error { _, err := c.MessagesRequestAppWebView53618BCE(nil); return err }},
		{"app webview legacy", func() error { _, err := c.MessagesRequestAppWebView8C5A3B3C(nil); return err }},
		{"chat join webview", func() error { _, err := c.MessagesRequestChatJoinWebView(nil); return err }},
		{"webview custom method", func() error { _, err := c.BotsInvokeWebViewCustomMethod(nil); return err }},
		{"download authorization", func() error { _, err := c.BotsCheckDownloadFileParams(nil); return err }},
		{"webview button", func() error { _, err := c.BotsRequestWebViewButton(nil); return err }},
		{"main webview", func() error { _, err := c.MessagesRequestMainWebView(nil); return err }},
		{"popular app bots", func() error { _, err := c.BotsGetPopularAppBots(nil); return err }},
		{"join chat results", func() error { _, err := c.BotsSetJoinChatResults(nil); return err }},
		{"legacy bot info languages", func() error { _, err := c.BotsGetBotInfo75EC12E6(nil); return err }},
		{"bot updates status", func() error { _, err := c.HelpSetBotUpdatesStatus(nil); return err }},
		{"custom request", func() error { _, err := c.BotsSendCustomRequest(nil); return err }},
		{"webhook answer", func() error { _, err := c.BotsAnswerWebhookJSONQuery(nil); return err }},
		{"broadcast rights", func() error {
			_, err := c.BotsSetBotBroadcastDefaultAdminRights(&mtproto.TLBotsSetBotBroadcastDefaultAdminRights{AdminRights: mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{}).To_ChatAdminRights()})
			return err
		}},
		{"group rights", func() error {
			_, err := c.BotsSetBotGroupDefaultAdminRights(&mtproto.TLBotsSetBotGroupDefaultAdminRights{AdminRights: mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{}).To_ChatAdminRights()})
			return err
		}},
		{"custom verification", func() error { _, err := c.BotsSetCustomVerification(nil); return err }},
	}
	for _, tc := range calls {
		if err := tc.call(); !errors.Is(err, mtproto.ErrMethodNotImpl) && !errors.Is(err, mtproto.ErrInputRequestInvalid) && !errors.Is(err, mtproto.ErrQueryIdEmpty) {
			t.Errorf("%s: got %v, want a fail-closed provider or input error", tc.name, err)
		}
	}
	if store.reads != 0 || store.writes != 0 {
		t.Fatalf("unsupported methods touched persistence: reads=%d writes=%d", store.reads, store.writes)
	}
}

func TestBotAndWebViewUnavailableAuthenticatesBeforeMethodCheck(t *testing.T) {
	c := &ApiFullCore{}
	calls := []struct {
		name string
		call func() error
	}{
		{"inline query", func() error { _, err := c.MessagesGetInlineBotResults(nil); return err }},
		{"inline result submission", func() error { _, err := c.MessagesSetInlineBotResults(nil); return err }},
		{"inline result send", func() error { _, err := c.MessagesSendInlineBotResult(nil); return err }},
		{"inline message edit", func() error { _, err := c.MessagesEditInlineBotMessage(nil); return err }},
		{"callback answer query", func() error { _, err := c.MessagesGetBotCallbackAnswer(nil); return err }},
		{"inline callback", func() error { _, err := c.MessagesSetBotCallbackAnswer(nil); return err }},
		{"requested peer", func() error { _, err := c.MessagesSendBotRequestedPeer(nil); return err }},
		{"guest chat result", func() error { _, err := c.MessagesSetBotGuestChatResult(nil); return err }},
		{"webview request", func() error { _, err := c.MessagesRequestWebView(nil); return err }},
		{"simple webview", func() error { _, err := c.MessagesRequestSimpleWebView413A3E73(nil); return err }},
		{"simple webview legacy", func() error { _, err := c.MessagesRequestSimpleWebView1A46500A(nil); return err }},
		{"simple webview URL", func() error { _, err := c.MessagesRequestSimpleWebView299BEC8E(nil); return err }},
		{"simple webview no URL", func() error { _, err := c.MessagesRequestSimpleWebView6ABB2F73(nil); return err }},
		{"webview result message", func() error { _, err := c.MessagesSendWebViewResultMessage(nil); return err }},
		{"webview data", func() error { _, err := c.MessagesSendWebViewData(nil); return err }},
		{"bot app lookup", func() error { _, err := c.MessagesGetBotApp(nil); return err }},
		{"app webview", func() error { _, err := c.MessagesRequestAppWebView53618BCE(nil); return err }},
		{"app webview legacy", func() error { _, err := c.MessagesRequestAppWebView8C5A3B3C(nil); return err }},
		{"chat join webview", func() error { _, err := c.MessagesRequestChatJoinWebView(nil); return err }},
		{"webview custom method", func() error { _, err := c.BotsInvokeWebViewCustomMethod(nil); return err }},
		{"download authorization", func() error { _, err := c.BotsCheckDownloadFileParams(nil); return err }},
		{"popular app bots", func() error { _, err := c.BotsGetPopularAppBots(nil); return err }},
		{"join chat results", func() error { _, err := c.BotsSetJoinChatResults(nil); return err }},
		{"legacy bot info languages", func() error { _, err := c.BotsGetBotInfo75EC12E6(nil); return err }},
	}
	for _, tc := range calls {
		if err := tc.call(); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			t.Errorf("%s: got %v, want AUTH_KEY_UNREGISTERED", tc.name, err)
		}
	}
}
