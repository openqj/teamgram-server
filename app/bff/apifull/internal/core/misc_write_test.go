package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMiscWriteNilError(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.HelpSetBotUpdatesStatus(nil); err != nil {
		t.Fatal(err)
	}
}
