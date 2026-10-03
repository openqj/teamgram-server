package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestUserGetChannelUsernamePropagatesDAOError(t *testing.T) {
	// Open is lazy; the canceled context below prevents any network connection.
	db, err := sqlx.Open(&sqlx.Config{DSN: "user:pass@tcp(127.0.0.1:1)/teamgram"})
	if err != nil {
		t.Fatalf("open test database handle: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	core := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
		UsernameDAO: mysql_dao.NewUsernameDAO(db),
	}}})

	got, err := core.UserGetChannelUsername(&userpb.TLUserGetChannelUsername{ChannelId: 42})
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("UserGetChannelUsername() = (%v, %v), want storage cancellation error", got, err)
	}
}
