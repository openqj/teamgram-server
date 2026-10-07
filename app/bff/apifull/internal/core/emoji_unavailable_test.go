package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestEmojiKeywordsLanguagesReturnsSupportedSubset(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81023001}}

	languages, err := c.MessagesGetEmojiKeywordsLanguages(&mtproto.TLMessagesGetEmojiKeywordsLanguages{
		LangCodes: []string{"ru", "en", "en", "zh-hans"},
	})
	if err != nil {
		t.Fatalf("emoji languages: %v", err)
	}
	if got := len(languages.GetDatas()); got != 1 || languages.GetDatas()[0].GetLangCode() != "en" {
		t.Fatalf("emoji languages: got=%v, want only en", languages.GetDatas())
	}

	languages, err = c.MessagesGetEmojiKeywordsLanguages(nil)
	if err != nil || languages == nil || len(languages.GetDatas()) != 0 {
		t.Fatalf("emoji languages empty request: result=%v err=%v, want empty vector", languages, err)
	}

	url, err := c.MessagesGetEmojiURL(&mtproto.TLMessagesGetEmojiURL{})
	if url != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("emoji URL: result=%v err=%v, want nil result and METHOD_NOT_IMPL", url, err)
	}
}

func TestEmojiCatalogMethodsRequireAuthentication(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{}}

	if result, err := c.MessagesGetEmojiKeywordsLanguages(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("emoji languages: result=%v err=%v, want AUTH_KEY_UNREGISTERED", result, err)
	}
	if result, err := c.MessagesGetEmojiURL(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("emoji URL: result=%v err=%v, want AUTH_KEY_UNREGISTERED", result, err)
	}
}
