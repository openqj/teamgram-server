/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/config"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

type Dao struct {
	*Mysql
	// These interfaces are the narrow update-store surface consumed by the
	// generated RPC core. They can point at either the PostgreSQL DAOs or the
	// legacy MySQL DAOs used by isolated compatibility tests.
	AuthSeqUpdatesDAO AuthSeqUpdatesStore
	UserPtsUpdatesDAO UserPtsUpdatesStore
	Postgres          *Postgres
}

type AuthSeqUpdatesStore interface {
	Insert(context.Context, *dataobject.AuthSeqUpdatesDO) (int64, int64, error)
	SelectLastSeq(context.Context, int64, int64) (*dataobject.AuthSeqUpdatesDO, error)
	SelectByGtSeq(context.Context, int64, int64, int32) ([]dataobject.AuthSeqUpdatesDO, error)
	SelectByGtDate(context.Context, int64, int64, int64, int32) ([]dataobject.AuthSeqUpdatesDO, error)
	SelectByGtSeqWithCB(context.Context, int64, int64, int32, func(int, int, *dataobject.AuthSeqUpdatesDO)) ([]dataobject.AuthSeqUpdatesDO, error)
	SelectByGtDateWithCB(context.Context, int64, int64, int64, int32, func(int, int, *dataobject.AuthSeqUpdatesDO)) ([]dataobject.AuthSeqUpdatesDO, error)
}

type UserPtsUpdatesStore interface {
	Insert(context.Context, *dataobject.UserPtsUpdatesDO) (int64, int64, error)
	SelectLastPts(context.Context, int64) (*dataobject.UserPtsUpdatesDO, error)
	SelectByGtPts(context.Context, int64, int32, int32) ([]dataobject.UserPtsUpdatesDO, error)
	SelectByGtPtsWithCB(context.Context, int64, int32, int32, func(int, int, *dataobject.UserPtsUpdatesDO)) ([]dataobject.UserPtsUpdatesDO, error)
}

func New(c config.Config) *Dao {
	if c.Postgres.DSN == "" {
		panic("updates: Postgres.DSN is required")
	}
	pg, err := newPostgresDao(c)
	if err != nil {
		panic(err)
	}
	return &Dao{
		AuthSeqUpdatesDAO: pg.AuthSeqUpdatesDAO,
		UserPtsUpdatesDAO: pg.UserPtsUpdatesDAO,
		Postgres:          pg,
	}
}

// Postgres owns the update pool for one service process. Keeping the pool on
// the runtime boundary allows future message/update transactions to share a
// connection without exposing pgx details through the RPC layer.
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
	if err := postgres.VerifySchema(context.Background(), pool,
		`SELECT id,user_id,pts,pts_count,update_type,update_data,date2 FROM user_pts_updates LIMIT 0`,
		`SELECT id,auth_id,user_id,seq,update_type,update_data,date2 FROM auth_seq_updates LIMIT 0`,
		`SELECT user_id,last_qts FROM apifull_secret_user_state LIMIT 0`); err != nil {
		pool.Close()
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
