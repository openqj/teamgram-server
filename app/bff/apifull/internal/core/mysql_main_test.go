package core

import (
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		// The package is an audit suite for the retired MySQL fixture. Keep it
		// opt-in so a fresh PostgreSQL checkout does not fail before tests run.
		os.Exit(0)
	}
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
