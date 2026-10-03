package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestUserDeletePeerSettingsPropagatesStorageError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "user:pass@tcp(127.0.0.1:1)/teamgram"})
	if err != nil {
		t.Fatalf("open test database handle: %v", err)
	}
	userDao := &dao.Dao{
		Mysql: &dao.Mysql{
			DB:                  db,
			UserPeerSettingsDAO: mysql_dao.NewUserPeerSettingsDAO(db),
		},
		CachedConn: sqlc.NewConnWithCache(db, nil),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := New(ctx, &svc.ServiceContext{Dao: userDao}).UserDeletePeerSettings(&user.TLUserDeletePeerSettings{
		UserId: 42, PeerType: mtproto.PEER_USER, PeerId: 84,
	})
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("UserDeletePeerSettings() = (%v, %v), want storage cancellation error", got, err)
	}
}
