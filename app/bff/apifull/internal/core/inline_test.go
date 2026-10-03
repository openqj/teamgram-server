package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestInlineBotUnauthed(t *testing.T) {
	c := &ApiFullCore{}
	_, err := c.MessagesGetInlineBotResults(nil)
	if err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthed: got %v", err)
	}
}

func TestInlineBotSetResultsUnavailable(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.MessagesSetInlineBotResults(nil); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("set results: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
}
