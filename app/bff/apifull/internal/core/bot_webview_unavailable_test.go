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
		{"inline callback", func() error { _, err := c.MessagesSetBotCallbackAnswer(nil); return err }},
		{"webview request", func() error { _, err := c.MessagesRequestWebView(nil); return err }},
		{"webview prolong", func() error { _, err := c.MessagesProlongWebView(nil); return err }},
		{"webview data", func() error { _, err := c.MessagesSendWebViewData(nil); return err }},
		{"webview custom method", func() error { _, err := c.BotsInvokeWebViewCustomMethod(nil); return err }},
		{"webview button", func() error { _, err := c.BotsRequestWebViewButton(nil); return err }},
		{"main webview", func() error { _, err := c.MessagesRequestMainWebView(nil); return err }},
		{"popular app bots", func() error { _, err := c.BotsGetPopularAppBots(nil); return err }},
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
		if err := tc.call(); !errors.Is(err, mtproto.ErrMethodNotImpl) {
			t.Errorf("%s: got %v, want METHOD_NOT_IMPL", tc.name, err)
		}
	}
	if store.reads != 0 || store.writes != 0 {
		t.Fatalf("unsupported methods touched persistence: reads=%d writes=%d", store.reads, store.writes)
	}
}

func TestBotAndWebViewUnavailableAuthenticatesBeforeMethodCheck(t *testing.T) {
	c := &ApiFullCore{}
	if got, err := c.MessagesRequestWebView(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("request webview: got=%v err=%v, want AUTH_KEY_UNREGISTERED", got, err)
	}
	if got, err := c.MessagesSetInlineBotResults(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("set inline results: got=%v err=%v, want AUTH_KEY_UNREGISTERED", got, err)
	}
	if got, err := c.BotsGetPopularAppBots(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("popular app bots: got=%v err=%v, want AUTH_KEY_UNREGISTERED", got, err)
	}
}
