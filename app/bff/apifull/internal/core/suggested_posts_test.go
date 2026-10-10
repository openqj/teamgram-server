package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesToggleSuggestedPostApprovalFailsClosedWithoutProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1001}}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 42, AccessHash: 99}).To_InputPeer()
	got, err := c.MessagesToggleSuggestedPostApproval(&mtproto.TLMessagesToggleSuggestedPostApproval{Peer: peer, MsgId: 7})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("result = (%v, %v), want nil and METHOD_NOT_IMPL", got, err)
	}
}

func TestMessagesToggleSuggestedPostApprovalValidatesInputBeforeProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1001}}
	for name, in := range map[string]*mtproto.TLMessagesToggleSuggestedPostApproval{
		"nil request":    nil,
		"missing peer":   {MsgId: 7},
		"empty peer":     {Peer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(), MsgId: 7},
		"missing msg id": {Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 42}).To_InputPeer(), MsgId: 0},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := c.MessagesToggleSuggestedPostApproval(in)
			if got != nil || err == nil {
				t.Fatalf("result = (%v, %v), want validation error", got, err)
			}
			if name == "missing msg id" && !errors.Is(err, mtproto.ErrMsgIdInvalid) {
				t.Fatalf("error = %v, want MSG_ID_INVALID", err)
			}
			if name != "missing msg id" && !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("error = %v, want PEER_ID_INVALID", err)
			}
		})
	}
}

func TestMessagesToggleSuggestedPostApprovalRequiresAuth(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	got, err := c.MessagesToggleSuggestedPostApproval(nil)
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("result = (%v, %v), want nil and AUTH_KEY_UNREGISTERED", got, err)
	}
}
