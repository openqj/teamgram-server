package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBackgroundEmoji81010(t *testing.T) {
	anon := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	if _, err := anon.AccountUpdateColor(&mtproto.TLAccountUpdateColor{
		BackgroundEmojiId: wrapperspb.Int64(1),
	}); err == nil {
		t.Fatal("uid 0")
	}

	const uid int64 = 81010
	c, client := newAccentColorCore(uid)
	ok, err := c.AccountUpdateColor(&mtproto.TLAccountUpdateColor{
		BackgroundEmojiId: wrapperspb.Int64(uid),
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("update: %v %#v", err, ok)
	}
	if client.request == nil || client.request.GetUserId() != uid || client.request.GetBackgroundEmojiId() != uid {
		t.Fatalf("user color request: %+v", client.request)
	}
	list, err := c.AccountGetDefaultBackgroundEmojis(nil)
	if list != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("background emoji catalogue: %#v %v", list, err)
	}
	colors, err := c.HelpGetPeerColors(nil)
	if err != nil || len(colors.GetColors()) != len(builtinPeerColors) || colors.GetColors()[0].GetColorId() != builtinPeerColors[0].id {
		t.Fatalf("colors: %#v %v", colors, err)
	}
	profileColors, err := c.HelpGetPeerProfileColors(nil)
	if err != nil || len(profileColors.GetColors()) != len(builtinPeerColors) || profileColors.GetColors()[0].GetColorId() != builtinPeerColors[0].id {
		t.Fatalf("profile colors: %#v %v", profileColors, err)
	}
	if err = profileColors.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("profile colors encode: %v", err)
	}
}
