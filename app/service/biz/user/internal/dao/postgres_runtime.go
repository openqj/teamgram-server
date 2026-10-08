package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres owns the user service's authoritative connection pool and stores.
type Postgres struct {
	Pool  *pgxpool.Pool
	Store *postgres_dao.Store
}

func NewPostgres(cfg postgres.Config) (*Postgres, error) {
	pool, err := postgres.NewPool(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return &Postgres{Pool: pool, Store: postgres_dao.NewStore(pool)}, nil
}

// InTx runs an authoritative user mutation on one PostgreSQL transaction.
// DAO transaction variants accept the pgx.Tx passed to the callback so user,
// contact, username, and privacy changes commit or roll back together.
func (d *Postgres) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if d == nil || d.Pool == nil {
		return errors.New("biz/user: postgres store is not configured")
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
