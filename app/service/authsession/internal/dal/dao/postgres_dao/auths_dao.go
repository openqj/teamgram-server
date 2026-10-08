package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
)

type AuthsDAO struct{ db DB }

func NewAuthsDAO(db DB) *AuthsDAO { return &AuthsDAO{db: db} }

func (dao *AuthsDAO) InsertOrUpdateLayer(ctx context.Context, do *dataobject.AuthsDO) (int64, int64, error) {
	params, err := paramsValue(do.Params)
	if err != nil {
		return 0, 0, err
	}
	return upsertAuths(ctx, dao.db, `
		INSERT INTO auths (auth_key_id, layer, client_ip, date_active, params)
		VALUES ($1, $2, NULLIF($3, '')::inet, $4, COALESCE($5::jsonb, 'null'::jsonb))
		ON CONFLICT (auth_key_id) DO UPDATE SET
		layer = EXCLUDED.layer, client_ip = EXCLUDED.client_ip, date_active = EXCLUDED.date_active
		RETURNING id`, do.AuthKeyId, do.Layer, do.ClientIp, do.DateActive, params)
}

func (dao *AuthsDAO) InsertOrUpdateLayerTx(ctx context.Context, tx pgx.Tx, do *dataobject.AuthsDO) (int64, int64, error) {
	return NewAuthsDAO(tx).InsertOrUpdateLayer(ctx, do)
}

func (dao *AuthsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.AuthsDO) (int64, int64, error) {
	params, err := paramsValue(do.Params)
	if err != nil {
		return 0, 0, err
	}
	return upsertAuths(ctx, dao.db, `
		INSERT INTO auths
		(auth_key_id, api_id, device_model, system_version, app_version,
		 system_lang_code, lang_pack, lang_code, proxy, params, client_ip, date_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
		 COALESCE($10::jsonb, 'null'::jsonb), NULLIF($11, '')::inet, $12)
		ON CONFLICT (auth_key_id) DO UPDATE SET
		api_id = EXCLUDED.api_id, device_model = EXCLUDED.device_model,
		system_version = EXCLUDED.system_version, app_version = EXCLUDED.app_version,
		system_lang_code = EXCLUDED.system_lang_code, lang_pack = EXCLUDED.lang_pack,
		lang_code = EXCLUDED.lang_code, proxy = EXCLUDED.proxy, params = EXCLUDED.params,
		client_ip = EXCLUDED.client_ip, date_active = EXCLUDED.date_active, deleted = FALSE
		RETURNING id`,
		do.AuthKeyId, do.ApiId, do.DeviceModel, do.SystemVersion, do.AppVersion,
		do.SystemLangCode, do.LangPack, do.LangCode, do.Proxy, params, do.ClientIp, do.DateActive)
}

func (dao *AuthsDAO) InsertOrUpdateTx(ctx context.Context, tx pgx.Tx, do *dataobject.AuthsDO) (int64, int64, error) {
	return NewAuthsDAO(tx).InsertOrUpdate(ctx, do)
}

func upsertAuths(ctx context.Context, db DB, query string, args ...any) (int64, int64, error) {
	return insertResult(ctx, db, query, args...)
}

func (dao *AuthsDAO) SelectByAuthKeyId(ctx context.Context, authKeyID int64) (*dataobject.AuthsDO, error) {
	return scanAuths(dao.db.QueryRow(ctx, `
		SELECT auth_key_id, layer, api_id, device_model, system_version,
		app_version, system_lang_code, lang_pack, lang_code, proxy, params,
		COALESCE(client_ip::text, ''), date_active
		FROM auths WHERE auth_key_id = $1 AND deleted = FALSE LIMIT 1`, authKeyID))
}
