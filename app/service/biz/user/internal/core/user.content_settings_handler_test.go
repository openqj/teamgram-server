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

func contentSettingsAuditDB(t *testing.T) *sqlx.DB {
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

func TestUserContentSettingsRoundTrip(t *testing.T) {
	db := contentSettingsAuditDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS user_settings (
		id bigint NOT NULL AUTO_INCREMENT,
		user_id bigint NOT NULL,
		key2 varchar(64) NOT NULL,
		value varchar(512) NOT NULL,
		deleted tinyint(1) NOT NULL DEFAULT 0,
		created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY user_key (user_id, key2)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	userID := time.Now().UnixNano()
	t.Cleanup(func() { _, _ = db.Exec(ctx, "DELETE FROM user_settings WHERE user_id = ?", userID) })
	core := &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			UserSettingsDAO: mysql_dao.NewUserSettingsDAO(db),
		}}},
		Logger: logx.WithContext(ctx),
	}
	if result, err := core.UserSetContentSettings(&userpb.TLUserSetContentSettings{UserId: userID, SensitiveEnabled: true}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set content settings = (%v, %v), want BoolTrue", result, err)
	}
	result, err := core.UserGetContentSettings(&userpb.TLUserGetContentSettings{UserId: userID})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.GetSensitiveEnabled() {
		t.Fatalf("get content settings = %v, want sensitive_enabled=true", result)
	}
}

func TestUserContentSettingsPropagatesStorageError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "ttl:ttl@tcp(127.0.0.1:1)/teamgram_content_settings_error?timeout=50ms&readTimeout=50ms&writeTimeout=50ms"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	core := &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			UserSettingsDAO: mysql_dao.NewUserSettingsDAO(db),
		}}},
		Logger: logx.WithContext(ctx),
	}
	if result, err := core.UserSetContentSettings(&userpb.TLUserSetContentSettings{UserId: 42, SensitiveEnabled: true}); result != nil || err == nil {
		t.Fatalf("set content settings storage error = (%v, %v), want nil and error", result, err)
	}
	if result, err := core.UserGetContentSettings(&userpb.TLUserGetContentSettings{UserId: 42}); result != nil || err == nil {
		t.Fatalf("get content settings storage error = (%v, %v), want nil and error", result, err)
	}
}
