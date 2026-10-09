// Package postgres owns the project's PostgreSQL connection boundary.
//
// New code should depend on this package instead of importing a MySQL driver
// or the Teamgram MySQL wrapper. Queries passed to this package use PostgreSQL
// numbered placeholders ($1, $2, ...).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config controls the shared pool used by one service process.
// DSN is a PostgreSQL URL or libpq-style connection string.
type Config struct {
	DSN string

	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
}

// withDefaults applies conservative service-local pool defaults. The limits
// are intentionally explicit so deployments can budget connections across
// all service processes instead of letting every process open an unbounded
// number of sessions.
func (c Config) withDefaults() Config {
	if c.MaxConns <= 0 {
		c.MaxConns = 16
	}
	if c.MinConns < 0 {
		c.MinConns = 0
	}
	if c.MinConns > c.MaxConns {
		c.MinConns = c.MaxConns
	}
	if c.MaxConnLifetime <= 0 {
		c.MaxConnLifetime = time.Hour
	}
	if c.MaxConnIdleTime <= 0 {
		c.MaxConnIdleTime = 30 * time.Minute
	}
	if c.HealthCheckPeriod <= 0 {
		c.HealthCheckPeriod = time.Minute
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	return c
}

// NewPool creates and verifies a PostgreSQL pool. It fails fast when DSN is
// empty or the database is unreachable, which prevents a service from
// accepting MTProto requests while its authoritative store is unavailable.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("postgres: DSN is required")
	}

	cfg = cfg.withDefaults()
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse DSN: %w", err)
	}
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	var serverVersionNum int
	if err := pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&serverVersionNum); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: read server version: %w", err)
	}
	if err := validateServerVersion(serverVersionNum); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// VerifySchema checks required columns before a service starts accepting work.
// Queries should be SELECT statements with LIMIT 0, without runtime mutations.
func VerifySchema(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, queries ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, query := range queries {
		rows, err := db.Query(ctx, query)
		if err != nil {
			return fmt.Errorf("postgres: required schema is unavailable: %w", err)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("postgres: verify required schema: %w", err)
		}
	}
	return nil
}

func validateServerVersion(serverVersionNum int) error {
	if serverVersionNum/10000 != 18 {
		return fmt.Errorf("postgres: PostgreSQL 18 is required (server_version_num=%d)", serverVersionNum)
	}
	return nil
}

// WithTx executes fn in a transaction and commits only when fn succeeds. A
// rollback is attempted for every failed callback or commit path.
func WithTx(ctx context.Context, pool interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}, opts pgx.TxOptions, fn func(pgx.Tx) error) error {
	if pool == nil {
		return errors.New("postgres: nil pool")
	}
	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("postgres: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit transaction: %w", err)
	}
	committed = true
	return nil
}

// IsUniqueViolation reports PostgreSQL's unique/exclusion constraint error.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsSerializationFailure reports serialization and deadlock failures that
// callers may retry at the owning transaction boundary.
func IsSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}
