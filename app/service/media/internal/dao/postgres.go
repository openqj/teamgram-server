package dao

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	postgres_dao "github.com/teamgram/teamgram-server/app/service/media/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres is the production media persistence boundary. The generated media
// handlers use the same aggregate methods regardless of the backing store;
// only this adapter owns the PostgreSQL pool and DAOs.
type Postgres struct {
	Pool          *pgxpool.Pool
	DocumentsDAO  *postgres_dao.DocumentsDAO
	PhotosDAO     *postgres_dao.PhotosDAO
	PhotoSizesDAO *postgres_dao.PhotoSizesDAO
	VideoSizesDAO *postgres_dao.VideoSizesDAO
}

func newPostgresDao(c config.Config) *Postgres {
	pool, err := postgres.NewPool(context.Background(), c.Postgres)
	if err != nil {
		panic(err)
	}
	store := postgres_dao.NewStore(pool)
	return &Postgres{
		Pool:          pool,
		DocumentsDAO:  store.Documents,
		PhotosDAO:     store.Photos,
		PhotoSizesDAO: store.PhotoSizes,
		VideoSizesDAO: store.VideoSizes,
	}
}

func (p *Postgres) Close() {
	if p != nil && p.Pool != nil {
		p.Pool.Close()
	}
}
