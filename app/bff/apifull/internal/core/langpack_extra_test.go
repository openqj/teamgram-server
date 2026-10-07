package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestLangpackUnavailableFailsClosed(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81013}}
	checks := []func() (any, error){
		func() (any, error) { return c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangCode: "en"}) },
		func() (any, error) {
			return c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangCode: "en", Keys: []string{"settings"}})
		},
		func() (any, error) { return c.LangpackGetDifference(&mtproto.TLLangpackGetDifference{LangCode: "en"}) },
		func() (any, error) { return c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{}) },
		func() (any, error) { return c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangCode: "en"}) },
	}
	for i, check := range checks {
		got, err := check()
		if !nilRPCResult(got) || !sameRPCErrorCode(err, mtproto.ErrMethodNotImpl) {
			t.Fatalf("check %d=(%#v,%v), want METHOD_NOT_IMPL", i, got, err)
		}
	}
}

func TestLangpackRequiresAuth(t *testing.T) {
	if got, err := (&ApiFullCore{}).LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{}); got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthenticated=(%#v,%v)", got, err)
	}
}

func TestEmbeddedEnglishLangpack(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81013}}
	languages, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "weba"})
	if err != nil || languages == nil || len(languages.GetDatas()) != 1 {
		t.Fatalf("languages=(%#v,%v)", languages, err)
	}
	if got := languages.GetDatas()[0]; got.GetLangCode() != "en" || got.GetStringsCount() < 3000 {
		t.Fatalf("language=%#v", got)
	}
	strings, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{
		LangPack: "weba",
		LangCode: "en",
		Keys:     []string{"SendMessage", "Delete", "SendMessage", "Missing"},
	})
	if err != nil || strings == nil || len(strings.GetDatas()) != 2 {
		t.Fatalf("strings=(%#v,%v)", strings, err)
	}
	if strings.GetDatas()[0].GetKey() != "SendMessage" || strings.GetDatas()[0].GetValue() != "Send Message" {
		t.Fatalf("send message=%#v", strings.GetDatas()[0])
	}
	difference, err := c.LangpackGetDifference(&mtproto.TLLangpackGetDifference{LangPack: "weba", LangCode: "en", FromVersion: 1})
	if err != nil || difference == nil || difference.GetVersion() != 1 || len(difference.GetStrings()) != 0 {
		t.Fatalf("not modified difference=(%#v,%v)", difference, err)
	}
	difference, err = c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "weba", LangCode: "en"})
	if err != nil || difference == nil || len(difference.GetStrings()) < 3000 {
		t.Fatalf("full difference=(%#v,%v)", difference, err)
	}
}

func TestEmbeddedEnglishLangpackRejectsUnsupportedLanguage(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81013}}
	if got, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "weba", LangCode: "zh-hans"}); got != nil || !sameRPCErrorCode(err, mtproto.ErrLangCodeNotSupported) {
		t.Fatalf("unsupported language=(%#v,%v)", got, err)
	}
	if got, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "unsupported"}); got != nil || !sameRPCErrorCode(err, mtproto.ErrLangPackInvalid) {
		t.Fatalf("invalid langpack=(%#v,%v)", got, err)
	}
}
