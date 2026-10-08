package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
)

type AuthKeysDAO struct{ db DB }

func NewAuthKeysDAO(db DB) *AuthKeysDAO { return &AuthKeysDAO{db: db} }

func (dao *AuthKeysDAO) Insert(ctx context.Context, do *dataobject.AuthKeysDO) (int64, int64, error) {
	return insertResult(ctx, dao.db,
		`INSERT INTO auth_keys (auth_key_id, body) VALUES ($1, $2) RETURNING id`,
		do.AuthKeyId, bodyValue(do.Body))
}

func (dao *AuthKeysDAO) InsertTx(ctx context.Context, tx pgx.Tx, do *dataobject.AuthKeysDO) (int64, int64, error) {
	return NewAuthKeysDAO(tx).Insert(ctx, do)
}

func (dao *AuthKeysDAO) SelectByAuthKeyId(ctx context.Context, authKeyID int64) (*dataobject.AuthKeysDO, error) {
	return scanAuthKeys(dao.db.QueryRow(ctx,
		`SELECT auth_key_id, body FROM auth_keys WHERE auth_key_id = $1 AND deleted = FALSE`, authKeyID))
}
