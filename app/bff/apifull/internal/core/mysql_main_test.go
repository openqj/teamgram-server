package core

import (
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		panic("APIFULL_MYSQL_DSN must point to the isolated audit database")
	}
	if cfg.DBName != "teamgram_audit" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13306" {
		panic("APIFULL_MYSQL_DSN must point to 127.0.0.1:13306/teamgram_audit")
	}
	if err := domain.Open(cfg.FormatDSN()); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
