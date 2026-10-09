package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func VerifySchema(ctx context.Context, db DB) error {
	return postgres.VerifySchema(ctx, db,
		`SELECT `+dialogColumns+` FROM dialogs LIMIT 0`,
		`SELECT `+filterColumns+` FROM dialog_filters LIMIT 0`,
		`SELECT `+draftColumns+` FROM drafts LIMIT 0`,
		`SELECT `+savedDialogColumns+` FROM saved_dialogs LIMIT 0`,
		`SELECT user_id,enabled FROM dialog_filter_tags LIMIT 0`)
}
