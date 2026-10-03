package dao

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	marmota_cache "github.com/teamgram/marmota/pkg/stores/cache"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dao/mysql_dao"
)

type authKeyOwnerAuditCache struct {
	marmota_cache.BatchCache
}

func (c *authKeyOwnerAuditCache) DelCtx(context.Context, ...string) error { return nil }

func TestBindAuthKeyUserSingleOwnerAuditMySQL(t *testing.T) {
	dsn := os.Getenv("AUTH_CREDENTIALS_AUDIT_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set AUTH_CREDENTIALS_AUDIT_MYSQL_DSN to the isolated teamgram_auth_credentials_audit database")
	}
	dsnConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if dsnConfig.DBName != "teamgram_auth_credentials_audit" || dsnConfig.Net != "tcp" || dsnConfig.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-isolated MySQL DSN: database=%q address=%q", dsnConfig.DBName, dsnConfig.Addr)
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err = sqlDB.Ping(); err != nil {
		t.Fatalf("connect isolated auth credential database: %v", err)
	}
	lockConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lockConn.Close() })
	var lockAcquired sql.NullInt64
	if err = lockConn.QueryRowContext(context.Background(), "SELECT GET_LOCK('auth-credential-single-owner-audit', 10)").Scan(&lockAcquired); err != nil || !lockAcquired.Valid || lockAcquired.Int64 != 1 {
		t.Fatalf("acquire isolated audit lock: acquired=%v error=%v", lockAcquired, err)
	}
	t.Cleanup(func() {
		_, _ = lockConn.ExecContext(context.Background(), "SELECT RELEASE_LOCK('auth-credential-single-owner-audit')")
	})

	for _, statement := range []string{
		"DROP TABLE IF EXISTS auth_users",
		"DROP TABLE IF EXISTS auth_key_infos",
		`CREATE TABLE auth_key_infos (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			auth_key_id BIGINT NOT NULL,
			auth_key_type INT NOT NULL DEFAULT 0,
			perm_auth_key_id BIGINT NOT NULL DEFAULT 0,
			temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
			media_temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			UNIQUE KEY auth_key_id (auth_key_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE auth_users (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			auth_key_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL DEFAULT 0,
			hash BIGINT NOT NULL DEFAULT 0,
			date_created BIGINT NOT NULL DEFAULT 0,
			date_active BIGINT NOT NULL DEFAULT 0,
			state INT NOT NULL DEFAULT 0,
			android_push_session_id BIGINT NOT NULL DEFAULT 0,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			active_auth_key_id BIGINT GENERATED ALWAYS AS (CASE WHEN deleted = 0 THEN auth_key_id ELSE NULL END) STORED,
			UNIQUE KEY auth_key_user (auth_key_id, user_id),
			UNIQUE KEY auth_users_one_active_owner (active_auth_key_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err = sqlDB.Exec(statement); err != nil {
			t.Fatalf("prepare isolated schema: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = sqlDB.Exec("DROP TABLE IF EXISTS auth_users")
		_, _ = sqlDB.Exec("DROP TABLE IF EXISTS auth_key_infos")
	})

	wrapperDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	testDao := &Dao{
		Mysql: &Mysql{
			DB:           wrapperDB,
			AuthUsersDAO: mysql_dao.NewAuthUsersDAO(wrapperDB),
		},
		CachedConn: sqlc.NewConnWithCache(wrapperDB, &authKeyOwnerAuditCache{}),
	}

	base := time.Now().UnixNano()
	firstAuthKeyID := base
	secondAuthKeyID := base + 1
	firstUserID := base + 10
	secondUserID := base + 11
	for _, authKeyID := range []int64{firstAuthKeyID, secondAuthKeyID} {
		if _, err = sqlDB.Exec("INSERT INTO auth_key_infos (auth_key_id, perm_auth_key_id) VALUES (?, ?)", authKeyID, authKeyID); err != nil {
			t.Fatal(err)
		}
	}

	firstHash, err := testDao.BindAuthKeyUser(context.Background(), firstAuthKeyID, firstUserID)
	if err != nil || firstHash == 0 {
		t.Fatalf("first bind = (%d, %v), want nonzero hash, nil", firstHash, err)
	}
	retryHash, err := testDao.BindAuthKeyUser(context.Background(), firstAuthKeyID, firstUserID)
	if err != nil || retryHash != firstHash {
		t.Fatalf("idempotent bind = (%d, %v), want (%d, nil)", retryHash, err, firstHash)
	}
	if _, err = testDao.BindAuthKeyUser(context.Background(), firstAuthKeyID, secondUserID); !errors.Is(err, ErrAuthKeyOwnedByAnotherUser) {
		t.Fatalf("different-owner bind error = %v, want %v", err, ErrAuthKeyOwnedByAnotherUser)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, userID := range []int64{firstUserID, secondUserID} {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			<-start
			_, bindErr := testDao.BindAuthKeyUser(context.Background(), secondAuthKeyID, userID)
			errs <- bindErr
		}(userID)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for bindErr := range errs {
		switch {
		case bindErr == nil:
			successes++
		case errors.Is(bindErr, ErrAuthKeyOwnedByAnotherUser):
			conflicts++
		default:
			t.Fatalf("concurrent bind error = %v", bindErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent bind outcomes = %d successes/%d conflicts, want 1/1", successes, conflicts)
	}

	var activeOwners int
	if err = sqlDB.QueryRow("SELECT COUNT(*) FROM auth_users WHERE auth_key_id = ? AND deleted = 0", secondAuthKeyID).Scan(&activeOwners); err != nil {
		t.Fatal(err)
	}
	if activeOwners != 1 {
		t.Fatalf("active owners = %d, want 1", activeOwners)
	}
}
