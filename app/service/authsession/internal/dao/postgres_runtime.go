package dao

// This file is the PostgreSQL runtime adapter for authsession.  The generated
// MySQL DAO remains available for isolated legacy tests, but production
// service construction selects this adapter whenever Postgres.DSN is set.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oschwald/geoip2-golang"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/config"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// Postgres is the authoritative authsession runtime implementation.
type Postgres struct {
	store *postgres_dao.Store
	pool  *pgxpool.Pool
	kv    kv.Store
	MMDB  *geoip2.Reader
}

// NewPostgres opens and health-checks the service-owned PostgreSQL pool.
func NewPostgres(c config.Config) (*Postgres, error) {
	pool, err := postgres.NewPool(context.Background(), c.Postgres)
	if err != nil {
		return nil, err
	}
	if len(c.KV) == 0 {
		pool.Close()
		return nil, errors.New("authsession: at least one Redis KV node is required")
	}
	for i := range c.KV {
		if c.KV[i].Host == "" {
			pool.Close()
			return nil, errors.New("authsession: Redis KV node host is required")
		}
		if c.KV[i].Type == "" {
			c.KV[i].Type = redis.NodeType
		}
		if c.KV[i].Weight <= 0 {
			c.KV[i].Weight = 100
		}
	}
	mmdbReader, _ := geoip2.Open(mmdb)
	return &Postgres{
		store: postgres_dao.NewStore(pool),
		pool:  pool,
		kv:    kv.NewStore(c.KV),
		MMDB:  mmdbReader,
	}, nil
}

func (d *Postgres) Close() {
	if d == nil {
		return
	}
	if d.MMDB != nil {
		_ = d.MMDB.Close()
	}
	if d.pool != nil {
		d.pool.Close()
	}
}

func (d *Postgres) keyInfo(ctx context.Context, authKeyID int64) (*mtproto.AuthKeyInfo, error) {
	info, err := d.store.AuthKeyInfos.SelectByAuthKeyId(ctx, authKeyID)
	if err != nil {
		return nil, err
	}
	key, err := d.store.AuthKeys.SelectByAuthKeyId(ctx, authKeyID)
	if err != nil {
		return nil, err
	}
	if info == nil || key == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	body, err := base64.RawStdEncoding.DecodeString(key.Body)
	if err != nil {
		return nil, fmt.Errorf("decode auth key: %w", err)
	}
	return mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{
		AuthKeyId:          authKeyID,
		AuthKey:            body,
		AuthKeyType:        info.AuthKeyType,
		PermAuthKeyId:      info.PermAuthKeyId,
		TempAuthKeyId:      info.TempAuthKeyId,
		MediaTempAuthKeyId: info.MediaTempAuthKeyId,
	}).To_AuthKeyInfo(), nil
}

func (d *Postgres) QueryAuthKeyV2(ctx context.Context, authKeyID int64) (*mtproto.AuthKeyInfo, error) {
	return d.keyInfo(ctx, authKeyID)
}

func (d *Postgres) SetAuthKeyV2(ctx context.Context, key *mtproto.AuthKeyInfo, _ int32) error {
	if key == nil || key.AuthKeyId == 0 || len(key.AuthKey) == 0 {
		return fmt.Errorf("invalid auth key")
	}
	return postgres.WithTx(ctx, d.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_keys (auth_key_id, body, deleted) VALUES ($1, $2, FALSE)
			ON CONFLICT (auth_key_id) DO UPDATE SET body = EXCLUDED.body, deleted = FALSE`,
			key.AuthKeyId, key.AuthKey); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO auth_key_infos
			(auth_key_id, auth_key_type, perm_auth_key_id, temp_auth_key_id, media_temp_auth_key_id, deleted)
			VALUES ($1, $2, $3, $4, $5, FALSE)
			ON CONFLICT (auth_key_id) DO UPDATE SET
			auth_key_type = EXCLUDED.auth_key_type,
			perm_auth_key_id = EXCLUDED.perm_auth_key_id,
			temp_auth_key_id = EXCLUDED.temp_auth_key_id,
			media_temp_auth_key_id = EXCLUDED.media_temp_auth_key_id,
			deleted = FALSE`, key.AuthKeyId, key.AuthKeyType, key.PermAuthKeyId,
			key.TempAuthKeyId, key.MediaTempAuthKeyId)
		return err
	})
}

