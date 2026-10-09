package core

import (
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "APIFULL_POSTGRES_DSN is not configured; database-backed APIFull core suite was not executed")
		os.Exit(0)
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		panic(err)
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		_ = domain.Close()
		panic(err)
	}
	code := m.Run()
	_ = persist.ClosePostgres()
	_ = domain.Close()
	os.Exit(code)
}
