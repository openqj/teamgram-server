package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

func TestMessagesSaveDefaultSendAsRejectsUnauthenticatedAndNilRequest(t *testing.T) {
	core := &MessagesCore{}
	if _, err := core.MessagesSaveDefaultSendAs(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("nil core metadata error = %v, want AUTH_KEY_UNREGISTERED", err)
	}

	core.MD = &metadata.RpcMetadata{UserId: 972341}
	if _, err := core.MessagesSaveDefaultSendAs(nil); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("nil request error = %v, want PEER_ID_INVALID", err)
	}
}

func TestMessagesSaveDefaultSendAsPersistsPreference(t *testing.T) {
	const userID int64 = 972341
	const peerID int64 = 972342
	const sendAsID int64 = 972343

	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: userID}}
	request := &mtproto.TLMessagesSaveDefaultSendAs{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID}).To_InputPeer(),
		SendAs: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: sendAsID}).To_InputPeer(),
	}
	got, err := core.MessagesSaveDefaultSendAs(request)
	if err != nil {
		t.Fatalf("MessagesSaveDefaultSendAs() error = %v", err)
	}
	if got != mtproto.BoolTrue {
		t.Fatalf("MessagesSaveDefaultSendAs() = %v, want BoolTrue", got)
	}

	value, err := persist.Default.Get("default_send_as:972341:2:972342")
	if err != nil {
		t.Fatalf("persisted preference read error = %v", err)
	}
	if value != "2:972343" {
		t.Fatalf("persisted preference = %q, want 2:972343", value)
	}
}