func (d *Postgres) BindTempAuthKeyV2(ctx context.Context, permAuthKeyID, tempAuthKeyID int64, tempType int32) error {
	if permAuthKeyID == 0 || tempAuthKeyID == 0 || permAuthKeyID == tempAuthKeyID {
		return fmt.Errorf("invalid temporary auth key binding")
	}
	if tempType != mtproto.AuthKeyTypeTemp && tempType != mtproto.AuthKeyTypeMediaTemp {
		return fmt.Errorf("invalid temporary auth key type: %d", tempType)
	}
	return postgres.WithTx(ctx, d.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var permType, tempActualType int32
		var tempOwner, permCurrent int64
		if err := tx.QueryRow(ctx, `SELECT auth_key_type, temp_auth_key_id, media_temp_auth_key_id FROM auth_key_infos WHERE auth_key_id = $1 AND deleted = FALSE FOR UPDATE`, permAuthKeyID).
			Scan(&permType, &permCurrent, new(int64)); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT auth_key_type, perm_auth_key_id FROM auth_key_infos WHERE auth_key_id = $1 AND deleted = FALSE FOR UPDATE`, tempAuthKeyID).
			Scan(&tempActualType, &tempOwner); err != nil {
			return err
		}
		if tempActualType != tempType {
			return fmt.Errorf("temporary auth key type mismatch: got %d, want %d", tempActualType, tempType)
		}
		if tempOwner != 0 && tempOwner != permAuthKeyID {
			return ErrTempAuthKeyAlreadyBound
		}
		field := "temp_auth_key_id"
		if tempType == mtproto.AuthKeyTypeMediaTemp {
			field = "media_temp_auth_key_id"
		}
		if _, err := tx.Exec(ctx, `UPDATE auth_key_infos SET `+field+` = $1 WHERE auth_key_id = $2 AND deleted = FALSE`, tempAuthKeyID, permAuthKeyID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE auth_key_infos SET perm_auth_key_id = $1 WHERE auth_key_id = $2 AND deleted = FALSE`, permAuthKeyID, tempAuthKeyID)
		return err
	})
}

func (d *Postgres) BindAuthKeyUser(ctx context.Context, authKeyID, userID int64) (int64, error) {
	if authKeyID == 0 || userID <= 0 {
		return 0, fmt.Errorf("invalid auth key binding")
	}
	var hash int64
	if err := binaryInt64(&hash); err != nil {
		return 0, err
	}
	err := postgres.WithTx(ctx, d.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var ignored int64
		if err := tx.QueryRow(ctx, `SELECT auth_key_id FROM auth_key_infos WHERE auth_key_id = $1 AND deleted = FALSE FOR UPDATE`, authKeyID).Scan(&ignored); err != nil {
			return err
		}
		var existingHash, existingUser, created int64
		err := tx.QueryRow(ctx, `SELECT user_id, hash, date_created FROM auth_users WHERE auth_key_id = $1 AND deleted = FALSE FOR UPDATE`, authKeyID).Scan(&existingUser, &existingHash, &created)
		switch {
		case err == nil:
			if existingUser != userID {
				return ErrAuthKeyOwnedByAnotherUser
			}
			hash = existingHash
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		now := time.Now().Unix()
		_, err = tx.Exec(ctx, `
			INSERT INTO auth_users (auth_key_id, user_id, hash, date_created, date_active, deleted)
			VALUES ($1, $2, $3, $4, $4, FALSE)`, authKeyID, userID, hash, now)
		if err != nil {
			if postgres.IsUniqueViolation(err) {
				return ErrAuthKeyOwnedByAnotherUser
			}
			return err
		}
		return nil
	})
	return hash, err
}

