package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestLangpackRejectsUnsupportedPack(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81013}}
	checks := []func() (any, error){
		func() (any, error) {
			return c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "unsupported", LangCode: "en"})
		},
		func() (any, error) {
			return c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "unsupported", LangCode: "en", Keys: []string{"settings"}})
		},
		func() (any, error) {
			return c.LangpackGetDifference(&mtproto.TLLangpackGetDifference{LangPack: "unsupported", LangCode: "en"})
		},
		func() (any, error) {
			return c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "unsupported"})
		},
		func() (any, error) {
			return c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "unsupported", LangCode: "en"})
		},
	}
	for i, check := range checks {
		got, err := check()
		if !nilRPCResult(got) || !sameRPCErrorCode(err, mtproto.ErrLangPackInvalid) {
			t.Fatalf("check %d=(%#v,%v), want LANG_PACK_INVALID", i, got, err)
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
	if err != nil || languages == nil || len(languages.GetDatas()) != 18 {
		t.Fatalf("languages=(%#v,%v)", languages, err)
	}
	if got := languages.GetDatas()[0]; got.GetLangCode() != "en" || got.GetStringsCount() < 3000 || got.GetTranslatedCount() < 3000 {
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

func TestEmbeddedLanguagePackSupportsMultipleLanguages(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81013}}
	if got, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "weba", LangCode: "zh-hans"}); err != nil || got == nil || got.GetLangCode() != "zh-hans" || got.GetNativeName() != "简体中文" {
		t.Fatalf("simplified Chinese=(%#v,%v)", got, err)
	}
	strings, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{
		LangPack: "weba",
		LangCode: "zh-hans",
		Keys:     []string{"SendMessage", "Delete"},
	})
	if err != nil || strings == nil || len(strings.GetDatas()) != 2 || strings.GetDatas()[0].GetValue() != "发送消息" {
		t.Fatalf("simplified Chinese strings=(%#v,%v)", strings, err)
	}
	if got, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "weba", LangCode: "xx"}); got != nil || !sameRPCErrorCode(err, mtproto.ErrLangCodeNotSupported) {
		t.Fatalf("unsupported language=(%#v,%v)", got, err)
	}
	if got, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "unsupported"}); got != nil || !sameRPCErrorCode(err, mtproto.ErrLangPackInvalid) {
		t.Fatalf("invalid langpack=(%#v,%v)", got, err)
	}
}

func TestEmbeddedArabicLanguagePackAcceptsArrayPluralValues(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81013}}
	strings, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{
		LangPack: "weba",
		LangCode: "ar",
		Keys:     []string{"Subscribers"},
	})
	if err != nil || strings == nil || len(strings.GetDatas()) != 1 {
		t.Fatalf("arabic Subscribers=(%#v,%v)", strings, err)
	}
	value := strings.GetDatas()[0]
	if value.GetKey() != "Subscribers" || value.GetPredicateName() != mtproto.Predicate_langPackStringPluralized || value.GetOtherValue() != "مشتركٍ" {
		t.Fatalf("arabic Subscribers value=%#v", value)
	}
}
