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
	err    error
}

func (s *receivedMessagesStoreStub) Record(_ context.Context, userID int64, maxID int32) error {
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

func TestMessagesReceivedMessagesPersistsCursor(t *testing.T) {
	store := &receivedMessagesStoreStub{}
	core := &MessagesCore{
		MD:     &metadata.RpcMetadata{UserId: 42},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ReceivedMessages: store}},
	}
	got, err := core.MessagesReceivedMessages(&mtproto.TLMessagesReceivedMessages{MaxId: 123})
	if err != nil || got == nil || len(got.GetDatas()) != 0 {
		t.Fatalf("MessagesReceivedMessages() = (%v, %v), want empty vector", got, err)
	}
	if store.userID != 42 || store.maxID != 123 {
		t.Fatalf("received cursor = (%d, %d), want (42, 123)", store.userID, store.maxID)
	}
}

func TestMessagesReceivedMessagesPropagatesStoreError(t *testing.T) {
	wantErr := errors.New("postgres unavailable")
	core := &MessagesCore{
		MD:     &metadata.RpcMetadata{UserId: 42},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ReceivedMessages: &receivedMessagesStoreStub{err: wantErr}}},
	}
	got, err := core.MessagesReceivedMessages(&mtproto.TLMessagesReceivedMessages{MaxId: 123})
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("MessagesReceivedMessages() = (%v, %v), want store error", got, err)
	}
}