func (d *Postgres) UnbindAuthUser(ctx context.Context, authKeyID, userID int64) error {
	if authKeyID == 0 {
		_, err := d.pool.Exec(ctx, `UPDATE auth_users SET deleted = TRUE, date_active = 0 WHERE user_id = $1 AND deleted = FALSE`, userID)
		return err
	}
	_, err := d.pool.Exec(ctx, `UPDATE auth_users SET deleted = TRUE, date_active = 0 WHERE auth_key_id = $1 AND user_id = $2 AND deleted = FALSE`, authKeyID, userID)
	return err
}

func (d *Postgres) SetClientSessionInfo(ctx context.Context, session *authsession.ClientSession) error {
	if session == nil {
		return fmt.Errorf("nil client session")
	}
	params := session.GetParams()
	if strings.TrimSpace(params) == "" {
		params = "null"
	}
	_, err := d.pool.Exec(ctx, `
		INSERT INTO auths (auth_key_id, layer, api_id, device_model, system_version, app_version,
		 system_lang_code, lang_pack, lang_code, proxy, params, client_ip, date_active, deleted)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$12::jsonb,$11,$13,FALSE)
		ON CONFLICT (auth_key_id) DO UPDATE SET layer=EXCLUDED.layer, api_id=EXCLUDED.api_id,
		 device_model=EXCLUDED.device_model, system_version=EXCLUDED.system_version,
		 app_version=EXCLUDED.app_version, system_lang_code=EXCLUDED.system_lang_code,
		 lang_pack=EXCLUDED.lang_pack, lang_code=EXCLUDED.lang_code, proxy=EXCLUDED.proxy,
		 params=EXCLUDED.params, client_ip=EXCLUDED.client_ip, date_active=EXCLUDED.date_active, deleted=FALSE`,
		session.GetAuthKeyId(), session.GetLayer(), session.GetApiId(), session.GetDeviceModel(),
		session.GetSystemVersion(), session.GetAppVersion(), session.GetSystemLangCode(), session.GetLangPack(),
		session.GetLangCode(), session.GetProxy(), session.GetIp(), params, time.Now().Unix())
	return err
}

func (d *Postgres) SetLayer(ctx context.Context, in *authsession.TLAuthsessionSetLayer) error {
	if in == nil {
		return fmt.Errorf("nil set layer request")
	}
	_, err := d.pool.Exec(ctx, `
		INSERT INTO auths (auth_key_id, layer, client_ip, date_active, params, deleted)
		VALUES ($1,$2,NULLIF($3, '')::inet,$4,'null'::jsonb,FALSE)
		ON CONFLICT (auth_key_id) DO UPDATE SET layer=EXCLUDED.layer,
		 client_ip=EXCLUDED.client_ip,date_active=EXCLUDED.date_active,deleted=FALSE`,
		in.GetAuthKeyId(), in.GetLayer(), in.GetIp(), time.Now().Unix())
	return err
}

func (d *Postgres) SetInitConnection(ctx context.Context, in *authsession.TLAuthsessionSetInitConnection) error {
	if in == nil {
		return fmt.Errorf("nil init connection request")
	}
	params := in.GetParams()
	if strings.TrimSpace(params) == "" {
		params = "null"
	}
	_, err := d.pool.Exec(ctx, `
		INSERT INTO auths (auth_key_id, api_id, device_model, system_version, app_version,
		 system_lang_code, lang_pack, lang_code, proxy, params, client_ip, date_active, deleted)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,NULLIF($11, '')::inet,$12,FALSE)
		ON CONFLICT (auth_key_id) DO UPDATE SET api_id=EXCLUDED.api_id,
		 device_model=EXCLUDED.device_model,system_version=EXCLUDED.system_version,
		 app_version=EXCLUDED.app_version,system_lang_code=EXCLUDED.system_lang_code,
		 lang_pack=EXCLUDED.lang_pack,lang_code=EXCLUDED.lang_code,proxy=EXCLUDED.proxy,
		 params=EXCLUDED.params,client_ip=EXCLUDED.client_ip,date_active=EXCLUDED.date_active,deleted=FALSE`,
		in.GetAuthKeyId(), in.GetApiId(), in.GetDeviceModel(), in.GetSystemVersion(), in.GetAppVersion(),
		in.GetSystemLangCode(), in.GetLangPack(), in.GetLangCode(), in.GetProxy(), params, in.GetIp(), time.Now().Unix())
	return err
}

