package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
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
