// Package postgres_dao contains the PostgreSQL persistence boundary for the
// messenger sync service.
package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is implemented by both pgxpool.Pool and pgx.Tx. Keeping this interface
// narrow lets callers add an update row in the same transaction as a message.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func rowsAffected(tag pgconn.CommandTag) int64 { return tag.RowsAffected() }

var _ DB = (*pgxpool.Pool)(nil)
var _ DB = (pgx.Tx)(nil)
