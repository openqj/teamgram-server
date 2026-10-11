package core

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestUniqueGiftChatThemesUsePublishedPostgresGifts(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	cleanup, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup.Close() })
	t.Cleanup(func() { _ = domain.Close() })

	owner := time.Now().UnixNano()
	slug := fmt.Sprintf("theme-gift-%d", owner)
	if err = domain.SaveGift(owner+1, owner, slug, 42); err != nil {
		t.Fatal(err)
	}
	var giftID int64
	if err = cleanup.QueryRow(`SELECT id FROM apifull_gift WHERE slug=$1`, slug).Scan(&giftID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = cleanup.Exec(`DELETE FROM apifull_gift WHERE id=$1`, giftID)
	})
	if err = domain.SetGiftChatThemeAvailable(giftID, true); err != nil {
		t.Fatal(err)
	}

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	first, err := core.AccountGetUniqueGiftChatThemes(&mtproto.TLAccountGetUniqueGiftChatThemes{Limit: 10})
	if err != nil || first == nil || len(first.GetThemes()) != 1 {
		t.Fatalf("catalog result = (%#v, %v)", first, err)
	}
	theme := first.GetThemes()[0]
	if theme == nil || theme.GetPredicateName() != mtproto.Predicate_chatThemeUniqueGift || theme.GetGift() == nil || theme.GetGift().GetSlug() != slug {
		t.Fatalf("theme = %#v, want published gift %q", theme, slug)
	}
	if first.GetHash() == 0 {
		t.Fatalf("catalog hash is zero: %#v", first)
	}
	notModified, err := core.AccountGetUniqueGiftChatThemes(&mtproto.TLAccountGetUniqueGiftChatThemes{Limit: 10, Hash: first.GetHash()})
	if err != nil || notModified == nil || notModified.GetPredicateName() != mtproto.Predicate_account_chatThemesNotModified {
		t.Fatalf("hash result = (%#v, %v), want chatThemesNotModified", notModified, err)
	}
	if got, err := core.AccountGetUniqueGiftChatThemes(&mtproto.TLAccountGetUniqueGiftChatThemes{Offset_STRING: "bad", Limit: 10}); got != nil || !errors.Is(err, mtproto.ErrOffsetInvalid) {
		t.Fatalf("invalid offset = (%#v, %v), want OFFSET_INVALID", got, err)
	}
	if err = domain.SetGiftChatThemeAvailable(giftID, false); err != nil {
		t.Fatal(err)
	}
	empty, err := core.AccountGetUniqueGiftChatThemes(&mtproto.TLAccountGetUniqueGiftChatThemes{Limit: 10})
	if err != nil || empty == nil || len(empty.GetThemes()) != 0 {
		t.Fatalf("cleared catalog = (%#v, %v)", empty, err)
	}
}
