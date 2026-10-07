package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestWebBrowserSettingsRoundTrip(t *testing.T) {
	const userID int64 = 81023091
	c := &WebBrowserCore{MD: &metadata.RpcMetadata{UserId: userID}}
	_ = saveWebBrowser(userID, webBrowserState{})
	t.Cleanup(func() { _ = saveWebBrowser(userID, webBrowserState{}) })

	settings, err := c.AccountUpdateWebBrowserSettings(&mtproto.TLAccountUpdateWebBrowserSettings{
		OpenExternalBrowser: true,
		DisplayCloseButton:  true,
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if !settings.GetOpenExternalBrowser() || !settings.GetDisplayCloseButton() || settings.GetHash() == 0 {
		t.Fatalf("update settings: got=%v", settings)
	}

	notModified, err := c.AccountGetWebBrowserSettings(&mtproto.TLAccountGetWebBrowserSettings{Hash: settings.GetHash()})
	if err != nil {
		t.Fatalf("get unchanged settings: %v", err)
	}
	if notModified.GetPredicateName() != "account_webBrowserSettingsNotModified" {
		t.Fatalf("get unchanged settings: predicate=%q", notModified.GetPredicateName())
	}

	if _, err = c.AccountToggleWebBrowserSettingsException60ED4229(&mtproto.TLAccountToggleWebBrowserSettingsException60ED4229{
		OpenExternalBrowser: mtproto.BoolTrue,
		Url:                 "https://example.com/path",
	}); err != nil {
		t.Fatalf("add external exception: %v", err)
	}
	if _, err = c.AccountToggleWebBrowserSettingsException60ED4229(&mtproto.TLAccountToggleWebBrowserSettingsException60ED4229{
		OpenExternalBrowser: mtproto.BoolFalse,
		Url:                 "https://inapp.example.test",
	}); err != nil {
		t.Fatalf("add in-app exception: %v", err)
	}

	settings, err = c.AccountGetWebBrowserSettings(nil)
	if err != nil || len(settings.GetExternalExceptions()) != 1 || len(settings.GetInappExceptions()) != 1 {
		t.Fatalf("get exceptions: settings=%v err=%v", settings, err)
	}
	if _, err = c.AccountToggleWebBrowserSettingsException60ED4229(&mtproto.TLAccountToggleWebBrowserSettingsException60ED4229{
		Delete:              true,
		OpenExternalBrowser: mtproto.BoolTrue,
		Url:                 "https://example.com/path",
	}); err != nil {
		t.Fatalf("delete external exception: %v", err)
	}

	settings, err = c.AccountDeleteWebBrowserSettingsExceptions(&mtproto.TLAccountDeleteWebBrowserSettingsExceptions{})
	if err != nil || len(settings.GetExternalExceptions()) != 0 || len(settings.GetInappExceptions()) != 0 {
		t.Fatalf("clear exceptions: settings=%v err=%v", settings, err)
	}
}

func TestWebBrowserSettingsRequireAuthentication(t *testing.T) {
	c := &WebBrowserCore{MD: &metadata.RpcMetadata{}}
	if result, err := c.AccountGetWebBrowserSettings(nil); result != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("get settings: result=%v err=%v, want USER_ID_INVALID", result, err)
	}
	if result, err := c.AccountToggleWebBrowserSettingsException60ED4229(nil); result != nil || !errors.Is(err, mtproto.ErrUrlInvalid) {
		t.Fatalf("toggle nil: result=%v err=%v, want URL_INVALID", result, err)
	}
}
