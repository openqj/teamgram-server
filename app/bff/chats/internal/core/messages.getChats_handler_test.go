package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesGetChatsRejectsInvalidRuntime(t *testing.T) {
	tests := []struct {
		name string
		core *ChatsCore
		in   *mtproto.TLMessagesGetChats
		want error
	}{
		{name: "missing authentication", core: &ChatsCore{}, in: &mtproto.TLMessagesGetChats{}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing chat provider", core: &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}, in: &mtproto.TLMessagesGetChats{}, want: mtproto.ErrInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.core.MessagesGetChats(test.in)
			if got != nil || err != test.want {
				t.Fatalf("MessagesGetChats() = (%v, %v), want (nil, %v)", got, err, test.want)
			}
		})
	}
}
