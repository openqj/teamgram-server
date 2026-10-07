package core

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dao/mysql_dao"
	bizdao "github.com/teamgram/teamgram-server/app/service/biz/message/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/svc"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

func TestMessageGetSavedHistoryMessagesPropagatesDAOError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "root@unix(" + filepath.Join(t.TempDir(), "missing.sock") + ")/message_saved_history_error_test?timeout=100ms"})
	if err != nil {
		t.Fatalf("open lazy test connection: %v", err)
	}
	core := New(context.Background(), &svc.ServiceContext{Dao: &bizdao.Dao{Mysql: &bizdao.Mysql{
		DB:          db,
		MessagesDAO: mysql_dao.NewMessagesDAO(db, 1),
		CommonDAO:   sqlx.NewCommonDAO(db),
	}}})

	got, err := core.MessageGetSavedHistoryMessages(&messagepb.TLMessageGetSavedHistoryMessages{
		UserId:   41,
		PeerType: mtproto.PEER_USER,
		PeerId:   42,
		Limit:    20,
	})
	if got != nil || err == nil {
		t.Fatalf("MessageGetSavedHistoryMessages() = (%v, %v), want (nil, query error)", got, err)
	}
}

func TestMessageGetSavedHistoryMessagesRejectsInvalidRequests(t *testing.T) {
	core := New(context.Background(), &svc.ServiceContext{})
	if got, err := core.MessageGetSavedHistoryMessages(nil); got != nil || err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("nil request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
	}

	if got, err := core.MessageGetSavedHistoryMessages(&messagepb.TLMessageGetSavedHistoryMessages{Limit: -1}); got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("missing provider = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestMessageGetSavedHistoryMessagesRejectsChannelStorageGap(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "root@unix(" + filepath.Join(t.TempDir(), "missing.sock") + ")/message_saved_history_channel_test?timeout=100ms"})
	if err != nil {
		t.Fatalf("open lazy test connection: %v", err)
	}
	core := New(context.Background(), &svc.ServiceContext{Dao: &bizdao.Dao{Mysql: &bizdao.Mysql{
		DB:          db,
		MessagesDAO: mysql_dao.NewMessagesDAO(db, 1),
		CommonDAO:   sqlx.NewCommonDAO(db),
	}}})

	got, err := core.MessageGetSavedHistoryMessages(&messagepb.TLMessageGetSavedHistoryMessages{
		UserId:   41,
		PeerType: mtproto.PEER_CHANNEL,
		PeerId:   42,
		Limit:    20,
	})
	if got != nil || err != mtproto.ErrEnterpriseIsBlocked {
		t.Fatalf("channel saved history = (%v, %v), want (nil, ERR_ENTERPRISE_IS_BLOCKED)", got, err)
	}
}

func TestNormalizeSavedHistoryPeerUsesCanonicalSelfKey(t *testing.T) {
	const selfID int64 = 136907713

	for _, peerType := range []int32{mtproto.PEER_SELF, mtproto.PEER_USER} {
		got := normalizeSavedHistoryPeer(mtproto.MakePeerUtil(peerType, selfID), selfID)
		if got == nil || got.PeerType != mtproto.PEER_USER || got.PeerId != selfID {
			t.Fatalf("peer type %d normalized to %+v, want PEER_USER/%d", peerType, got, selfID)
		}
	}

	other := normalizeSavedHistoryPeer(mtproto.MakePeerUtil(mtproto.PEER_USER, 42), selfID)
	if other == nil || other.PeerType != mtproto.PEER_USER || other.PeerId != 42 {
		t.Fatalf("other user peer normalized to %+v, want PEER_USER/42", other)
	}
}
