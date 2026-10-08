// Package postgres_dao contains the PostgreSQL persistence boundary for the
// basic-group chat service. It deliberately has no dependency on Teamgram's
// MySQL wrapper so the service can migrate one aggregate at a time.
package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// DB is implemented by pgxpool.Pool and pgx.Tx. Keeping the narrow interface
// lets callers execute chat mutations in the same transaction as other state.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	Pool               *pgxpool.Pool
	Chats              *ChatsDAO
	Participants       *ChatParticipantsDAO
	Invites            *ChatInvitesDAO
	InviteUsers        *ChatInviteParticipantsDAO
	InviteParticipants *ChatInviteParticipantsDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	inviteParticipants := NewChatInviteParticipantsDAO(pool)
	return &Store{
		Pool:               pool,
		Chats:              NewChatsDAO(pool),
		Participants:       NewChatParticipantsDAO(pool),
		Invites:            NewChatInvitesDAO(pool),
		InviteUsers:        inviteParticipants,
		InviteParticipants: inviteParticipants,
	}
}

type ChatsDAO struct{ db DB }

func NewChatsDAO(db DB) *ChatsDAO { return &ChatsDAO{db: db} }

func commandRows(tag pgconn.CommandTag) int64 { return tag.RowsAffected() }

func scanChat(row pgx.Row) (*dataobject.ChatsDO, error) {
	do := new(dataobject.ChatsDO)
	err := row.Scan(
		&do.Id, &do.CreatorUserId, &do.AccessHash, &do.ParticipantCount,
		&do.Title, &do.About, &do.PhotoId, &do.DefaultBannedRights,
		&do.MigratedToId, &do.MigratedToAccessHash, &do.Noforwards,
		&do.AvailableReactionsType, &do.AvailableReactions, &do.Deactivated,
		&do.TtlPeriod, &do.Version, &do.Date,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanChatRows(rows pgx.Rows) ([]dataobject.ChatsDO, error) {
	defer rows.Close()
	result := make([]dataobject.ChatsDO, 0)
	for rows.Next() {
		var do dataobject.ChatsDO
		if err := rows.Scan(
			&do.Id, &do.CreatorUserId, &do.AccessHash, &do.ParticipantCount,
			&do.Title, &do.About, &do.PhotoId, &do.DefaultBannedRights,
			&do.MigratedToId, &do.MigratedToAccessHash, &do.Noforwards,
			&do.AvailableReactionsType, &do.AvailableReactions, &do.Deactivated,
			&do.TtlPeriod, &do.Version, &do.Date,
		); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
