package mysql_dao

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

func TestUserNotifySettingsPersistsAndReadsBack(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set APIFULL_MYSQL_DSN to the isolated 127.0.0.1:13306/teamgram_audit database")
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse APIFULL_MYSQL_DSN: %v", err)
	}
	if cfg.DBName != "teamgram_audit" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", cfg.DBName, cfg.Addr)
	}

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })
	if err = adminDB.Ping(); err != nil {
		t.Fatalf("connect isolated audit database: %v", err)
	}

	var exists int
	if err = adminDB.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = 'user_notify_settings'`, cfg.DBName).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists == 0 {
		_, err = adminDB.Exec(`CREATE TABLE user_notify_settings (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
		if err != nil {
			t.Fatalf("create isolated audit fixture table: %v", err)
		}
		t.Cleanup(func() { _, _ = adminDB.Exec("DROP TABLE IF EXISTS user_notify_settings") })
	}

	stamp := time.Now().UnixNano()
	userID := stamp
	peerID := stamp + 1
	const peerType = int32(2)
	if _, err = adminDB.Exec("DELETE FROM user_notify_settings WHERE user_id = ?", userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminDB.Exec("DELETE FROM user_notify_settings WHERE user_id = ?", userID) })

	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open user DAO database: %v", err)
	}
	dao := NewUserNotifySettingsDAO(wrappedDB)
	ctx := context.Background()

	write := func(showPreviews, silent, muteUntil int32, sound string) {
		t.Helper()
		_, _, err := dao.InsertOrUpdateExt(ctx, userID, peerType, peerID, map[string]interface{}{
			"show_previews": showPreviews,
			"silent":        silent,
			"mute_until":    muteUntil,
			"sound":         sound,
		})
		if err != nil {
			t.Fatalf("upsert notify settings: %v", err)
		}
	}
	assertSettings := func(showPreviews, silent, muteUntil int32, sound string) {
		t.Helper()
		got, err := dao.Select(ctx, userID, peerType, peerID)
		if err != nil {
			t.Fatalf("select notify settings: %v", err)
		}
		if got == nil {
			t.Fatal("select notify settings returned nil")
		}
		if got.ShowPreviews != showPreviews || got.Silent != silent || got.MuteUntil != muteUntil || got.Sound != sound {
			t.Fatalf("notify settings = %#v, want showPreviews=%d silent=%d muteUntil=%d sound=%q", got, showPreviews, silent, muteUntil, sound)
		}
	}

	write(0, 1, 12345, "chime")
	assertSettings(0, 1, 12345, "chime")
	write(1, 0, 0, "default")
	assertSettings(1, 0, 0, "default")

	if _, err = dao.DeleteAll(ctx, userID); err != nil {
		t.Fatalf("delete notify settings: %v", err)
	}
	got, err := dao.Select(ctx, userID, peerType, peerID)
	if err != nil {
		t.Fatalf("select deleted notify settings: %v", err)
	}
	if got != nil {
		t.Fatalf("select deleted notify settings = %#v, want nil", got)
	}
}
