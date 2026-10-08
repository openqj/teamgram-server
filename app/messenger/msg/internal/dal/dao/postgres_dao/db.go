// Package postgres_dao exposes the PostgreSQL persistence surface used by the
// messenger service. The aggregate implementations live in the corresponding
// business services; aliases here keep the messenger DAO contract stable while
// the runtime moves off the MySQL wrapper.
package postgres_dao

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	chat_pg "github.com/teamgram/teamgram-server/app/service/biz/chat"
	dialog_pg "github.com/teamgram/teamgram-server/app/service/biz/dialog"
	message_pg "github.com/teamgram/teamgram-server/app/service/biz/message"
)

// DB is implemented by pgxpool.Pool and pgx.Tx. It is intentionally kept
// narrow so message, dialog, and chat writes can share one transaction.
type DB = message_pg.PostgresDB

type (
	MessagesDAO          = message_pg.PostgresMessagesDAO
	MessageReadOutboxDAO = message_pg.PostgresMessageReadOutboxDAO
	DialogsDAO           = dialog_pg.PostgresDialogsDAO
	SavedDialogsDAO      = dialog_pg.PostgresSavedDialogs
	ChatParticipantsDAO  = chat_pg.PostgresChatParticipants
)

var (
	NewMessagesDAO          = message_pg.NewPostgresMessagesDAO
	NewMessageReadOutboxDAO = message_pg.NewPostgresMessageReadOutboxDAO
	NewDialogsDAO           = dialog_pg.NewPostgresDialogsDAO
	NewSavedDialogsDAO      = dialog_pg.NewPostgresSavedDialogsDAO
	NewChatParticipantsDAO  = chat_pg.NewPostgresChatParticipantsDAO
)

// Store groups all PostgreSQL DAOs needed by messenger. Keeping the pool on
// the store allows callers to begin a transaction and pass it to the *On
// methods exposed by the individual DAOs.
type Store struct {
	Pool              *pgxpool.Pool
	Messages          *MessagesDAO
	MessageReadOutbox *MessageReadOutboxDAO
	HashTags          *HashTagsDAO
	Dialogs           *DialogsDAO
	SavedDialogs      *SavedDialogsDAO
	ChatParticipants  *ChatParticipantsDAO
	UserPtsUpdates    *UserPtsUpdatesDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Pool:              pool,
		Messages:          NewMessagesDAO(pool),
		MessageReadOutbox: NewMessageReadOutboxDAO(pool),
		HashTags:          NewHashTagsDAO(pool),
		Dialogs:           NewDialogsDAO(pool),
		SavedDialogs:      NewSavedDialogsDAO(pool),
		ChatParticipants:  NewChatParticipantsDAO(pool),
		UserPtsUpdates:    NewUserPtsUpdatesDAO(pool),
	}
}

func rowsAffected(tag pgconn.CommandTag) int64 { return tag.RowsAffected() }

// Keep pgx in this package's public dependency set so pgx.Tx satisfies DB for
// callers without requiring a second adapter type.
var _ DB = (*pgxpool.Pool)(nil)
var _ DB = (pgx.Tx)(nil)
