package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestBotAdminStateRoundTrip81017(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81017}}
	if got, err := c.MessagesToggleBotInAttachMenu(&mtproto.TLMessagesToggleBotInAttachMenu{
		Bot: &mtproto.InputUser{UserId: 81017},
	}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("attach menu without user provider: got=%v err=%v, want USER_ID_INVALID", got, err)
	}

	if got, err := c.BotsSetCustomVerification(&mtproto.TLBotsSetCustomVerification{}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("custom verification: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
}

func TestCustomVerificationPeerCanonicalizesSelf(t *testing.T) {
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	typ, id, err := customVerificationPeer(81017, peer)
	if err != nil || typ != mtproto.PEER_SELF || id != 81017 {
		t.Fatalf("customVerificationPeer(self) = (%d, %d, %v)", typ, id, err)
	}

	invalid := mtproto.MakeTLInputPeerUsername(&mtproto.InputPeer{Username: "@bot"}).To_InputPeer()
	if _, _, err = customVerificationPeer(81017, invalid); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("customVerificationPeer(username) error = %v, want PEER_ID_INVALID", err)
	}
}
