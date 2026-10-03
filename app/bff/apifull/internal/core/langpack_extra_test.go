package core

import (
	"errors"
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
		if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
			t.Fatalf("check %d=(%#v,%v), want METHOD_NOT_IMPL", i, got, err)
		}
	}
}

func TestLangpackRequiresAuth(t *testing.T) {
	if got, err := (&ApiFullCore{}).LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{}); got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthenticated=(%#v,%v)", got, err)
	}
}
