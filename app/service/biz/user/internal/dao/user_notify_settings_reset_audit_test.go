package dao

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	marmotaCache "github.com/teamgram/marmota/pkg/stores/cache"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
)

type notifySettingsAuditCache struct {
	marmotaCache.BatchCache
	deleted []string
}

func (c *notifySettingsAuditCache) DelCtx(_ context.Context, keys ...string) error {
	c.deleted = append(c.deleted, keys...)
	return nil
}

func TestResetUserNotifySettingsInvalidatesEveryActivePeerCache(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set APIFULL_MYSQL_DSN to the isolated 127.0.0.1:13306/teamgram_audit database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse APIFULL_MYSQL_DSN: %v", err)
	}
	if config.DBName != "teamgram_audit" || config.Net != "tcp" || config.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", config.DBName, config.Addr)
	}

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })
	if err = adminDB.Ping(); err != nil {
		t.Fatalf("connect isolated audit database: %v", err)
	}
	if _, err = adminDB.Exec(`CREATE TABLE IF NOT EXISTS user_notify_settings (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		user_id BIGINT NOT NULL,
		peer_type INT NOT NULL,
		peer_id BIGINT NOT NULL,
		show_previews INT NOT NULL DEFAULT -1,
		silent INT NOT NULL DEFAULT -1,
		mute_until INT NOT NULL DEFAULT -1,
		sound VARCHAR(255) NOT NULL DEFAULT 'default',
		deleted TINYINT(1) NOT NULL DEFAULT 0,
		UNIQUE KEY user_peer (user_id, peer_type, peer_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatalf("create notify settings fixture table: %v", err)
	}

	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open audit database: %v", err)
	}
	userID := time.Now().UnixNano()
	t.Cleanup(func() { _, _ = adminDB.Exec("DELETE FROM user_notify_settings WHERE user_id = ?", userID) })

	cache := &notifySettingsAuditCache{}
	testDAO := &Dao{
		Mysql:      newMysqlDao(wrappedDB),
		CachedConn: sqlc.NewConnWithCache(wrappedDB, cache),
	}
	for _, peer := range []struct {
		peerType int32
		peerID   int64
	}{
		{peerType: mtproto.PEER_USER, peerID: userID + 1},
		{peerType: mtproto.PEER_CHAT, peerID: userID + 2},
	} {
		if err = testDAO.SetUserPeerNotifySettings(context.Background(), userID, peer.peerType, peer.peerID, &mtproto.PeerNotifySettings{}); err != nil {
			t.Fatalf("seed peer %d/%d: %v", peer.peerType, peer.peerID, err)
		}
	}
	cache.deleted = nil

	if err = testDAO.ResetUserNotifySettings(context.Background(), userID); err != nil {
		t.Fatalf("ResetUserNotifySettings: %v", err)
	}

	var active int
	if err = adminDB.QueryRow("SELECT COUNT(*) FROM user_notify_settings WHERE user_id = ? AND deleted = 0", userID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active notify rows = %d, want 0", active)
	}
	for _, key := range []string{
		genUserNotifySettingsCacheKey(userID, mtproto.PEER_USER, userID+1),
		genUserNotifySettingsCacheKey(userID, mtproto.PEER_CHAT, userID+2),
	} {
		found := false
		for _, deleted := range cache.deleted {
			if deleted == key {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("cache invalidations = %v, missing %q", cache.deleted, key)
		}
	}
}
