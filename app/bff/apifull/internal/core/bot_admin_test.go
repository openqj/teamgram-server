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
