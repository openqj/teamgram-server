package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesGetAllChatsRejectsInvalidRuntime(t *testing.T) {
	tests := []struct {
		name string
		core *ChatsCore
		in   *mtproto.TLMessagesGetAllChats
		want error
	}{
		{name: "missing authentication", core: &ChatsCore{}, in: &mtproto.TLMessagesGetAllChats{}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing dialog provider", core: &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}, in: &mtproto.TLMessagesGetAllChats{}, want: mtproto.ErrInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.core.MessagesGetAllChats(test.in)
			if got != nil || err != test.want {
				t.Fatalf("MessagesGetAllChats() = (%v, %v), want (nil, %v)", got, err, test.want)
			}
		})
	}
}
