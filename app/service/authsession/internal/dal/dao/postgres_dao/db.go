// Package postgres_dao contains the PostgreSQL persistence for authsession.
//
// It intentionally does not depend on the Teamgram MySQL wrapper. The public
// service layer can migrate to these DAOs while the old generated package is
// removed after PostgreSQL acceptance.
package postgres_dao

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
)

// DB is implemented by pgxpool.Pool and pgx.Tx. Keeping the narrow interface
// makes each DAO usable in the same transaction as the owning mutation.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	Pool         *pgxpool.Pool
	Auths        *AuthsDAO
	AuthKeys     *AuthKeysDAO
	AuthKeyInfos *AuthKeyInfosDAO
	AuthUsers    *AuthUsersDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Pool:         pool,
		Auths:        NewAuthsDAO(pool),
		AuthKeys:     NewAuthKeysDAO(pool),
		AuthKeyInfos: NewAuthKeyInfosDAO(pool),
		AuthUsers:    NewAuthUsersDAO(pool),
	}
}

func result(tag pgconn.CommandTag) (lastInsertID, rowsAffected int64) {
	return 0, tag.RowsAffected()
}

func insertResult(ctx context.Context, db DB, query string, args ...any) (lastInsertID, rowsAffected int64, err error) {
	err = db.QueryRow(ctx, query, args...).Scan(&lastInsertID)
	if err == nil {
		rowsAffected = 1
	}
	return
}

func paramsValue(params string) (any, error) {
	if strings.TrimSpace(params) == "" {
		return nil, nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(params), &raw); err != nil {
		return nil, fmt.Errorf("authsession: invalid params JSON: %w", err)
	}
	return raw, nil
}

func bodyValue(body string) []byte {
	decoded, err := base64.RawStdEncoding.DecodeString(body)
	if err == nil {
		return decoded
	}
	// The old store used base64 text. Accept standard padded base64 as well so
	// a freshly provisioned PostgreSQL store can be populated by a tool that
	// emits padded values.
	if decoded, err = base64.StdEncoding.DecodeString(body); err == nil {
		return decoded
	}
	return []byte(body)
}

func bodyString(body []byte) string {
	return base64.RawStdEncoding.EncodeToString(body)
}

func scanAuths(row pgx.Row) (*dataobject.AuthsDO, error) {
	do := new(dataobject.AuthsDO)
	var params []byte
	var clientIP string
	err := row.Scan(
		&do.AuthKeyId, &do.Layer, &do.ApiId, &do.DeviceModel,
		&do.SystemVersion, &do.AppVersion, &do.SystemLangCode,
		&do.LangPack, &do.LangCode, &do.Proxy, &params,
		&clientIP, &do.DateActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	do.ClientIp = clientIP
	do.Params = string(params)
	return do, nil
}

func scanAuthUsers(row pgx.Row) (*dataobject.AuthUsersDO, error) {
	do := new(dataobject.AuthUsersDO)
	err := row.Scan(
		&do.Id, &do.AuthKeyId, &do.UserId, &do.Hash,
		&do.DateCreated, &do.DateActive, &do.AndroidPushSessionId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return do, err
}

func scanAuthKeyInfos(row pgx.Row) (*dataobject.AuthKeyInfosDO, error) {
	do := new(dataobject.AuthKeyInfosDO)
	err := row.Scan(&do.AuthKeyId, &do.AuthKeyType, &do.PermAuthKeyId, &do.TempAuthKeyId, &do.MediaTempAuthKeyId)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return do, err
}

func scanAuthKeys(row pgx.Row) (*dataobject.AuthKeysDO, error) {
	do := new(dataobject.AuthKeysDO)
	var body []byte
	err := row.Scan(&do.AuthKeyId, &body)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	do.Body = bodyString(body)
	return do, nil
}
