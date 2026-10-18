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

type receivedMessagesProviderStub struct {
	receivedMessagesStoreStub
	rows        []dao.ReceivedNotification
	ackUserID   int64
	ackMaxID    int32
	acknowledge bool
	err         error
}

func (s *receivedMessagesProviderStub) RecordNotification(_ context.Context, _ int64, _, _ int32) error {
	return s.err
}

func (s *receivedMessagesProviderStub) Acknowledge(_ context.Context, userID int64, maxID int32) ([]dao.ReceivedNotification, error) {
	s.acknowledge = true
	s.ackUserID, s.ackMaxID = userID, maxID
	return s.rows, s.err
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

func TestMessagesReceivedMessagesAcknowledgesTypedNotifications(t *testing.T) {
	provider := &receivedMessagesProviderStub{rows: []dao.ReceivedNotification{{ID: 17, Flags: 0}, {ID: 23, Flags: 4}}}
	core := &MessagesCore{
		MD:     &metadata.RpcMetadata{UserId: 42},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ReceivedMessages: provider}},
	}
	got, err := core.MessagesReceivedMessages(&mtproto.TLMessagesReceivedMessages{MaxId: 23})
	if err != nil || got == nil || len(got.GetDatas()) != 2 {
		t.Fatalf("MessagesReceivedMessages() = (%v, %v), want two typed rows", got, err)
	}
	if !provider.acknowledge || provider.ackUserID != 42 || provider.ackMaxID != 23 {
		t.Fatalf("acknowledge = (%v, %d, %d), want (true, 42, 23)", provider.acknowledge, provider.ackUserID, provider.ackMaxID)
	}
	if got.GetDatas()[0].GetId() != 17 || got.GetDatas()[1].GetFlags() != 4 {
		t.Fatalf("typed rows = %+v, want ids/flags 17/0 and 23/4", got.GetDatas())
	}
}

func TestMessagesReceivedMessagesMapsProviderFailureToInternalError(t *testing.T) {
	provider := &receivedMessagesProviderStub{err: errors.New("database unavailable")}
	core := &MessagesCore{
		MD:     &metadata.RpcMetadata{UserId: 42},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ReceivedMessages: provider}},
	}
	got, err := core.MessagesReceivedMessages(&mtproto.TLMessagesReceivedMessages{MaxId: 23})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesReceivedMessages() = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}
