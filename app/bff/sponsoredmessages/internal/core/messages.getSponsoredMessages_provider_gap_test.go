package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesGetSponsoredMessagesFailsClosedWithoutProvider(t *testing.T) {
	c := &SponsoredMessagesCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	got, err := c.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{Peer: peer})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("get sponsored messages = (%v, %v), want nil and METHOD_NOT_IMPL", got, err)
	}
}
