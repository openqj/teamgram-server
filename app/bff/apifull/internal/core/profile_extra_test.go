package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestProfileExtraUnauthed(t *testing.T) {
	c := &ApiFullCore{}
	if _, err := c.MessagesGetSavedGifs(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthed: got %v", err)
	}
}

func TestGifGetUser1(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	doc := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 42, AccessHash: 7}).To_InputDocument()
	if _, err := c.MessagesSaveGif(&mtproto.TLMessagesSaveGif{Id: doc}); err != nil {
		t.Fatal(err)
	}
	got, err := c.MessagesGetSavedGifs(&mtproto.TLMessagesGetSavedGifs{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.GetGifs()) == 0 || got.GetGifs()[0].GetId() != 42 {
		t.Fatalf("gif get: %+v", got)
	}
}
