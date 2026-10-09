package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestHelpSetBotUpdatesStatusFailsClosed(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.HelpSetBotUpdatesStatus(nil); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("err = %v, want METHOD_NOT_IMPL", err)
	}
}
