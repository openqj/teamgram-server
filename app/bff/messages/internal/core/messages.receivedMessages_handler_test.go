package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesReceivedMessagesRejectsInvalidInput(t *testing.T) {
	authenticated := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	for _, tc := range []struct {
		name string
		core *MessagesCore
		in   *mtproto.TLMessagesReceivedMessages
		want error
	}{
		{name: "nil core", want: mtproto.ErrAuthKeyUnregistered},
		{name: "missing metadata", core: &MessagesCore{}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: authenticated, want: mtproto.ErrInputConstructorInvalid},
		{name: "negative max id", core: authenticated, in: &mtproto.TLMessagesReceivedMessages{MaxId: -1}, want: mtproto.ErrMessageIdInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.core.MessagesReceivedMessages(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("MessagesReceivedMessages() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestMessagesReceivedMessagesFailsClosedWithoutProvider(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	for _, maxID := range []int32{0, 123} {
		got, err := core.MessagesReceivedMessages(&mtproto.TLMessagesReceivedMessages{MaxId: maxID})
		if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
			t.Fatalf("MessagesReceivedMessages(max_id=%d) = (%v, %v), want (nil, METHOD_NOT_IMPL)", maxID, got, err)
		}
	}
}
