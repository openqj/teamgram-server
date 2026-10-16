package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
)

type receivedMessagesStoreStub struct {
	userID int64
	maxID  int32
	called bool
	err    error
}

func (s *receivedMessagesStoreStub) Record(_ context.Context, userID int64, maxID int32) error {
	s.called = true
	s.userID, s.maxID = userID, maxID
	return s.err
}

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

func TestMessagesReceivedMessagesDoesNotRecordCursorWithoutNotificationProvider(t *testing.T) {
	store := &receivedMessagesStoreStub{}
	core := &MessagesCore{
		MD:     &metadata.RpcMetadata{UserId: 42},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ReceivedMessages: store}},
	}
	got, err := core.MessagesReceivedMessages(&mtproto.TLMessagesReceivedMessages{MaxId: 123})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("MessagesReceivedMessages() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
	if store.called {
		t.Fatalf("received cursor was recorded without a notification cancellation provider: (%d, %d)", store.userID, store.maxID)
	}
}
