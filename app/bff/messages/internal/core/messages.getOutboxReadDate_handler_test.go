package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesGetOutboxReadDateFailsClosedWithoutProviders(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	request := &mtproto.TLMessagesGetOutboxReadDate{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 11}).To_InputPeer(),
		MsgId: 9,
	}

	got, err := core.MessagesGetOutboxReadDate(request)
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetOutboxReadDate() = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}
