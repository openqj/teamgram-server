// Package postgres_dao contains the PostgreSQL persistence boundary for
// messages. The DB interface also accepts pgx.Tx for atomic message and
// update/outbox writes.
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
	Pool              *pgxpool.Pool
	Messages          *MessagesDAO
	MessageReadOutbox *MessageReadOutboxDAO
	HashTags          *HashTagsDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Pool:              pool,
		Messages:          NewMessagesDAO(pool),
		MessageReadOutbox: NewMessageReadOutboxDAO(pool),
		HashTags:          NewHashTagsDAO(pool),
	}
}

func rowsAffected(tag pgconn.CommandTag) int64 { return tag.RowsAffected() }
