// Copyright 2025 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/config"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
)

const (
	SessionRegistration = "registration"
	SessionLogin        = "login"
)

var (
	ErrUnavailable        = errors.New("passkey persistence is unavailable")
	ErrSessionNotFound    = errors.New("passkey ceremony session not found")
	ErrSessionUsed        = errors.New("passkey ceremony session already used")
	ErrCredentialNotFound = errors.New("passkey credential not found")
	ErrCredentialConflict = errors.New("passkey credential already belongs to another active registration")
	ErrCounterReplay      = errors.New("passkey authenticator counter replay")
)

type Session struct {
	Challenge string
	UserID    int64
	Kind      string
	Data      []byte
	ExpiresAt int64
}

type Credential struct {
	ID            []byte
	UserID        int64
	Name          string
	Date          int64
	LastUsageDate int64
	SignCount     uint32
	Data          []byte
}

type Dao struct {
	DB                *pgxpool.Pool
	UserClient        user_client.UserClient
	AuthsessionClient authsession_client.AuthsessionClient
}

func New(c config.Config) *Dao {
	db, err := openPostgres(c.PostgresDSN)
	if err != nil {
		panic(err)
	}
	return &Dao{
		DB:                db,
		UserClient:        user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		AuthsessionClient: authsession_client.NewAuthsessionClient(rpcx.GetCachedRpcClient(c.AuthSessionClient)),
	}
}

func openPostgres(dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("passkey: PostgresDSN is required")
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("passkey: parse PostgreSQL DSN: %w", err)
	}
	poolConfig.MaxConns = 8
	poolConfig.MinConns = 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("passkey: open PostgreSQL: %w", err)
	}
	if err = db.Ping(ctx); err == nil {
		var version int
		err = db.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version)
		if err == nil && version/10000 != 18 {
			err = fmt.Errorf("PostgreSQL 18 is required (server_version_num=%d)", version)
		}
	}
	if err == nil {
		for _, query := range []string{
			`SELECT challenge,user_id,kind,data,expires_at,used,created_at FROM apifull_passkey_session LIMIT 0`,
			`SELECT credential_id,user_id,name,date_created,last_usage_date,sign_count,credential,deleted FROM apifull_passkey_credential LIMIT 0`,
		} {
			if _, err = db.Exec(ctx, query); err != nil {
				break
			}
		}
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("passkey: verify PostgreSQL persistence: %w", err)
	}
	return db, nil
}

func (d *Dao) available() error {
	if d == nil || d.DB == nil {
		return ErrUnavailable
	}
	return nil
}

func validSessionKind(kind string) bool {
	return kind == SessionRegistration || kind == SessionLogin
}

