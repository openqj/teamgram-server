package dao

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres owns the message service's authoritative connection pool and stores.
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

func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