func (d *Postgres) SetAndroidPushSessionId(ctx context.Context, userID, keyID, sessionID int64) error {
	_, err := d.pool.Exec(ctx, `UPDATE auth_users SET android_push_session_id = $1 WHERE auth_key_id = $2 AND user_id = $3 AND deleted = FALSE`, sessionID, keyID, userID)
	return err
}

type pgAuthData struct {
	client *authsession.ClientSession
	user   *pgBindUser
}

type pgBindUser struct {
	userID, hash, created, active, pushID int64
}

func (d *Postgres) getAuthData(ctx context.Context, keyID int64) (*pgAuthData, error) {
	var out pgAuthData
	var storedKeyID int64
	var ip string
	var params []byte
	var layer, apiID int32
	var device, system, app, systemLang, langPack, langCode, proxy string
	var active int64
	err := d.pool.QueryRow(ctx, `SELECT auth_key_id, layer, api_id, device_model, system_version,
		app_version, system_lang_code, lang_pack, lang_code, proxy, params,
		COALESCE(client_ip::text,''), date_active FROM auths WHERE auth_key_id=$1 AND deleted=FALSE`, keyID).
		Scan(&storedKeyID, &layer, &apiID, &device, &system, &app, &systemLang, &langPack, &langCode, &proxy, &params, &ip, &active)
	if err != nil {
		return nil, err
	}
	keyID = storedKeyID
	out.client = authsession.MakeTLClientSession(&authsession.ClientSession{
		AuthKeyId: keyID, Ip: ip, Layer: layer, ApiId: apiID, DeviceModel: device,
		SystemVersion: system, AppVersion: app, SystemLangCode: systemLang,
		LangPack: langPack, LangCode: langCode, Proxy: proxy, Params: string(params),
	}).To_ClientSession()
	var u pgBindUser
	if err := d.pool.QueryRow(ctx, `SELECT user_id, hash, date_created, date_active, android_push_session_id
		FROM auth_users WHERE auth_key_id=$1 AND deleted=FALSE`, keyID).
		Scan(&u.userID, &u.hash, &u.created, &u.active, &u.pushID); err == nil {
		out.user = &u
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return &out, nil
}

func (d *Postgres) GetAuthKeyUserId(ctx context.Context, keyID int64) int64 {
	data, err := d.getAuthData(ctx, keyID)
	if err != nil || data == nil || data.user == nil {
		return 0
	}
	return data.user.userID
}

func (d *Postgres) GetCacheAuthData(ctx context.Context, keyID int64) (*CacheAuthData, error) {
	data, err := d.getAuthData(ctx, keyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &CacheAuthData{}, sqlc.ErrNotFound
		}
		return nil, err
	}
	result := &CacheAuthData{Client: data.client}
	if data.user != nil {
		result.BindUser = &BindUser{
			UserId: data.user.userID, Hash: data.user.hash,
			DateCreated: data.user.created, DateActivated: data.user.active,
			AndroidPushSessionId: data.user.pushID,
		}
	}
	return result, nil
}

