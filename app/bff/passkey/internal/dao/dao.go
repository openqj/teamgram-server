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
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
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
	DB                *sql.DB
	DBErr             error
	UserClient        user_client.UserClient
	AuthsessionClient authsession_client.AuthsessionClient
}

func New(c config.Config) *Dao {
	d := &Dao{
		UserClient:        user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		AuthsessionClient: authsession_client.NewAuthsessionClient(rpcx.GetCachedRpcClient(c.AuthSessionClient)),
	}
	if c.MysqlDSN == "" {
		d.DBErr = ErrUnavailable
		return d
	}
	d.DB, d.DBErr = sql.Open("mysql", c.MysqlDSN)
	if d.DBErr != nil {
		return d
	}
	d.DB.SetMaxOpenConns(8)
	d.DB.SetMaxIdleConns(8)
	if d.DBErr = d.DB.Ping(); d.DBErr != nil {
		_ = d.DB.Close()
		return d
	}
	if d.DBErr = migrate(d.DB); d.DBErr != nil {
		_ = d.DB.Close()
	}
	return d
}

func (d *Dao) available() error {
	if d == nil || d.DB == nil {
		return ErrUnavailable
	}
	return d.DBErr
}

func migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS apifull_passkey_session (
			challenge VARCHAR(255) NOT NULL PRIMARY KEY,
			user_id BIGINT NOT NULL DEFAULT 0,
			kind VARCHAR(16) NOT NULL,
			data MEDIUMBLOB NOT NULL,
			expires_at BIGINT NOT NULL,
			used TINYINT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			KEY idx_apifull_passkey_session_expiry (expires_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_passkey_credential (
			credential_id VARBINARY(255) NOT NULL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			name VARCHAR(255) NOT NULL DEFAULT '',
			date_created BIGINT NOT NULL,
			last_usage_date BIGINT NOT NULL DEFAULT 0,
			sign_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
			credential MEDIUMBLOB NOT NULL,
			deleted TINYINT NOT NULL DEFAULT 0,
			KEY idx_apifull_passkey_credential_user (user_id, deleted)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dao) PutSession(ctx context.Context, session *Session) error {
	if err := d.available(); err != nil {
		return err
	}
	if session == nil || session.Challenge == "" || session.Kind == "" || len(session.Data) == 0 || session.ExpiresAt <= 0 {
		return fmt.Errorf("invalid passkey session")
	}
	_, err := d.DB.ExecContext(ctx, `
		INSERT INTO apifull_passkey_session (challenge, user_id, kind, data, expires_at, used, created_at)
		VALUES (?, ?, ?, ?, ?, 0, ?)
	`, session.Challenge, session.UserID, session.Kind, session.Data, session.ExpiresAt, time.Now().Unix())
	return err
}

func (d *Dao) GetSession(ctx context.Context, challenge, kind string) (*Session, error) {
	if err := d.available(); err != nil {
		return nil, err
	}
	if challenge == "" || kind == "" {
		return nil, ErrSessionNotFound
	}
	var session Session
	var used int
	err := d.DB.QueryRowContext(ctx, `
		SELECT challenge, user_id, kind, data, expires_at, used
		FROM apifull_passkey_session WHERE challenge = ? AND kind = ?
	`, challenge, kind).Scan(&session.Challenge, &session.UserID, &session.Kind, &session.Data, &session.ExpiresAt, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	if used != 0 || session.ExpiresAt < time.Now().Unix() {
		return nil, ErrSessionUsed
	}
	return &session, nil
}

func (d *Dao) ConsumeSession(ctx context.Context, challenge, kind string) error {
	if err := d.available(); err != nil {
		return err
	}
	result, err := d.DB.ExecContext(ctx, `
		UPDATE apifull_passkey_session SET used = 1
		WHERE challenge = ? AND kind = ? AND used = 0 AND expires_at >= ?
	`, challenge, kind, time.Now().Unix())
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrSessionUsed
	}
	return nil
}

func (d *Dao) ListCredentials(ctx context.Context, userID int64) ([]Credential, error) {
	if err := d.available(); err != nil {
		return nil, err
	}
	rows, err := d.DB.QueryContext(ctx, `
		SELECT credential_id, user_id, name, date_created, last_usage_date, sign_count, credential
		FROM apifull_passkey_credential WHERE user_id = ? AND deleted = 0 ORDER BY date_created, credential_id
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
	err := d.DB.QueryRowContext(ctx, `
		SELECT credential_id, user_id, name, date_created, last_usage_date, sign_count, credential
		FROM apifull_passkey_credential WHERE credential_id = ? AND deleted = 0
	`, id).Scan(&credential.ID, &credential.UserID, &credential.Name, &credential.Date, &credential.LastUsageDate, &count, &credential.Data)
	if errors.Is(err, sql.ErrNoRows) {
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
	_, err := d.DB.ExecContext(ctx, `
		INSERT INTO apifull_passkey_credential
		(credential_id, user_id, name, date_created, last_usage_date, sign_count, credential, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
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
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var expiresAt int64
	var used int
	var userID int64
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, expires_at, used FROM apifull_passkey_session
		WHERE challenge = ? AND kind = ? FOR UPDATE
	`, challenge, kind).Scan(&userID, &expiresAt, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return err
	}
	if used != 0 || expiresAt < time.Now().Unix() || userID != credential.UserID {
		return ErrSessionUsed
	}
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_passkey_session SET used = 1 WHERE challenge = ? AND kind = ?`, challenge, kind); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO apifull_passkey_credential
		(credential_id, user_id, name, date_created, last_usage_date, sign_count, credential, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
	`, credential.ID, credential.UserID, credential.Name, credential.Date, credential.LastUsageDate, credential.SignCount, credential.Data); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *Dao) UpdateCredential(ctx context.Context, id []byte, expectedCount uint32, nextCount uint32, data []byte, usageDate int64) error {
	if err := d.available(); err != nil {
		return err
	}
	result, err := d.DB.ExecContext(ctx, `
		UPDATE apifull_passkey_credential
		SET sign_count = ?, credential = ?, last_usage_date = ?
		WHERE credential_id = ? AND deleted = 0 AND sign_count = ?
	`, nextCount, data, usageDate, id, expectedCount)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
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
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var used int
	var expiresAt int64
	if err = tx.QueryRowContext(ctx, `
		SELECT expires_at, used FROM apifull_passkey_session
		WHERE challenge = ? AND kind = ? FOR UPDATE
	`, challenge, kind).Scan(&expiresAt, &used); errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	} else if err != nil {
		return err
	}
	if used != 0 || expiresAt < time.Now().Unix() {
		return ErrSessionUsed
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE apifull_passkey_credential
		SET sign_count = ?, credential = ?, last_usage_date = ?
		WHERE credential_id = ? AND deleted = 0 AND sign_count = ?
	`, nextCount, data, usageDate, id, expectedCount)
	if err != nil {
		return err
	}
	if n, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if n != 1 {
		return ErrCounterReplay
	}
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_passkey_session SET used = 1 WHERE challenge = ? AND kind = ?`, challenge, kind); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *Dao) DeleteCredential(ctx context.Context, userID int64, id []byte) (bool, error) {
	if err := d.available(); err != nil {
		return false, err
	}
	result, err := d.DB.ExecContext(ctx, `
		UPDATE apifull_passkey_credential SET deleted = 1
		WHERE user_id = ? AND credential_id = ? AND deleted = 0
	`, userID, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (d *Dao) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}
	return d.DB.Close()
}
