package mysql_dao

import (
	"context"
	"database/sql"
	"os"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

func TestUserPeerBlocksSelectListHonorsOffset(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("legacy MySQL audit fixture is not configured")
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBName != "teamgram_audit" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", cfg.DBName, cfg.Addr)
	}

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer adminDB.Close()
	if err = adminDB.Ping(); err != nil {
		t.Fatalf("connect isolated audit database: %v", err)
	}

	var exists int
	if err = adminDB.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = 'user_peer_blocks'`, cfg.DBName).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	createdTable := exists == 0
	if createdTable {
		_, err = adminDB.Exec(`CREATE TABLE user_peer_blocks (
			id BIGINT NOT NULL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			peer_type INT NOT NULL,
			peer_id BIGINT NOT NULL,
			date BIGINT NOT NULL DEFAULT 0,
			deleted TINYINT(1) NOT NULL DEFAULT 0
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
		if err != nil {
			t.Fatalf("create isolated audit fixture table: %v", err)
		}
		t.Cleanup(func() { _, _ = adminDB.Exec("DROP TABLE IF EXISTS user_peer_blocks") })
	}

	const userID int64 = 991229813
	_, err = adminDB.Exec("DELETE FROM user_peer_blocks WHERE user_id = ?", userID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminDB.Exec("DELETE FROM user_peer_blocks WHERE user_id = ?", userID) })
	for i := int64(1); i <= 4; i++ {
		_, err = adminDB.Exec("INSERT INTO user_peer_blocks (id, user_id, peer_type, peer_id, date) VALUES (?, ?, 1, ?, ?)", userID*10+i, userID, i, i*100)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = adminDB.Exec("INSERT INTO user_peer_blocks (id, user_id, peer_type, peer_id, date, deleted) VALUES (?, ?, 1, 5, 500, 1)", userID*10+5, userID)
	if err != nil {
		t.Fatal(err)
	}

	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open user DAO database: %v", err)
	}
	dao := NewUserPeerBlocksDAO(wrappedDB)
	got, err := dao.SelectList(context.Background(), userID, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PeerId != 2 || got[1].PeerId != 3 {
		t.Fatalf("offset page = %#v, want peer IDs [2 3]", got)
	}

	count, err := dao.SelectCount(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("blocked count = %d, want 4", count)
	}
}
