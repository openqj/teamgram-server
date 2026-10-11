package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestGiftChatThemeOffsetAcceptsLayer229CompatibilityField(t *testing.T) {
	got, err := giftChatThemeOffset(&mtproto.TLAccountGetUniqueGiftChatThemes{Offset_INT32: 7})
	if err != nil || got != 7 {
		t.Fatalf("integer offset = (%d, %v), want (7, nil)", got, err)
	}

	got, err = giftChatThemeOffset(&mtproto.TLAccountGetUniqueGiftChatThemes{Offset_STRING: "9", Offset_INT32: 7})
	if err != nil || got != 9 {
		t.Fatalf("string offset precedence = (%d, %v), want (9, nil)", got, err)
	}
}

func TestGiftChatThemeOffsetRejectsNegativeCompatibilityField(t *testing.T) {
	got, err := giftChatThemeOffset(&mtproto.TLAccountGetUniqueGiftChatThemes{Offset_INT32: -1})
	if got != 0 || !errors.Is(err, mtproto.ErrOffsetInvalid) {
		t.Fatalf("negative integer offset = (%d, %v), want (0, OFFSET_INVALID)", got, err)
	}
}

func TestGiftChatThemesProviderResultRequiresCanonicalUnion(t *testing.T) {
	valid := &mtproto.Account_ChatThemes{PredicateName: mtproto.Predicate_account_chatThemes}
	if !validGiftChatThemesProviderResult(valid) {
		t.Fatal("canonical account.chatThemes result was rejected")
	}
	notModified := &mtproto.Account_ChatThemes{PredicateName: mtproto.Predicate_account_chatThemesNotModified}
	if !validGiftChatThemesProviderResult(notModified) {
		t.Fatal("canonical account.chatThemesNotModified result was rejected")
	}
	for _, predicate := range []string{"", "updates", "account_chatThemesUnknown"} {
		if validGiftChatThemesProviderResult(&mtproto.Account_ChatThemes{PredicateName: predicate}) {
			t.Fatalf("provider predicate %q was accepted", predicate)
		}
	}
}

func TestUniqueGiftChatThemesRejectsUnknownProviderConstructor(t *testing.T) {
	core := newBusinessProviderGapCore(t, "account.getUniqueGiftChatThemes", map[string]any{
		"predicate_name": "updates",
	})
	result, err := core.AccountGetUniqueGiftChatThemes(&mtproto.TLAccountGetUniqueGiftChatThemes{Limit: 10})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("unknown provider constructor = (%v, %v), want nil and INTERNAL_SERVER_ERROR", result, err)
	}
}
