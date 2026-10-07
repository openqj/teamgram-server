package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestBizRestAwayMessage(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	ok, err := c.AccountUpdateBusinessAwayMessage(&mtproto.TLAccountUpdateBusinessAwayMessage{
		Message: &mtproto.InputBusinessAwayMessage{ShortcutId: 3, OfflineOnly: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		t.Fatal("expected boolTrue")
	}
}

func TestBusinessGreetingMySQL(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 15}}
	ok, err := c.AccountUpdateBusinessGreetingMessage(&mtproto.TLAccountUpdateBusinessGreetingMessage{
		Message: &mtproto.InputBusinessGreetingMessage{
			PredicateName:  "hello-mysql",
			ShortcutId:     15,
			NoActivityDays: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok == nil || !mtproto.FromBool(ok) {
		t.Fatal("expected boolTrue")
	}
	got := businessGreetingMessage(15)
	if got == nil || got.GetPredicateName() != "hello-mysql" {
		t.Fatalf("greeting: %+v", got)
	}
}

func TestBusinessLocationClear(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 16}}
	if _, err := c.AccountUpdateBusinessLocation(&mtproto.TLAccountUpdateBusinessLocation{
		Address: mtproto.MakeFlagsString("temporary"),
	}); err != nil {
		t.Fatal(err)
	}
	if raw, err := persist.Default.Get(locationKey(16)); err != nil || raw == "" {
		t.Fatalf("location write: raw=%q err=%v", raw, err)
	}
	if _, err := c.AccountUpdateBusinessLocation(&mtproto.TLAccountUpdateBusinessLocation{}); err != nil {
		t.Fatal(err)
	}
	if raw, err := persist.Default.Get(locationKey(16)); err != nil || raw != "" {
		t.Fatalf("location clear: raw=%q err=%v", raw, err)
	}
}
