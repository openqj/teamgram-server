package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// VerifySchema checks every table owned by the user service before the
// process starts accepting RPCs. Keeping this at the connection boundary
// prevents a partially applied 003_biz_user migration from failing only when
// a less frequently used handler is first called.
func VerifySchema(ctx context.Context, db DB) error {
	return postgres.VerifySchema(ctx, db,
		`SELECT `+userColumns+` FROM users LIMIT 0`,
		`SELECT id,username,peer_type,peer_id,editable,active,order2,deleted FROM username LIMIT 0`,
		`SELECT id,owner_user_id,contact_user_id,contact_phone,contact_first_name,contact_last_name,mutual,close_friend,stories_hidden,date2,is_deleted FROM user_contacts LIMIT 0`,
		`SELECT 1 FROM user_privacies LIMIT 0`,
		`SELECT 1 FROM user_presences LIMIT 0`,
		`SELECT 1 FROM user_peer_blocks LIMIT 0`,
		`SELECT 1 FROM user_peer_settings LIMIT 0`,
		`SELECT 1 FROM user_notify_settings LIMIT 0`,
		`SELECT 1 FROM user_global_privacy_settings LIMIT 0`,
		`SELECT 1 FROM user_profile_photos LIMIT 0`,
		`SELECT 1 FROM user_saved_music LIMIT 0`,
		`SELECT 1 FROM user_settings LIMIT 0`,
		`SELECT 1 FROM bots LIMIT 0`,
		`SELECT 1 FROM bot_commands LIMIT 0`,
		`SELECT 1 FROM default_history_ttl LIMIT 0`,
		`SELECT 1 FROM imported_contacts LIMIT 0`,
		`SELECT 1 FROM phone_books LIMIT 0`,
		`SELECT 1 FROM popular_contacts LIMIT 0`,
		`SELECT 1 FROM predefined_users LIMIT 0`,
		`SELECT 1 FROM unregistered_contacts LIMIT 0`)
}
