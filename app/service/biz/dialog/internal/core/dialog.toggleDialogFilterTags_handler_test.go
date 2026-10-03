package core

import (
	"context"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func dialogTagsAuditDB(t *testing.T) *sqlx.DB {
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

func TestDialogToggleDialogFilterTagsAcknowledgesDisable(t *testing.T) {
	db := dialogTagsAuditDB(t)
	ctx := context.Background()
	uid := time.Now().UnixNano()
	c := &DialogCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			Mysql: &dao.Mysql{DB: db},
		}},
	}
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, "delete from dialog_filter_tags where user_id = ?", uid)
	})

	for _, enabled := range []*mtproto.Bool{mtproto.BoolTrue, mtproto.BoolFalse} {
		result, err := c.DialogToggleDialogFilterTags(&dialog.TLDialogToggleDialogFilterTags{
			UserId:  uid,
			Enabled: enabled,
		})
		if err != nil || !mtproto.FromBool(result) {
			t.Fatalf("toggle enabled=%v = (%v, %v), want true", mtproto.FromBool(enabled), result, err)
		}
	}

	stored, err := c.DialogGetDialogFilterTags(&dialog.TLDialogGetDialogFilterTags{UserId: uid})
	if err != nil {
		t.Fatal(err)
	}
	if mtproto.FromBool(stored) {
		t.Fatal("disabled dialog-filter tags were read back as enabled")
	}
}
