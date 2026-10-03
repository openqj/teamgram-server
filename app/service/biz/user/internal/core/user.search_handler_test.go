package core

import (
	"context"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func newSearchDAOErrorCore(t *testing.T) *UserCore {
	t.Helper()
	ctx := context.Background()

	// Port 1 is intentionally unavailable. This exercises the actual DAO call
	// without contacting a configured database or any running Teamgram service.
	db, err := sqlx.Open(&sqlx.Config{
		DSN: "search:search@tcp(127.0.0.1:1)/teamgram_contacts_search_error_test?timeout=50ms&readTimeout=50ms&writeTimeout=50ms",
	})
	if err != nil {
		t.Fatalf("open lazy test database connection: %v", err)
	}

	return &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: &userdao.Dao{Mysql: &userdao.Mysql{
				UsersDAO:    mysql_dao.NewUsersDAO(db),
				UsernameDAO: mysql_dao.NewUsernameDAO(db),
			}},
		},
		Logger: logx.WithContext(ctx),
	}
}

func TestUserSearchPropagatesDAOError(t *testing.T) {
	core := newSearchDAOErrorCore(t)
	got, err := core.UserSearch(&userpb.TLUserSearch{
		Q:                "alice",
		ExcludedContacts: []int64{42},
		Limit:            10,
	})
	if err == nil || got != nil {
		t.Fatalf("UserSearch() = (%v, %v), want nil result and DAO error", got, err)
	}
}

func TestUserSearchUsernamePropagatesDAOError(t *testing.T) {
	core := newSearchDAOErrorCore(t)
	got, err := core.UserSearchUsername(&userpb.TLUserSearchUsername{
		Q:                "alice",
		ExcludedContacts: []int64{42},
		Limit:            10,
	})
	if err == nil || got != nil {
		t.Fatalf("UserSearchUsername() = (%v, %v), want nil result and DAO error", got, err)
	}
}