func (d *Postgres) GetApiLayer(ctx context.Context, keyID int64) int32 {
	data, _ := d.getAuthData(ctx, keyID)
	if data == nil || data.client == nil {
		return 0
	}
	return data.client.GetLayer()
}
func (d *Postgres) GetLangCode(ctx context.Context, keyID int64) string {
	data, _ := d.getAuthData(ctx, keyID)
	if data == nil || data.client == nil {
		return "en"
	}
	return data.client.GetLangCode()
}
func (d *Postgres) GetLangPack(ctx context.Context, keyID int64) string {
	data, _ := d.getAuthData(ctx, keyID)
	if data == nil || data.client == nil {
		return ""
	}
	return data.client.GetLangPack()
}
func (d *Postgres) GetClient(ctx context.Context, keyID int64) string {
	data, _ := d.getAuthData(ctx, keyID)
	if data == nil || data.client == nil {
		return ""
	}
	c := data.client.GetLangPack()
	if c == "android" && strings.Contains(data.client.GetAppVersion(), "TDLib") {
		return "react"
	}
	if c == "" && (strings.HasSuffix(data.client.GetAppVersion(), " A") || strings.HasSuffix(data.client.GetAppVersion(), " Z")) {
		return "weba"
	}
	return c
}

func (d *Postgres) GetAuthorization(ctx context.Context, keyID int64) (*mtproto.Authorization, error) {
	data, err := d.getAuthData(ctx, keyID)
	if err != nil {
		return nil, err
	}
	if data.user == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	country, region := d.countryRegion(data.client.GetIp())
	return mtproto.MakeTLAuthorization(&mtproto.Authorization{
		OfficialApp: true, Hash: data.user.hash, DeviceModel: data.client.GetDeviceModel(),
		SystemVersion: data.client.GetSystemVersion(), ApiId: data.client.GetApiId(),
		AppName: data.client.GetLangPack(), AppVersion: data.client.GetAppVersion(),
		DateCreated: int32(data.user.created), DateActive: int32(data.user.active),
		Ip: data.client.GetIp(), Country: country, Region: region,
	}).To_Authorization(), nil
}

