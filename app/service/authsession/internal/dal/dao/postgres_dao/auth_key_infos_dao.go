package postgres_dao

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
)

type AuthKeyInfosDAO struct{ db DB }

func NewAuthKeyInfosDAO(db DB) *AuthKeyInfosDAO { return &AuthKeyInfosDAO{db: db} }

func (dao *AuthKeyInfosDAO) Insert(ctx context.Context, do *dataobject.AuthKeyInfosDO) (int64, int64, error) {
	return insertResult(ctx, dao.db, `
		INSERT INTO auth_key_infos
		(auth_key_id, auth_key_type, perm_auth_key_id, temp_auth_key_id, media_temp_auth_key_id)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		do.AuthKeyId, do.AuthKeyType, do.PermAuthKeyId, do.TempAuthKeyId, do.MediaTempAuthKeyId)
}

func (dao *AuthKeyInfosDAO) InsertTx(ctx context.Context, tx pgx.Tx, do *dataobject.AuthKeyInfosDO) (int64, int64, error) {
	return NewAuthKeyInfosDAO(tx).Insert(ctx, do)
}

func (dao *AuthKeyInfosDAO) SelectByAuthKeyId(ctx context.Context, authKeyID int64) (*dataobject.AuthKeyInfosDO, error) {
	return scanAuthKeyInfos(dao.db.QueryRow(ctx, `
		SELECT auth_key_id, auth_key_type, perm_auth_key_id, temp_auth_key_id, media_temp_auth_key_id
		FROM auth_key_infos WHERE auth_key_id = $1 AND deleted = FALSE LIMIT 1`, authKeyID))
}

// UpdateCustomMap updates only the fields used by MTProto auth-key binding.
// The whitelist prevents a caller from turning a map key into SQL syntax.
func (dao *AuthKeyInfosDAO) UpdateCustomMap(ctx context.Context, fields map[string]any, authKeyID int64) (int64, error) {
	return updateCustomMap(ctx, dao.db, fields, authKeyID)
}

func (dao *AuthKeyInfosDAO) UpdateCustomMapTx(ctx context.Context, tx pgx.Tx, fields map[string]any, authKeyID int64) (int64, error) {
	return updateCustomMap(ctx, tx, fields, authKeyID)
}

func updateCustomMap(ctx context.Context, db DB, fields map[string]any, authKeyID int64) (int64, error) {
	allowed := map[string]bool{
		"perm_auth_key_id":       true,
		"temp_auth_key_id":       true,
		"media_temp_auth_key_id": true,
	}
	if len(fields) == 0 {
		return 0, nil
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		if !allowed[name] {
			return 0, fmt.Errorf("authsession: unsupported auth_key_infos field %q", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)

	sets := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	idx := 1
	for _, name := range names {
		sets = append(sets, fmt.Sprintf("%s = $%d", name, idx))
		args = append(args, fields[name])
		idx++
	}
	args = append(args, authKeyID)
	tag, err := db.Exec(ctx, fmt.Sprintf("UPDATE auth_key_infos SET %s WHERE auth_key_id = $%d AND deleted = FALSE", joinComma(sets), idx), args...)
	return tag.RowsAffected(), err
}

func joinComma(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ", "
		}
		result += value
	}
	return result
}
