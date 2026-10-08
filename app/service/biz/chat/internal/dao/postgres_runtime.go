package dao

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres is the service-owned PostgreSQL runtime boundary. The generated
// handler surface can migrate aggregate by aggregate without opening a second
// pool or reaching into pgx from the transport layer.
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

func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
