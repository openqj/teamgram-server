package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestGiftPreferenceHandlersValidateLocalScope(t *testing.T) {
	uid := int64(902801)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if result, err := c.PaymentsToggleChatStarGiftNotifications(&mtproto.TLPaymentsToggleChatStarGiftNotifications{}); result != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("missing notification peer = (%#v,%v)", result, err)
	}
	other := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid + 1}).To_InputPeer()
	if result, err := c.PaymentsToggleStarGiftsPinnedToTop(&mtproto.TLPaymentsToggleStarGiftsPinnedToTop{Peer: other}); result != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("foreign pin peer = (%#v,%v), want PEER_ID_INVALID", result, err)
	}
	foreign := mtproto.MakeTLInputSavedStarGiftUser(&mtproto.InputSavedStarGift{}).To_InputSavedStarGift()
	self := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	if result, err := c.PaymentsToggleStarGiftsPinnedToTop(&mtproto.TLPaymentsToggleStarGiftsPinnedToTop{Peer: self, Stargift: []*mtproto.InputSavedStarGift{foreign}}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("unsupported gift reference = (%#v,%v), want METHOD_NOT_IMPL", result, err)
	}
}
