package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestFactCheckRoundtrip(t *testing.T) {
	peer := &mtproto.InputPeer{UserId: 7}
	key := factStoreKey(1, peer, 42)
	if err := persist.Default.Set(key, ""); err != nil {
		t.Fatal(err)
	}

	anon := &ApiFullCore{}
	if _, err := anon.MessagesEditFactCheck(&mtproto.TLMessagesEditFactCheck{
		Peer: peer, MsgId: 42, Text: &mtproto.TextWithEntities{Text: "nope"},
	}); err == nil {
		t.Fatal("edit without auth")
	}
	if _, err := anon.MessagesDeleteFactCheck(&mtproto.TLMessagesDeleteFactCheck{
		Peer: peer, MsgId: 42,
	}); err == nil {
		t.Fatal("delete without auth")
	}
	if _, err := anon.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{
		Peer: peer, MsgId: []int32{42},
	}); err == nil {
		t.Fatal("get without auth")
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	text := &mtproto.TextWithEntities{Text: "checked"}
	if _, err := c.MessagesEditFactCheck(&mtproto.TLMessagesEditFactCheck{
		Peer: peer, MsgId: 42, Text: text,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := c.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{
		Peer: peer, MsgId: []int32{42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Datas) != 1 || got.Datas[0].GetText().GetText() != "checked" || got.Datas[0].GetNeedCheck() {
		t.Fatalf("get after edit: %#v", got)
	}

	if _, err = c.MessagesDeleteFactCheck(&mtproto.TLMessagesDeleteFactCheck{
		Peer: peer, MsgId: 42,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{
		Peer: peer, MsgId: []int32{42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Datas) != 1 || !got.Datas[0].GetNeedCheck() || got.Datas[0].GetText() != nil {
		t.Fatalf("get after delete: %#v", got)
	}
}
