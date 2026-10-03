package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func newAccountTTLDAOErrorCore(t *testing.T) *UserCore {
	t.Helper()
	ctx := context.Background()
	db, err := sqlx.Open(&sqlx.Config{
		DSN: "ttl:ttl@tcp(127.0.0.1:1)/teamgram_ttl_error_test?timeout=50ms&readTimeout=50ms&writeTimeout=50ms",
	})
	if err != nil {
		t.Fatalf("open lazy test database connection: %v", err)
	}
	return &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			UsersDAO: mysql_dao.NewUsersDAO(db),
		}}},
		Logger: logx.WithContext(ctx),
	}
}

func TestUserSetAccountDaysTTLPropagatesStorageError(t *testing.T) {
	core := newAccountTTLDAOErrorCore(t)
	got, err := core.UserSetAccountDaysTTL(&userpb.TLUserSetAccountDaysTTL{
		UserId: 42,
		Ttl:    180,
	})
	if got != nil || err == nil {
		t.Fatalf("UserSetAccountDaysTTL() = (%v, %v), want nil result and storage error", got, err)
	}
}

func TestUserGetAccountDaysTTLPropagatesStorageError(t *testing.T) {
	core := newAccountTTLDAOErrorCore(t)
	got, err := core.UserGetAccountDaysTTL(&userpb.TLUserGetAccountDaysTTL{UserId: 42})
	if got != nil || err == nil {
		t.Fatalf("UserGetAccountDaysTTL() = (%v, %v), want nil result and storage error", got, err)
	}
	if errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("UserGetAccountDaysTTL() error = %v, want underlying storage error rather than USER_ID_INVALID", err)
	}
}

func TestUserSetAuthorizationTTLPropagatesStorageError(t *testing.T) {
	core := newAccountTTLDAOErrorCore(t)
	got, err := core.UserSetAuthorizationTTL(&userpb.TLUserSetAuthorizationTTL{UserId: 42, Ttl: 30})
	if got != nil || err == nil {
		t.Fatalf("UserSetAuthorizationTTL() = (%v, %v), want nil result and storage error", got, err)
	}
}

func TestUserGetAuthorizationTTLPropagatesStorageError(t *testing.T) {
	core := newAccountTTLDAOErrorCore(t)
	got, err := core.UserGetAuthorizationTTL(&userpb.TLUserGetAuthorizationTTL{UserId: 42})
	if got != nil || err == nil {
		t.Fatalf("UserGetAuthorizationTTL() = (%v, %v), want nil result and storage error", got, err)
	}
	if errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("UserGetAuthorizationTTL() error = %v, want underlying storage error rather than USER_ID_INVALID", err)
	}
}

func TestUserAuthorizationTTLRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN must point to the isolated teamgram_audit database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.DBName != "teamgram_audit" || config.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", config.DBName, config.Addr)
	}
	db := sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS users (
		id bigint NOT NULL,
		authorization_ttl_days int NOT NULL DEFAULT 0,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	userID := time.Now().UnixNano()
	if _, err := db.Exec(ctx, "INSERT INTO users (id, authorization_ttl_days) VALUES (?, 0)", userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(ctx, "DELETE FROM users WHERE id = ?", userID) })

	core := &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			UsersDAO: mysql_dao.NewUsersDAO(db),
		}}},
		Logger: logx.WithContext(ctx),
	}
	if result, err := core.UserSetAuthorizationTTL(&userpb.TLUserSetAuthorizationTTL{UserId: userID, Ttl: 365}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set authorization TTL = (%v, %v), want BoolTrue", result, err)
	}
	result, err := core.UserGetAuthorizationTTL(&userpb.TLUserGetAuthorizationTTL{UserId: userID})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.GetDays() != 365 {
		t.Fatalf("get authorization TTL = %v, want 365 days", result)
	}
}
