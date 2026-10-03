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

func TestMessageGetHistoryMessagesPropagatesDAOError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "root@unix(" + filepath.Join(t.TempDir(), "missing.sock") + ")/message_history_error_test?timeout=100ms"})
	if err != nil {
		t.Fatalf("open lazy test connection: %v", err)
	}
	core := New(context.Background(), &svc.ServiceContext{Dao: &bizdao.Dao{Mysql: &bizdao.Mysql{
		DB:          db,
		MessagesDAO: mysql_dao.NewMessagesDAO(db, 1),
	}}})

	got, err := core.MessageGetHistoryMessages(&messagepb.TLMessageGetHistoryMessages{
		UserId:   41,
		PeerType: mtproto.PEER_USER,
		PeerId:   42,
		Limit:    20,
	})
	if got != nil || err == nil {
		t.Fatalf("MessageGetHistoryMessages() = (%v, %v), want (nil, query error)", got, err)
	}
}
