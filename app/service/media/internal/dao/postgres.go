package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	postgres_dao "github.com/teamgram/teamgram-server/app/service/media/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
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
	if err := postgres_dao.VerifySchema(context.Background(), pool); err != nil {
		pool.Close()
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

func (p *Postgres) SavePhoto(ctx context.Context, photo *dataobject.PhotosDO, sizes []*dataobject.PhotoSizesDO, videoSizes []*dataobject.VideoSizesDO) error {
	if p == nil || p.Pool == nil {
		return errors.New("media: PostgreSQL store is not initialized")
	}
	return postgres.WithTx(ctx, p.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, _, err := p.PhotosDAO.InsertTx(ctx, tx, photo); err != nil {
			return err
		}
		for _, size := range sizes {
			if _, _, err := p.PhotoSizesDAO.InsertTx(ctx, tx, size); err != nil {
				return err
			}
		}
		for _, size := range videoSizes {
			if _, _, err := p.VideoSizesDAO.InsertTx(ctx, tx, size); err != nil {
				return err
			}
		}
		return nil
	})
}
