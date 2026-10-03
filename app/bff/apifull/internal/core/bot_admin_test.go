package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestBotAdminStateRoundTrip81017(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81017}}
	var err error
	if _, err = c.MessagesToggleBotInAttachMenu(&mtproto.TLMessagesToggleBotInAttachMenu{
		Bot: &mtproto.InputUser{UserId: 81017},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := c.MessagesGetAttachMenuBot(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetBot().GetBotId() != 81017 {
		t.Fatalf("bot id %d", got.GetBot().GetBotId())
	}

	if got, err := c.BotsSetCustomVerification(&mtproto.TLBotsSetCustomVerification{}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("custom verification: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
}
