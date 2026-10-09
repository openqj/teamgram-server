package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func VerifySchema(ctx context.Context, db DB) error {
	return postgres.VerifySchema(ctx, db,
		`SELECT `+messageColumns+` FROM messages LIMIT 0`,
		`SELECT id,user_id,peer_dialog_id,read_user_id,read_outbox_max_id,read_outbox_max_date FROM message_read_outbox LIMIT 0`,
		`SELECT id,user_id,peer_type,peer_id,hash_tag,hash_tag_message_id,deleted FROM hash_tags LIMIT 0`)
}