func (d *Dao) PutSession(ctx context.Context, session *Session) error {
	if err := d.available(); err != nil {
		return err
	}
	if session == nil || session.Challenge == "" || !validSessionKind(session.Kind) || session.UserID < 0 || len(session.Data) == 0 || session.ExpiresAt <= time.Now().Unix() {
		return fmt.Errorf("invalid passkey session")
	}
	tx, err := d.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Used and expired ceremonies are no longer useful. Prune them in the
	// same transaction as the new challenge to keep this table bounded while
	// retaining the one-time challenge invariant.
	if _, err = tx.Exec(ctx, `
		DELETE FROM apifull_passkey_session WHERE used = TRUE OR expires_at <= $1
	`, time.Now().Unix()); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO apifull_passkey_session (challenge, user_id, kind, data, expires_at, used, created_at)
		VALUES ($1, $2, $3, $4, $5, FALSE, $6)
	`, session.Challenge, session.UserID, session.Kind, session.Data, session.ExpiresAt, time.Now().Unix())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *Dao) GetSession(ctx context.Context, challenge, kind string) (*Session, error) {
	if err := d.available(); err != nil {
		return nil, err
	}
	if challenge == "" || kind == "" {
		return nil, ErrSessionNotFound
	}
	var session Session
	var used bool
	err := d.DB.QueryRow(ctx, `
		SELECT challenge, user_id, kind, data, expires_at, used
		FROM apifull_passkey_session WHERE challenge = $1 AND kind = $2
	`, challenge, kind).Scan(&session.Challenge, &session.UserID, &session.Kind, &session.Data, &session.ExpiresAt, &used)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	if used || session.ExpiresAt <= time.Now().Unix() {
		return nil, ErrSessionUsed
	}
	return &session, nil
}

func (d *Dao) ConsumeSession(ctx context.Context, challenge, kind string) error {
	if err := d.available(); err != nil {
		return err
	}
	result, err := d.DB.Exec(ctx, `
		UPDATE apifull_passkey_session SET used = TRUE
		WHERE challenge = $1 AND kind = $2 AND used = FALSE AND expires_at > $3
	`, challenge, kind, time.Now().Unix())
	if err != nil {
		return err
	}
	n := result.RowsAffected()
	if n != 1 {
		return ErrSessionUsed
	}
	return nil
}

func (d *Dao) ListCredentials(ctx context.Context, userID int64) ([]Credential, error) {
	if err := d.available(); err != nil {
		return nil, err
	}
	rows, err := d.DB.Query(ctx, `
		SELECT credential_id, user_id, name, date_created, last_usage_date, sign_count, credential
		FROM apifull_passkey_credential WHERE user_id = $1 AND deleted = FALSE ORDER BY date_created, credential_id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var credentials []Credential
	for rows.Next() {
		var credential Credential
		var count uint64
		if err = rows.Scan(&credential.ID, &credential.UserID, &credential.Name, &credential.Date, &credential.LastUsageDate, &count, &credential.Data); err != nil {
			return nil, err
		}
		credential.SignCount = uint32(count)
		credentials = append(credentials, credential)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return credentials, nil
}

func (d *Dao) GetCredential(ctx context.Context, id []byte) (*Credential, error) {
	if err := d.available(); err != nil {
		return nil, err
	}
	if len(id) == 0 {
		return nil, ErrCredentialNotFound
	}
	var credential Credential
	var count uint64
	err := d.DB.QueryRow(ctx, `
		SELECT credential_id, user_id, name, date_created, last_usage_date, sign_count, credential
		FROM apifull_passkey_credential WHERE credential_id = $1 AND deleted = FALSE
	`, id).Scan(&credential.ID, &credential.UserID, &credential.Name, &credential.Date, &credential.LastUsageDate, &count, &credential.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCredentialNotFound
	}
	if err != nil {
		return nil, err
	}
	credential.SignCount = uint32(count)
	return &credential, nil
}

func (d *Dao) InsertCredential(ctx context.Context, credential *Credential) error {
	if err := d.available(); err != nil {
		return err
	}
	if credential == nil || credential.UserID <= 0 || len(credential.ID) == 0 || len(credential.Data) == 0 {
		return fmt.Errorf("invalid passkey credential")
	}
	_, err := d.DB.Exec(ctx, `
		INSERT INTO apifull_passkey_credential
		(credential_id, user_id, name, date_created, last_usage_date, sign_count, credential, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE)
	`, credential.ID, credential.UserID, credential.Name, credential.Date, credential.LastUsageDate, credential.SignCount, credential.Data)
	return err
}

// ConsumeAndInsertCredential commits the registration ceremony and credential
// in one transaction so a failed insert cannot leave the challenge unusable.
func (d *Dao) ConsumeAndInsertCredential(ctx context.Context, challenge, kind string, credential *Credential) error {
	if err := d.available(); err != nil {
		return err
	}
	if challenge == "" || kind == "" || credential == nil || credential.UserID <= 0 || len(credential.ID) == 0 || len(credential.Data) == 0 {
		return fmt.Errorf("invalid passkey registration")
	}
	tx, err := d.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var expiresAt int64
	var used bool
	var userID int64
	err = tx.QueryRow(ctx, `
		SELECT user_id, expires_at, used FROM apifull_passkey_session
		WHERE challenge = $1 AND kind = $2 FOR UPDATE
	`, challenge, kind).Scan(&userID, &expiresAt, &used)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return err
	}
	if used || expiresAt <= time.Now().Unix() || userID != credential.UserID {
		return ErrSessionUsed
	}
	if _, err = tx.Exec(ctx, `UPDATE apifull_passkey_session SET used = TRUE WHERE challenge = $1 AND kind = $2`, challenge, kind); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO apifull_passkey_credential
		(credential_id, user_id, name, date_created, last_usage_date, sign_count, credential, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE)
		ON CONFLICT (credential_id) DO UPDATE
		SET user_id = EXCLUDED.user_id,
		    name = EXCLUDED.name,
		    date_created = EXCLUDED.date_created,
		    last_usage_date = EXCLUDED.last_usage_date,
		    sign_count = EXCLUDED.sign_count,
		    credential = EXCLUDED.credential,
		    deleted = FALSE
		WHERE apifull_passkey_credential.deleted = TRUE
		  AND apifull_passkey_credential.user_id = EXCLUDED.user_id
	`, credential.ID, credential.UserID, credential.Name, credential.Date, credential.LastUsageDate, credential.SignCount, credential.Data)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrCredentialConflict
	}
	return tx.Commit(ctx)
}

func (d *Dao) UpdateCredential(ctx context.Context, id []byte, expectedCount uint32, nextCount uint32, data []byte, usageDate int64) error {
	if err := d.available(); err != nil {
		return err
	}
	result, err := d.DB.Exec(ctx, `
		UPDATE apifull_passkey_credential
		SET sign_count = $1, credential = $2, last_usage_date = $3
		WHERE credential_id = $4 AND deleted = FALSE AND sign_count = $5
	`, nextCount, data, usageDate, id, expectedCount)
	if err != nil {
		return err
	}
	n := result.RowsAffected()
	if n != 1 {
		return ErrCounterReplay
	}
	return nil
}

// ConsumeLoginAndUpdateCredential atomically consumes the login challenge and
// advances the authenticator counter, preventing assertion replay.
func (d *Dao) ConsumeLoginAndUpdateCredential(ctx context.Context, challenge, kind string, id []byte, expectedCount, nextCount uint32, data []byte, usageDate int64) error {
	if err := d.available(); err != nil {
		return err
	}
	if challenge == "" || kind == "" || len(id) == 0 || len(data) == 0 {
		return fmt.Errorf("invalid passkey login")
	}
	tx, err := d.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var used bool
	var expiresAt int64
	if err = tx.QueryRow(ctx, `
		SELECT expires_at, used FROM apifull_passkey_session
		WHERE challenge = $1 AND kind = $2 FOR UPDATE
	`, challenge, kind).Scan(&expiresAt, &used); errors.Is(err, pgx.ErrNoRows) {
		return ErrSessionNotFound
	} else if err != nil {
		return err
	}
	if used || expiresAt <= time.Now().Unix() {
		return ErrSessionUsed
	}
	result, err := tx.Exec(ctx, `
		UPDATE apifull_passkey_credential
		SET sign_count = $1, credential = $2, last_usage_date = $3
		WHERE credential_id = $4 AND deleted = FALSE AND sign_count = $5
	`, nextCount, data, usageDate, id, expectedCount)
	if err != nil {
		return err
	}
	if n := result.RowsAffected(); n != 1 {
		return ErrCounterReplay
	}
	if _, err = tx.Exec(ctx, `UPDATE apifull_passkey_session SET used = TRUE WHERE challenge = $1 AND kind = $2`, challenge, kind); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *Dao) DeleteCredential(ctx context.Context, userID int64, id []byte) (bool, error) {
	if err := d.available(); err != nil {
		return false, err
	}
	tx, err := d.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `
		UPDATE apifull_passkey_credential SET deleted = TRUE
		WHERE user_id = $1 AND credential_id = $2 AND deleted = FALSE
	`, userID, id)
	if err != nil {
		return false, err
	}
	deleted := result.RowsAffected() == 1
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return deleted, nil
}

func (d *Dao) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}
	d.DB.Close()
	return nil
}
