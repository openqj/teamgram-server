package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dao/mysql_dao"
	bizdao "github.com/teamgram/teamgram-server/app/service/biz/message/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/svc"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

func newCountHandlerErrorCore(t *testing.T) *MessageCore {
	t.Helper()
	db, err := sqlx.Open(&sqlx.Config{DSN: "root@unix(" + filepath.Join(t.TempDir(), "missing.sock") + ")/message_count_error_test?timeout=100ms"})
	if err != nil {
		t.Fatalf("open lazy test connection: %v", err)
	}
	return New(context.Background(), &svc.ServiceContext{Dao: &bizdao.Dao{Mysql: &bizdao.Mysql{
		DB:          db,
		MessagesDAO: mysql_dao.NewMessagesDAO(db, 1),
	}}})
}

func TestMessageGetHistoryMessagesCountPropagatesDAOError(t *testing.T) {
	core := newCountHandlerErrorCore(t)
	got, err := core.MessageGetHistoryMessagesCount(&messagepb.TLMessageGetHistoryMessagesCount{
		UserId:   41,
		PeerType: mtproto.PEER_USER,
		PeerId:   42,
	})
	if got != nil || err == nil {
		t.Fatalf("MessageGetHistoryMessagesCount() = (%v, %v), want (nil, query error)", got, err)
	}
}

func TestMessageGetUnreadMentionsCountPropagatesDAOError(t *testing.T) {
	core := newCountHandlerErrorCore(t)
	got, err := core.MessageGetUnreadMentionsCount(&messagepb.TLMessageGetUnreadMentionsCount{
		UserId:   41,
		PeerType: mtproto.PEER_CHAT,
		PeerId:   42,
	})
	if got != nil || err == nil {
		t.Fatalf("MessageGetUnreadMentionsCount() = (%v, %v), want (nil, query error)", got, err)
	}
}

func TestMessageCountHandlersRejectInvalidRequests(t *testing.T) {
	core := New(context.Background(), &svc.ServiceContext{Dao: &bizdao.Dao{}})
	tests := []struct {
		name string
		call func() (any, error)
	}{
		{
			name: "history nil",
			call: func() (any, error) { return core.MessageGetHistoryMessagesCount(nil) },
		},
		{
			name: "mentions nil",
			call: func() (any, error) { return core.MessageGetUnreadMentionsCount(nil) },
		},
		{
			name: "history missing user",
			call: func() (any, error) {
				return core.MessageGetHistoryMessagesCount(&messagepb.TLMessageGetHistoryMessagesCount{PeerType: mtproto.PEER_USER, PeerId: 42})
			},
		},
		{
			name: "mentions missing peer",
			call: func() (any, error) {
				return core.MessageGetUnreadMentionsCount(&messagepb.TLMessageGetUnreadMentionsCount{UserId: 41, PeerType: mtproto.PEER_CHAT})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.call()
			if !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("call() = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
			}
		})
	}
}
