package domain

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

const postgresMaxOpenConns = 8

// Workers hold advisory locks while using other connections for durable
// writes. Reserve half the pool so concurrent workers cannot exhaust it.
var postgresAdvisoryLockSlots = make(chan struct{}, postgresMaxOpenConns/2)

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
	if strings.TrimSpace(dsn) == "" {
		return errors.New("apifull: PostgreSQL DSN is required")
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
	conn.SetMaxOpenConns(postgresMaxOpenConns)
	conn.SetMaxIdleConns(postgresMaxOpenConns)
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

func lockPostgresTransaction(ctx context.Context, name string, wait time.Duration) (func(), bool, error) {
	waitCtx := ctx
	cancel := func() {}
	if wait > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, wait)
	}
	defer cancel()
	if wait <= 0 {
		select {
		case postgresAdvisoryLockSlots <- struct{}{}:
		default:
			return nil, false, nil
		}
	} else {
		select {
		case postgresAdvisoryLockSlots <- struct{}{}:
		case <-waitCtx.Done():
			return nil, false, ctx.Err()
		}
	}
	conn, err := db.Conn(waitCtx)
	if err != nil {
		<-postgresAdvisoryLockSlots
		if waitCtx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		_ = conn.Close()
		<-postgresAdvisoryLockSlots
		return nil, false, err
	}
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			_ = tx.Rollback()
			_ = conn.Close()
			<-postgresAdvisoryLockSlots
		})
	}
	acquired := true
	if wait <= 0 {
		err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, name).Scan(&acquired)
	} else {
		_, err = tx.ExecContext(waitCtx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, name)
	}
	if err != nil || !acquired {
		release()
		if waitCtx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, err
	}
	return release, true, nil
}

func execPostgresMutation(query string, args ...any) (sql.Result, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
