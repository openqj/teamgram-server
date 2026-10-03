package mysql_dao

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

func TestUnregisteredContactsImporterCountsExcludeDeletedOwnerRows(t *testing.T) {
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
	var databaseName string
	if err = adminDB.QueryRow("SELECT DATABASE()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "teamgram_audit" {
		t.Fatalf("connected database = %q, want teamgram_audit", databaseName)
	}

	var exists int
	if err = adminDB.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = 'unregistered_contacts'`, databaseName).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	createdTable := exists == 0
	if createdTable {
		_, err = adminDB.Exec(`CREATE TABLE unregistered_contacts (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			phone VARCHAR(32) NOT NULL,
			importer_user_id BIGINT NOT NULL,
			import_first_name VARCHAR(64) NOT NULL,
			import_last_name VARCHAR(64) NOT NULL,
			imported TINYINT(1) NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY phone_importer (phone, importer_user_id),
			KEY phone_imported (phone, importer_user_id, imported)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
		if err != nil {
			t.Fatalf("create isolated audit fixture table: %v", err)
		}
		t.Cleanup(func() { _, _ = adminDB.Exec("DROP TABLE IF EXISTS unregistered_contacts") })
	}

	stamp := time.Now().UnixNano()
	phone1 := fmt.Sprintf("+999%d", stamp)
	phone2 := phone1 + "2"
	_, err = adminDB.Exec("DELETE FROM unregistered_contacts WHERE phone IN (?, ?)", phone1, phone2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminDB.Exec("DELETE FROM unregistered_contacts WHERE phone IN (?, ?)", phone1, phone2) })

	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open user DAO database: %v", err)
	}
	dao := NewUnregisteredContactsDAO(wrappedDB)
	ctx := context.Background()
	insert := func(phone string, importerID int64) {
		t.Helper()
		if _, _, err := dao.InsertOrUpdate(ctx, &dataobject.UnregisteredContactsDO{
			Phone:          phone,
			ImporterUserId: importerID,
		}); err != nil {
			t.Fatalf("upsert unregistered contact: %v", err)
		}
	}
	insert(phone1, stamp+1)
	insert(phone1, stamp+1)
	insert(phone1, stamp+2)
	insert(phone2, stamp+3)
	if _, err = adminDB.Exec("UPDATE unregistered_contacts SET imported = 1 WHERE phone = ?", phone2); err != nil {
		t.Fatal(err)
	}

	counts, err := dao.SelectDistinctImporterCountsByPhoneList(ctx, []string{phone2, phone1})
	if err != nil {
		t.Fatal(err)
	}
	if counts[phone1] != 2 {
		t.Fatalf("distinct importers for %q = %d, want 2", phone1, counts[phone1])
	}
	if _, ok := counts[phone2]; ok {
		t.Fatalf("imported phone %q unexpectedly counted: %#v", phone2, counts)
	}
	empty, err := dao.SelectDistinctImporterCountsByPhoneList(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty phone list = (%v, %v), want empty success", empty, err)
	}

	if _, err = dao.DeleteImporterByUserAndPhone(ctx, phone1, stamp+1); err != nil {
		t.Fatalf("delete one importer's phone row: %v", err)
	}
	remaining, err := dao.SelectDistinctImporterCountsByPhoneList(ctx, []string{phone1})
	if err != nil {
		t.Fatalf("count phone importers after scoped deletion: %v", err)
	}
	if remaining[phone1] != 1 {
		t.Fatalf("active importers for %q after deleting one owner = %d, want 1", phone1, remaining[phone1])
	}

}
