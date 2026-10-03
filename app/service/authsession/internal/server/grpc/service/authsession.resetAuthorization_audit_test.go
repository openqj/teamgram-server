package service

import (
	"context"
	"database/sql"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	marmota_cache "github.com/teamgram/marmota/pkg/stores/cache"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/svc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type authsessionAuditCache struct {
	marmota_cache.BatchCache
	mu      sync.Mutex
	deleted []string
}

func (c *authsessionAuditCache) TakeCtx(_ context.Context, value any, _ string, query func(any) error) error {
	return query(value)
}

func (c *authsessionAuditCache) DelCtx(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, keys...)
	return nil
}

func (c *authsessionAuditCache) resetDeleted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = nil
}

func (c *authsessionAuditCache) deletedKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.deleted...)
}

func TestAuthsessionResetAuthorizationAuditMySQL(t *testing.T) {
	dsn := os.Getenv("AUTHSESSION_AUDIT_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set AUTHSESSION_AUDIT_MYSQL_DSN to the isolated teamgram_authsession_audit database")
	}

	dsnConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if dsnConfig.DBName != "teamgram_authsession_audit" || dsnConfig.Net != "tcp" || dsnConfig.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-isolated MySQL DSN: database=%q address=%q", dsnConfig.DBName, dsnConfig.Addr)
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err = sqlDB.Ping(); err != nil {
		t.Fatalf("connect isolated authsession audit database: %v", err)
	}

	lockConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lockConn.Close() })
	var lockAcquired sql.NullInt64
	if err = lockConn.QueryRowContext(context.Background(), "SELECT GET_LOCK('layer229-authsession-reset-audit', 10)").Scan(&lockAcquired); err != nil || !lockAcquired.Valid || lockAcquired.Int64 != 1 {
		t.Fatalf("acquire isolated authsession audit lock: acquired=%v error=%v", lockAcquired, err)
	}
	t.Cleanup(func() {
		_, _ = lockConn.ExecContext(context.Background(), "SELECT RELEASE_LOCK('layer229-authsession-reset-audit')")
	})

	createdTables := make(map[string]bool)
	ensureTable := func(name, ddl string) {
		t.Helper()
		var count int
		if err := sqlDB.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", name).Scan(&count); err != nil {
			t.Fatalf("inspect isolated authsession audit schema: %v", err)
		}
		if _, err := sqlDB.Exec(ddl); err != nil {
			t.Fatalf("create isolated %s fixture table: %v", name, err)
		}
		createdTables[name] = count == 0
	}
	ensureTable("auth_users", `CREATE TABLE IF NOT EXISTS auth_users (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		auth_key_id BIGINT NOT NULL,
		user_id BIGINT NOT NULL DEFAULT 0,
		hash BIGINT NOT NULL DEFAULT 0,
		date_created BIGINT NOT NULL DEFAULT 0,
		date_active BIGINT NOT NULL DEFAULT 0,
		state INT NOT NULL DEFAULT 0,
		android_push_session_id BIGINT NOT NULL DEFAULT 0,
		deleted BOOLEAN NOT NULL DEFAULT FALSE,
		UNIQUE KEY auth_key_user (auth_key_id, user_id),
		KEY user_deleted (user_id, deleted)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	ensureTable("auth_key_infos", `CREATE TABLE IF NOT EXISTS auth_key_infos (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		auth_key_id BIGINT NOT NULL,
		auth_key_type INT NOT NULL DEFAULT 0,
		perm_auth_key_id BIGINT NOT NULL DEFAULT 0,
		temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
		media_temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
		deleted BOOLEAN NOT NULL DEFAULT FALSE,
		UNIQUE KEY auth_key_id (auth_key_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	ensureTable("auth_keys", `CREATE TABLE IF NOT EXISTS auth_keys (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		auth_key_id BIGINT NOT NULL,
		body TEXT NOT NULL,
		deleted BOOLEAN NOT NULL DEFAULT FALSE,
		UNIQUE KEY auth_key_id (auth_key_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	t.Cleanup(func() {
		for _, table := range []string{"auth_keys", "auth_key_infos", "auth_users"} {
			if createdTables[table] {
				_, _ = sqlDB.Exec("DROP TABLE IF EXISTS " + table)
			}
		}
	})

	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open isolated authsession DAO database: %v", err)
	}
	cache := &authsessionAuditCache{}
	testDao := &dao.Dao{
		Mysql: &dao.Mysql{
			AuthUsersDAO:    mysql_dao.NewAuthUsersDAO(wrappedDB),
			AuthKeysDAO:     mysql_dao.NewAuthKeysDAO(wrappedDB),
			AuthKeyInfosDAO: mysql_dao.NewAuthKeyInfosDAO(wrappedDB),
		},
		CachedConn: sqlc.NewConnWithCache(wrappedDB, cache),
	}

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	authsession.RegisterRPCAuthsessionServer(grpcServer, New(&svc.ServiceContext{Dao: testDao}))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///authsession-audit",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := authsession.NewRPCAuthsessionClient(conn)

	ctx := context.Background()
	base := time.Now().UnixNano()
	userID := base
	otherUserID := base + 1
	currentKeyID := base + 10
	firstRevokedKeyID := base + 11
	secondRevokedKeyID := base + 12
	otherUserKeyID := base + 13
	hashTargetKeyID := base + 14
	hashSiblingKeyID := base + 15
	firstHash := base + 101
	secondHash := base + 102
	insertKey := func(keyID, tempKeyID int64) {
		t.Helper()
		if _, err := sqlDB.Exec("INSERT INTO auth_key_infos (auth_key_id, auth_key_type, perm_auth_key_id, temp_auth_key_id) VALUES (?, 0, ?, ?)", keyID, keyID, tempKeyID); err != nil {
			t.Fatalf("seed isolated auth key metadata: %v", err)
		}
		if _, err := sqlDB.Exec("INSERT INTO auth_keys (auth_key_id, body) VALUES (?, '')", keyID); err != nil {
			t.Fatalf("seed isolated auth key: %v", err)
		}
	}
	insertSession := func(keyID, ownerID, hash int64) {
		t.Helper()
		if _, err := sqlDB.Exec("INSERT INTO auth_users (auth_key_id, user_id, hash, date_created, date_active) VALUES (?, ?, ?, ?, ?)", keyID, ownerID, hash, base, base); err != nil {
			t.Fatalf("seed isolated authorization session: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = sqlDB.Exec("DELETE FROM auth_users WHERE user_id IN (?, ?)", userID, otherUserID)
		_, _ = sqlDB.Exec("DELETE FROM auth_key_infos WHERE auth_key_id IN (?, ?, ?, ?, ?, ?)", currentKeyID, firstRevokedKeyID, secondRevokedKeyID, otherUserKeyID, hashTargetKeyID, hashSiblingKeyID)
		_, _ = sqlDB.Exec("DELETE FROM auth_keys WHERE auth_key_id IN (?, ?, ?, ?, ?, ?)", currentKeyID, firstRevokedKeyID, secondRevokedKeyID, otherUserKeyID, hashTargetKeyID, hashSiblingKeyID)
	})
	insertKey(currentKeyID, 0)
	insertKey(firstRevokedKeyID, base+1001)
	insertKey(secondRevokedKeyID, 0)
	insertKey(otherUserKeyID, 0)
	insertSession(currentKeyID, userID, base+100)
	insertSession(firstRevokedKeyID, userID, firstHash)
	insertSession(secondRevokedKeyID, userID, secondHash)
	insertSession(otherUserKeyID, otherUserID, base+103)
	readSession := func(keyID, ownerID int64) (deleted bool, dateCreated, dateActive int64) {
		t.Helper()
		if err := sqlDB.QueryRow("SELECT deleted, date_created, date_active FROM auth_users WHERE auth_key_id = ? AND user_id = ?", keyID, ownerID).Scan(&deleted, &dateCreated, &dateActive); err != nil {
			t.Fatalf("read isolated authorization state: %v", err)
		}
		return
	}
	assertActive := func(keyID, ownerID int64, wantActive bool) {
		t.Helper()
		deleted, _, _ := readSession(keyID, ownerID)
		if active := !deleted; active != wantActive {
			t.Fatalf("authorization key %d active=%t, want %t", keyID, active, wantActive)
		}
	}

	reply, err := client.AuthsessionResetAuthorization(ctx, &authsession.TLAuthsessionResetAuthorization{
		UserId:    userID,
		AuthKeyId: currentKeyID,
	})
	if err != nil || reply == nil {
		t.Fatalf("reset all other authorizations = (%v, %v), want a vector", reply, err)
	}
	gotIDs := append([]int64(nil), reply.Datas...)
	sort.Slice(gotIDs, func(i, j int) bool { return gotIDs[i] < gotIDs[j] })
	wantIDs := []int64{firstRevokedKeyID, secondRevokedKeyID}
	if len(gotIDs) != len(wantIDs) || gotIDs[0] != wantIDs[0] || gotIDs[1] != wantIDs[1] {
		t.Fatalf("revoked authorization IDs = %v, want %v", gotIDs, wantIDs)
	}
	assertActive(currentKeyID, userID, true)
	assertActive(firstRevokedKeyID, userID, false)
	assertActive(secondRevokedKeyID, userID, false)
	assertActive(otherUserKeyID, otherUserID, true)
	for _, keyID := range []int64{firstRevokedKeyID, secondRevokedKeyID} {
		deleted, dateCreated, dateActive := readSession(keyID, userID)
		if !deleted || dateCreated != 0 || dateActive != 0 {
			t.Fatalf("revoked authorization %d state=(deleted:%t created:%d active:%d), want tombstoned and inactive", keyID, deleted, dateCreated, dateActive)
		}
	}
	wantCacheKeys := []string{"auth_data.2#" + itoa(firstRevokedKeyID), "auth_data.2#" + itoa(secondRevokedKeyID)}
	gotCacheKeys := cache.deletedKeys()
	sort.Strings(wantCacheKeys)
	sort.Strings(gotCacheKeys)
	if len(gotCacheKeys) != len(wantCacheKeys) || gotCacheKeys[0] != wantCacheKeys[0] || gotCacheKeys[1] != wantCacheKeys[1] {
		t.Fatalf("authorization cache invalidations = %v, want %v", gotCacheKeys, wantCacheKeys)
	}

	insertKey(hashTargetKeyID, base+1002)
	insertKey(hashSiblingKeyID, 0)
	insertSession(hashTargetKeyID, userID, secondHash)
	insertSession(hashSiblingKeyID, userID, firstHash)
	cache.resetDeleted()
	reply, err = client.AuthsessionResetAuthorization(ctx, &authsession.TLAuthsessionResetAuthorization{
		UserId:    userID,
		AuthKeyId: currentKeyID,
		Hash:      secondHash,
	})
	if err != nil || reply == nil || len(reply.Datas) != 1 || reply.Datas[0] != hashTargetKeyID {
		t.Fatalf("reset authorization by hash = (%v, %v), want [%d]", reply, err, hashTargetKeyID)
	}
	assertActive(currentKeyID, userID, true)
	assertActive(firstRevokedKeyID, userID, false)
	assertActive(secondRevokedKeyID, userID, false)
	assertActive(hashTargetKeyID, userID, false)
	assertActive(hashSiblingKeyID, userID, true)
	assertActive(otherUserKeyID, otherUserID, true)
	if got := cache.deletedKeys(); len(got) != 1 || got[0] != "auth_data.2#"+itoa(hashTargetKeyID) {
		t.Fatalf("hash reset cache invalidations = %v, want only the matching authorization", got)
	}

	assertActive(currentKeyID, userID, true)
	assertActive(hashSiblingKeyID, userID, true)
	assertActive(otherUserKeyID, otherUserID, true)
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
