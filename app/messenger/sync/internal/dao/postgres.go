package dao

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres contains the durable update queues used by sync. The pool is kept
// on the DAO so a future message/update transaction can share one connection.
type Postgres struct {
	Pool *pgxpool.Pool
	*postgres_dao.AuthSeqUpdatesDAO
	*postgres_dao.UserPtsUpdatesDAO
}

func newPostgresDao(c config.Config) (*Postgres, error) {
	pool, err := postgres.NewPool(context.Background(), c.Postgres)
	if err != nil {
		return nil, err
	}
	return &Postgres{
		Pool:              pool,
		AuthSeqUpdatesDAO: postgres_dao.NewAuthSeqUpdatesDAO(pool),
		UserPtsUpdatesDAO: postgres_dao.NewUserPtsUpdatesDAO(pool),
	}, nil
}

func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
