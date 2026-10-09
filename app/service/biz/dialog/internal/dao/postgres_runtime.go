package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres owns the dialog service's authoritative connection pool and stores.
type Postgres struct {
	Pool  *pgxpool.Pool
	Store *postgres_dao.Store
}

func NewPostgres(cfg postgres.Config) (*Postgres, error) {
	pool, err := postgres.NewPool(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	if err := postgres_dao.VerifySchema(context.Background(), pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{Pool: pool, Store: postgres_dao.NewStore(pool)}, nil
}

// InTx runs an authoritative dialog mutation on one PostgreSQL transaction.
// DAO transaction variants accept the pgx.Tx passed to the callback, keeping
// dialog rows and related filter/draft state atomic.
func (d *Postgres) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if d == nil || d.Pool == nil {
		return errors.New("biz/dialog: postgres store is not configured")
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

func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
