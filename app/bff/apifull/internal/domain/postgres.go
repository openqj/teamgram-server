package domain

import (
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// DefaultPostgresDSN is the local PostgreSQL 18 endpoint used by the APIFull
// development configuration. Deployments should provide an explicit DSN.
const DefaultPostgresDSN = "postgres://postgres:postgres@127.0.0.1:5432/teamgram?sslmode=disable"

// OpenPostgres opens the APIFull domain against PostgreSQL and provisions the
// complete current schema when application-owned migrations are enabled.
func OpenPostgres(dsn string) error {
	return openPostgresDomain(dsn, false)
}

// OpenPostgresReadOnly connects to a schema provisioned by deployment without
// issuing DDL.
func OpenPostgresReadOnly(dsn string) error {
	return openPostgresDomain(dsn, true)
}

func openPostgresDomain(dsn string, readOnly bool) error {
	if dsn == "" {
		dsn = DefaultPostgresDSN
	}
	if readOnly {
		if err := persist.OpenPostgresReadOnly(dsn); err != nil {
			return err
		}
	} else if err := persist.OpenPostgres(dsn); err != nil {
		return err
	}
	conn, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		return err
	}
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(8)
	if err = conn.Ping(); err != nil {
		_ = conn.Close()
		return err
	}
	if !readOnly {
		if err = migratePostgres(conn); err != nil {
			_ = conn.Close()
			return err
		}
	}
	if db != nil {
		_ = db.Close()
	}
	db = conn
	return nil
}
