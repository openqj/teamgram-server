package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestRingtoneSaveGetRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	doc := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 42, AccessHash: 7}).To_InputDocument()
	if _, err := c.AccountSaveRingtone(&mtproto.TLAccountSaveRingtone{Id: doc, Unsave: mtproto.BoolFalse}); err != nil {
		t.Fatal(err)
	}
	got, err := c.AccountGetSavedRingtones(&mtproto.TLAccountGetSavedRingtones{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.GetRingtones()) == 0 || got.GetRingtones()[0].GetId() != 42 || got.GetRingtones()[0].GetAccessHash() != 7 {
		t.Fatalf("ringtone get: %+v", got)
	}
}
