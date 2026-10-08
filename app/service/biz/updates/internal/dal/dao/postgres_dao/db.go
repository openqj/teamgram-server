// Package postgres_dao contains the PostgreSQL persistence boundary for update
// state. It deliberately uses pgx directly so sequence writes can share a
// transaction with the authoritative message mutation and outbox record.
package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	Pool           *pgxpool.Pool
	UserPtsUpdates *UserPtsUpdatesDAO
	AuthSeqUpdates *AuthSeqUpdatesDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Pool:           pool,
		UserPtsUpdates: NewUserPtsUpdatesDAO(pool),
		AuthSeqUpdates: NewAuthSeqUpdatesDAO(pool),
	}
}

func rowsAffected(tag pgconn.CommandTag) int64 { return tag.RowsAffected() }