func (d *Postgres) GetAuthorizations(ctx context.Context, userID, excludeKeyID int64) []*mtproto.Authorization {
	rows, err := d.pool.Query(ctx, `SELECT auth_key_id FROM auth_users WHERE user_id=$1 AND deleted=FALSE ORDER BY id`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := make([]*mtproto.Authorization, 0)
	for rows.Next() {
		var keyID int64
		if rows.Scan(&keyID) != nil {
			continue
		}
		data, err := d.getAuthData(ctx, keyID)
		if err != nil || data == nil || data.user == nil {
			continue
		}
		country, region := d.countryRegion(data.client.GetIp())
		a := mtproto.MakeTLAuthorization(&mtproto.Authorization{
			OfficialApp: true, Hash: data.user.hash, DeviceModel: data.client.GetDeviceModel(),
			SystemVersion: data.client.GetSystemVersion(), ApiId: data.client.GetApiId(),
			AppName: data.client.GetLangPack(), AppVersion: data.client.GetAppVersion(),
			DateCreated: int32(data.user.created), DateActive: int32(data.user.active),
			Ip: data.client.GetIp(), Country: country, Region: region,
		}).To_Authorization()
		if keyID == excludeKeyID {
			a.Current = true
			a.Hash = 0
			result = append([]*mtproto.Authorization{a}, result...)
		} else {
			result = append(result, a)
		}
	}
	return result
}

func (d *Postgres) ResetAuthorization(ctx context.Context, userID, currentKeyID, hash int64) ([]int64, error) {
	ids, keys := make([]int64, 0), make([]int64, 0)
	err := withPostgresRetry(ctx, func() error {
		ids = ids[:0]
		keys = keys[:0]
		return postgres.WithTx(ctx, d.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id, auth_key_id, hash FROM auth_users WHERE user_id=$1 AND deleted=FALSE FOR UPDATE`, userID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id, key, rowHash int64
				if err := rows.Scan(&id, &key, &rowHash); err != nil {
					return err
				}
				if (hash == 0 && key != currentKeyID) || (hash != 0 && rowHash == hash && key != currentKeyID) {
					ids = append(ids, id)
					keys = append(keys, key)
				}
			}
			if err := rows.Err(); err != nil {
				return err
			}
			if len(ids) == 0 {
				return nil
			}
			_, err = tx.Exec(ctx, `UPDATE auth_users SET deleted=TRUE, date_active=0 WHERE id = ANY($1::bigint[])`, ids)
			return err
		})
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (d *Postgres) countryRegion(ip string) (string, string) {
	if d.MMDB == nil {
		return "UNKNOWN", ""
	}
	r, err := d.MMDB.City(net.ParseIP(ip))
	if err != nil {
		return "UNKNOWN", ""
	}
	return r.City.Names["en"] + ", " + r.Country.Names["en"], r.Country.IsoCode
}

const pgSaltPrefix = "salts"

func (d *Postgres) PutSaltCache(ctx context.Context, keyID int64, salt *mtproto.TLFutureSalt) error {
	return d.putSalts(ctx, keyID, []*mtproto.TLFutureSalt{salt})
}

func (d *Postgres) putSalts(ctx context.Context, keyID int64, salts []*mtproto.TLFutureSalt) error {
	b, err := json.Marshal(salts)
	if err != nil {
		return err
	}
	return d.kv.SetexCtx(ctx, fmt.Sprintf("%s_%d", pgSaltPrefix, keyID), string(b), len(salts)*saltTimeout)
}

func (d *Postgres) getSalts(ctx context.Context, keyID int64) ([]*mtproto.TLFutureSalt, error) {
	b, err := d.kv.GetCtx(ctx, fmt.Sprintf("%s_%d", pgSaltPrefix, keyID))
	if err != nil || b == "" {
		return nil, err
	}
	var salts []*mtproto.TLFutureSalt
	if err := jsonx.UnmarshalFromString(b, &salts); err != nil {
		return nil, err
	}
	return salts, nil
}

func (d *Postgres) GetFutureSalts(ctx context.Context, keyID int64, num int32) (*mtproto.TLFutureSalts, error) {
	if num <= 0 {
		return nil, fmt.Errorf("invalid salt count")
	}
	old, err := d.getSalts(ctx, keyID)
	if err != nil {
		return nil, err
	}
	now := int32(time.Now().Unix())
	active := make([]*mtproto.TLFutureSalt, 0, num)
	var previous *mtproto.TLFutureSalt
	last := now
	for i, salt := range old {
		if salt.GetValidUntil() < now {
			continue
		}
		if previous == nil && i > 0 {
			previous = old[i-1]
		}
		active = append(active, salt)
		if salt.GetValidUntil() > last {
			last = salt.GetValidUntil()
		}
	}
	if previous == nil && len(active) == 0 && len(old) > 0 {
		previous = old[len(old)-1]
	}
	if previous != nil && previous.GetValidUntil()+300 < now {
		previous = nil
	}
	for int32(len(active)) < num {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		salt := int64(0)
		for _, x := range b {
			salt = (salt << 8) | int64(x)
		}
		active = append(active, mtproto.MakeTLFutureSalt(&mtproto.FutureSalt{ValidSince: last, ValidUntil: last + saltTimeout, Salt: salt}))
		last += saltTimeout
	}
	returned := active
	if int32(len(returned)) > num {
		returned = returned[:num]
	}
	result := returned
	if previous != nil {
		result = append([]*mtproto.TLFutureSalt{previous}, result...)
	}
	persisted := active
	if previous != nil {
		persisted = append([]*mtproto.TLFutureSalt{previous}, persisted...)
	}
	if err := d.putSalts(ctx, keyID, persisted); err != nil {
		return nil, err
	}
	return &mtproto.TLFutureSalts{Data2: &mtproto.FutureSalts{ReqMsgId: 0, Now: now, Salts: result}}, nil
}

func binaryInt64(out *int64) error {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	for _, x := range b {
		*out = (*out << 8) | int64(x)
	}
	if *out <= 0 {
		*out = 1
	}
	return nil
}

func withPostgresRetry(ctx context.Context, fn func() error) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := fn()
		if err == nil || !postgres.IsSerializationFailure(err) || attempt == 2 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}
