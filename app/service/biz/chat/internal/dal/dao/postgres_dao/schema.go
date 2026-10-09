package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func VerifySchema(ctx context.Context, db DB) error {
	return postgres.VerifySchema(ctx, db,
		`SELECT `+chatColumns+` FROM chats LIMIT 0`,
		`SELECT `+participantColumns+` FROM chat_participants LIMIT 0`,
		`SELECT `+inviteColumns+` FROM chat_invites LIMIT 0`,
		`SELECT `+inviteParticipantColumns+` FROM chat_invite_participants LIMIT 0`)
}
