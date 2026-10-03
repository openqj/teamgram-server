package core

import (
	"context"
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

func defaultHistoryTTLAuditDB(t *testing.T) *sqlx.DB {
	t.Helper()
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
	return sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
}

func TestUserDefaultHistoryTTLRoundTrip(t *testing.T) {
	db := defaultHistoryTTLAuditDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS default_history_ttl (
		id bigint NOT NULL AUTO_INCREMENT,
		user_id bigint NOT NULL,
		period int NOT NULL,
		created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY user_id (user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	userID := time.Now().UnixNano()
	t.Cleanup(func() { _, _ = db.Exec(ctx, "DELETE FROM default_history_ttl WHERE user_id = ?", userID) })

	core := &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			DefaultHistoryTtlDAO: mysql_dao.NewDefaultHistoryTtlDAO(db),
		}}},
		Logger: logx.WithContext(ctx),
	}
	if result, err := core.UserSetDefaultHistoryTTL(&userpb.TLUserSetDefaultHistoryTTL{UserId: userID, Ttl: 604800}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set default history TTL = (%v, %v), want BoolTrue", result, err)
	}
	result, err := core.UserGetDefaultHistoryTTL(&userpb.TLUserGetDefaultHistoryTTL{UserId: userID})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.GetPeriod() != 604800 {
		t.Fatalf("get default history TTL = %v, want period 604800", result)
	}
}

func TestUserDefaultHistoryTTLPropagatesStorageError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "ttl:ttl@tcp(127.0.0.1:1)/teamgram_ttl_error_test?timeout=50ms&readTimeout=50ms&writeTimeout=50ms"})
	if err != nil {
		t.Fatal(err)
	}
	core := &UserCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			DefaultHistoryTtlDAO: mysql_dao.NewDefaultHistoryTtlDAO(db),
		}}},
		Logger: logx.WithContext(context.Background()),
	}
	if result, err := core.UserSetDefaultHistoryTTL(&userpb.TLUserSetDefaultHistoryTTL{UserId: 42, Ttl: 60}); result != nil || err == nil {
		t.Fatalf("set storage error = (%v, %v), want nil result and error", result, err)
	}
	if result, err := core.UserGetDefaultHistoryTTL(&userpb.TLUserGetDefaultHistoryTTL{UserId: 42}); result != nil || err == nil {
		t.Fatalf("get storage error = (%v, %v), want nil result and error", result, err)
	}
}
