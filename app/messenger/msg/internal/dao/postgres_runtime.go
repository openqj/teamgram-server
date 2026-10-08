package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres is the message service's PostgreSQL runtime boundary. Keeping the
// pool and aggregate stores together makes it possible for an inbox delivery
// to persist a message, its hash tags, and pts state on one pgx transaction.
//
// The generated handlers still use the legacy Mysql field on Dao until their
// transaction callbacks are converted. New code must use this boundary and
// must not add methods to the MySQL DAO.
type Postgres struct {
	Pool  *pgxpool.Pool
	Store *postgres_dao.Store
}

// NewPostgres opens and health-checks the service-owned PostgreSQL pool.
func NewPostgres(cfg postgres.Config) (*Postgres, error) {
	pool, err := postgres.NewPool(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return &Postgres{Pool: pool, Store: postgres_dao.NewStore(pool)}, nil
}

// InTx executes fn in a database transaction. Message persistence code should
// use the transaction passed to the DAO *On methods so delivery and update
// state remain atomic.
func (d *Postgres) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if d == nil || d.Pool == nil {
		return errors.New("messenger/msg: postgres store is not configured")
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Close releases the pool during graceful service shutdown.
func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
