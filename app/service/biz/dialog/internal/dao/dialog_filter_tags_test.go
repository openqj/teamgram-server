package dao

import (
	"context"
	"os"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

func TestDialogFilterTagsRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("APIFULL_MYSQL_DSN must point to the isolated teamgram_audit database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.DBName != "teamgram_audit" || config.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", config.DBName, config.Addr)
	}
	d := &Dao{Mysql: newMysqlDao(sqlx.NewMySQL(&sqlx.Config{DSN: dsn}))}
	ctx := context.Background()
	const uid int64 = 91021
	if err := d.SetDialogFilterTags(ctx, uid, false); err != nil {
		t.Fatal(err)
	}
	on, err := d.GetDialogFilterTags(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("tags on after disable")
	}
	if err = d.SetDialogFilterTags(ctx, uid, true); err != nil {
		t.Fatal(err)
	}
	on, err = d.GetDialogFilterTags(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("tags off after enable")
	}
	missing, err := d.GetDialogFilterTags(ctx, 91021999)
	if err != nil {
		t.Fatal(err)
	}
	if missing {
		t.Fatal("missing row should be off")
	}
}
