package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func VerifySchema(ctx context.Context, db DB) error {
	return postgres.VerifySchema(ctx, db,
		`SELECT `+documentColumns+` FROM documents LIMIT 0`,
		`SELECT `+photoColumns+` FROM photos LIMIT 0`,
		`SELECT `+photoSizeColumns+` FROM photo_sizes LIMIT 0`,
		`SELECT `+videoSizeColumns+` FROM video_sizes LIMIT 0`,
		`SELECT encrypted_file_id, owner_id, access_hash, file_size, dc_id, key_fingerprint FROM encrypted_files LIMIT 0`)
}
