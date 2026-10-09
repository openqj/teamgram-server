package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
)

type AuthUsersDAO struct{ db DB }

func NewAuthUsersDAO(db DB) *AuthUsersDAO { return &AuthUsersDAO{db: db} }

const authUsersColumns = `id, auth_key_id, user_id, hash, date_created, date_active, android_push_session_id`

func (dao *AuthUsersDAO) InsertOrUpdates(ctx context.Context, do *dataobject.AuthUsersDO) (int64, int64, error) {
	return dao.insertOrUpdates(ctx, dao.db, do)
}

func (dao *AuthUsersDAO) InsertOrUpdatesTx(ctx context.Context, tx pgx.Tx, do *dataobject.AuthUsersDO) (int64, int64, error) {
	return dao.insertOrUpdates(ctx, tx, do)
}

func (dao *AuthUsersDAO) insertOrUpdates(ctx context.Context, db DB, do *dataobject.AuthUsersDO) (int64, int64, error) {
	return insertResult(ctx, db, `
		INSERT INTO auth_users (auth_key_id, user_id, hash, date_created, date_active)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (auth_key_id) WHERE deleted = FALSE DO UPDATE SET
		user_id = EXCLUDED.user_id, hash = EXCLUDED.hash,
		date_created = EXCLUDED.date_created, date_active = EXCLUDED.date_active,
		deleted = FALSE
		WHERE auth_users.user_id = EXCLUDED.user_id
		RETURNING id`, do.AuthKeyId, do.UserId, do.Hash, do.DateCreated, do.DateActive)
}

func (dao *AuthUsersDAO) Select(ctx context.Context, authKeyID int64) (*dataobject.AuthUsersDO, error) {
	return scanAuthUsers(dao.db.QueryRow(ctx,
		`SELECT `+authUsersColumns+` FROM auth_users WHERE auth_key_id = $1 AND deleted = FALSE`, authKeyID))
}

func (dao *AuthUsersDAO) UpdateAndroidPushSessionId(ctx context.Context, sessionID, authKeyID, userID int64) (int64, error) {
	return updateAndroidPushSessionID(ctx, dao.db, sessionID, authKeyID, userID)
}

func (dao *AuthUsersDAO) UpdateAndroidPushSessionIdTx(ctx context.Context, tx pgx.Tx, sessionID, authKeyID, userID int64) (int64, error) {
	return updateAndroidPushSessionID(ctx, tx, sessionID, authKeyID, userID)
}

func updateAndroidPushSessionID(ctx context.Context, db DB, sessionID, authKeyID, userID int64) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE auth_users SET android_push_session_id = $1 WHERE auth_key_id = $2 AND user_id = $3`, sessionID, authKeyID, userID)
	return tag.RowsAffected(), err
}

func (dao *AuthUsersDAO) SelectAuthKeyIds(ctx context.Context, userID int64) ([]dataobject.AuthUsersDO, error) {
	return dao.selectList(ctx, userID)
}

func (dao *AuthUsersDAO) SelectAuthKeyIdsWithCB(ctx context.Context, userID int64, cb func(sz, i int, v *dataobject.AuthUsersDO)) ([]dataobject.AuthUsersDO, error) {
	values, err := dao.selectList(ctx, userID)
	if err == nil && cb != nil {
		for i := range values {
			cb(len(values), i, &values[i])
		}
	}
	return values, err
}

func (dao *AuthUsersDAO) SelectListByUserId(ctx context.Context, userID int64) ([]dataobject.AuthUsersDO, error) {
	return dao.selectList(ctx, userID)
}

func (dao *AuthUsersDAO) SelectListByUserIdWithCB(ctx context.Context, userID int64, cb func(sz, i int, v *dataobject.AuthUsersDO)) ([]dataobject.AuthUsersDO, error) {
	return dao.SelectAuthKeyIdsWithCB(ctx, userID, cb)
}

func (dao *AuthUsersDAO) selectList(ctx context.Context, userID int64) ([]dataobject.AuthUsersDO, error) {
	rows, err := dao.db.Query(ctx, `SELECT `+authUsersColumns+` FROM auth_users WHERE user_id = $1 AND deleted = FALSE ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]dataobject.AuthUsersDO, 0)
	for rows.Next() {
		var do dataobject.AuthUsersDO
		if err := rows.Scan(&do.Id, &do.AuthKeyId, &do.UserId, &do.Hash, &do.DateCreated, &do.DateActive, &do.AndroidPushSessionId); err != nil {
			return nil, err
		}
		values = append(values, do)
	}
	return values, rows.Err()
}

func (dao *AuthUsersDAO) DeleteByHashList(ctx context.Context, ids []int64) (int64, error) {
	return deleteByIDs(ctx, dao.db, ids)
}

func (dao *AuthUsersDAO) DeleteByHashListTx(ctx context.Context, tx pgx.Tx, ids []int64) (int64, error) {
	return deleteByIDs(ctx, tx, ids)
}

func deleteByIDs(ctx context.Context, db DB, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := db.Exec(ctx, `UPDATE auth_users SET deleted = TRUE, date_created = 0, date_active = 0 WHERE id = ANY($1::bigint[])`, ids)
	return tag.RowsAffected(), err
}

func (dao *AuthUsersDAO) Delete(ctx context.Context, authKeyID, userID int64) (int64, error) {
	return deleteOne(ctx, dao.db, authKeyID, userID)
}

func (dao *AuthUsersDAO) DeleteTx(ctx context.Context, tx pgx.Tx, authKeyID, userID int64) (int64, error) {
	return deleteOne(ctx, tx, authKeyID, userID)
}

func deleteOne(ctx context.Context, db DB, authKeyID, userID int64) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE auth_users SET deleted = TRUE, date_active = 0 WHERE auth_key_id = $1 AND user_id = $2`, authKeyID, userID)
	return tag.RowsAffected(), err
}

func (dao *AuthUsersDAO) DeleteUser(ctx context.Context, userID int64) (int64, error) {
	tag, err := dao.db.Exec(ctx, `UPDATE auth_users SET deleted = TRUE, date_active = 0 WHERE user_id = $1`, userID)
	return tag.RowsAffected(), err
}

func (dao *AuthUsersDAO) DeleteUserTx(ctx context.Context, tx pgx.Tx, userID int64) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE auth_users SET deleted = TRUE, date_active = 0 WHERE user_id = $1`, userID)
	return tag.RowsAffected(), err
}
