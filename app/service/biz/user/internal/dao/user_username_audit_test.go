package dao

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
	marmota_cache "github.com/teamgram/marmota/pkg/stores/cache"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
)

type usernameAuditCache struct {
	marmota_cache.BatchCache
	deleted []string
}

func (c *usernameAuditCache) DelCtx(_ context.Context, keys ...string) error {
	c.deleted = append(c.deleted, keys...)
	return nil
}

func TestUpdateUserUsernameAuditDatabase(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_USERNAME_AUDIT_DSN")
	if dsn == "" {
		t.Skip("set TEAMGRAM_USERNAME_AUDIT_DSN to the isolated teamgram_username_audit database")
	}

	dsnConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if dsnConfig.DBName != "teamgram_username_audit" || dsnConfig.Net != "tcp" || dsnConfig.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-isolated MySQL DSN: database=%q address=%q", dsnConfig.DBName, dsnConfig.Addr)
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err = sqlDB.Ping(); err != nil {
		t.Fatalf("connect isolated audit database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = sqlDB.Exec("DROP TABLE IF EXISTS username")
		_, _ = sqlDB.Exec("DROP TABLE IF EXISTS users")
	})

	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open user DAO database: %v", err)
	}
	cache := &usernameAuditCache{}
	testDao := &Dao{
		Mysql:      newMysqlDao(wrappedDB),
		CachedConn: sqlc.NewConnWithCache(wrappedDB, cache),
	}

	reset := func() {
		t.Helper()
		for _, statement := range []string{
			"DROP TABLE IF EXISTS username",
			"DROP TABLE IF EXISTS users",
			`CREATE TABLE users (
				id BIGINT NOT NULL PRIMARY KEY,
				username VARCHAR(64) NOT NULL DEFAULT ''
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE username (
				id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
				username VARCHAR(32) NOT NULL,
				peer_type INT NOT NULL DEFAULT 0,
				peer_id BIGINT NOT NULL DEFAULT 0,
				editable BOOLEAN NOT NULL DEFAULT TRUE,
				active BOOLEAN NOT NULL DEFAULT TRUE,
				order2 BIGINT NOT NULL DEFAULT 0,
				deleted BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY username (username)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		} {
			if _, err := sqlDB.Exec(statement); err != nil {
				t.Fatalf("reset isolated fixture: %v", err)
			}
		}
	}
	seed := func(id int64, username string) {
		t.Helper()
		if _, err := sqlDB.Exec("INSERT INTO users (id, username) VALUES (?, ?)", id, username); err != nil {
			t.Fatal(err)
		}
		if username == "" {
			return
		}
		if _, err := sqlDB.Exec("INSERT INTO username (username, peer_type, peer_id, editable, active) VALUES (?, ?, ?, TRUE, TRUE)", username, mtproto.PEER_USER, id); err != nil {
			t.Fatal(err)
		}
	}
	readProfile := func(id int64) string {
		t.Helper()
		var username string
		if err := sqlDB.QueryRow("SELECT username FROM users WHERE id = ?", id).Scan(&username); err != nil {
			t.Fatal(err)
		}
		return username
	}
	indexOwner := func(username string) (int32, int64, error) {
		t.Helper()
		var peerType int32
		var peerID int64
		err := sqlDB.QueryRow("SELECT peer_type, peer_id FROM username WHERE username = ?", username).Scan(&peerType, &peerID)
		return peerType, peerID, err
	}

	t.Run("valid update and index readback", func(t *testing.T) {
		reset()
		cache.deleted = nil
		seed(42, "oldname")

		if err := testDao.UpdateUserUsername(context.Background(), 42, "newname"); err != nil {
			t.Fatalf("update username: %v", err)
		}
		if got := readProfile(42); got != "newname" {
			t.Fatalf("profile readback = %q, want newname", got)
		}
		peerType, peerID, err := indexOwner("newname")
		if err != nil || peerType != mtproto.PEER_USER || peerID != 42 {
			t.Fatalf("index readback = (%d,%d,%v), want (%d,42,nil)", peerType, peerID, err, mtproto.PEER_USER)
		}
		if _, _, err = indexOwner("oldname"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("old index readback error = %v, want sql.ErrNoRows", err)
		}
		if !containsUsernameAuditKey(cache.deleted, "user_data.2#42") || !containsUsernameAuditKey(cache.deleted, "username_42") {
			t.Fatalf("cache invalidation keys = %v, want profile and username keys", cache.deleted)
		}
	})

	t.Run("duplicate username is rejected without changes", func(t *testing.T) {
		reset()
		seed(42, "oldname")
		seed(43, "takenname")

		err := testDao.UpdateUserUsername(context.Background(), 42, "takenname")
		if err != mtproto.ErrUsernameOccupied {
			t.Fatalf("duplicate update error = %v, want %v", err, mtproto.ErrUsernameOccupied)
		}
		if got := readProfile(42); got != "oldname" {
			t.Fatalf("profile after duplicate = %q, want oldname", got)
		}
		peerType, peerID, err := indexOwner("oldname")
		if err != nil || peerType != mtproto.PEER_USER || peerID != 42 {
			t.Fatalf("old index after duplicate = (%d,%d,%v), want (%d,42,nil)", peerType, peerID, err, mtproto.PEER_USER)
		}
		peerType, peerID, err = indexOwner("takenname")
		if err != nil || peerType != mtproto.PEER_USER || peerID != 43 {
			t.Fatalf("occupied index after duplicate = (%d,%d,%v), want (%d,43,nil)", peerType, peerID, err, mtproto.PEER_USER)
		}
	})

	t.Run("profile write failure rolls back index writes", func(t *testing.T) {
		reset()
		seed(42, "oldname")
		if _, err := sqlDB.Exec(`ALTER TABLE users ADD CONSTRAINT username_audit_reject_newname CHECK (username <> 'newname')`); err != nil {
			t.Fatalf("create isolated profile write constraint: %v", err)
		}

		if err := testDao.UpdateUserUsername(context.Background(), 42, "newname"); err == nil {
			t.Fatal("profile write failure was reported as success")
		}
		if got := readProfile(42); got != "oldname" {
			t.Fatalf("profile after failed transaction = %q, want oldname", got)
		}
		peerType, peerID, err := indexOwner("oldname")
		if err != nil || peerType != mtproto.PEER_USER || peerID != 42 {
			t.Fatalf("old index after failed transaction = (%d,%d,%v), want (%d,42,nil)", peerType, peerID, err, mtproto.PEER_USER)
		}
		if _, _, err = indexOwner("newname"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("new index after failed transaction error = %v, want sql.ErrNoRows", err)
		}
	})
}

func containsUsernameAuditKey(keys []string, wanted string) bool {
	for _, key := range keys {
		if key == wanted {
			return true
		}
	}
	return false
}
