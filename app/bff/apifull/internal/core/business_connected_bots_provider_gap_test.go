package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAccountGetBotBusinessConnectionFailsClosedWithoutProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	got, err := c.AccountGetBotBusinessConnection(&mtproto.TLAccountGetBotBusinessConnection{ConnectionId: "connection-42"})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("get bot business connection = (%v, %v), want nil and METHOD_NOT_IMPL", got, err)
	}
}
