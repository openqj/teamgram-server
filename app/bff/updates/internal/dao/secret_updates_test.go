package dao

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func isolatedUpdatesDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("UPDATES_MYSQL_DSN")
	if dsn == "" {
		t.Skip("UPDATES_MYSQL_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse UPDATES_MYSQL_DSN: %v", err)
	}
	if cfg.DBName != "teamgram_audit" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13306" {
		t.Fatalf("test requires 127.0.0.1:13306/teamgram_audit")
	}
	return dsn
}

func TestSecretUpdatesReaderRequiresMySQL(t *testing.T) {
	reader, err := NewSecretUpdatesReader("")
	if reader != nil || !errors.Is(err, ErrSecretUpdatesDisabled) {
		t.Fatalf("reader = %v, err = %v", reader, err)
	}
}

func TestMySQLSecretUpdatesReader(t *testing.T) {
	dsn := isolatedUpdatesDSN(t)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}

	userID := time.Now().UnixNano()
	if _, err = db.Exec(`INSERT INTO apifull_secret_user_state (user_id, last_qts, confirmed_qts) VALUES (?,3,0)`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_secret_message WHERE recipient_user_id=?`, userID)
		_, _ = db.Exec(`DELETE FROM apifull_secret_user_state WHERE user_id=?`, userID)
		_ = db.Close()
	})
	rows := []struct {
		qts     int32
		service int
		file    bool
	}{{3, 0, true}, {1, 0, false}, {2, 1, false}}
	for _, row := range rows {
		var fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint any
		if row.file {
			fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint = int64(90), int64(91), int64(92), int32(4), int32(93)
		}
		_, err = db.Exec(`INSERT INTO apifull_secret_message
			(chat_id, sender_user_id, recipient_user_id, random_id, qts, date, encrypted_data, service,
			 file_id, file_access_hash, file_size, file_dc_id, file_key_fingerprint, acknowledged_at, read_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0,0)`,
			20, userID+1, userID, userID+int64(row.qts), row.qts, 100+row.qts, []byte{byte(row.qts)}, row.service,
			fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint)
		if err != nil {
			t.Fatal(err)
		}
	}

	reader, err := NewSecretUpdatesReader(dsn)
	if err != nil {
		t.Fatal(err)
	}
	mysqlReader := reader.(*mysqlSecretUpdatesReader)
	defer mysqlReader.db.Close()

	current, err := reader.CurrentQTS(context.Background(), userID)
	if err != nil || current != 3 {
		t.Fatalf("current qts = (%d, %v)", current, err)
	}
	first, err := reader.GetDifference(context.Background(), userID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.CurrentQTS != 3 || !first.HasMore || len(first.Messages) != 2 || first.Messages[0].QTS != 1 || first.Messages[1].QTS != 2 || !first.Messages[1].Service {
		t.Fatalf("first page = %+v", first)
	}
	last, err := reader.GetDifference(context.Background(), userID, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if last.HasMore || len(last.Messages) != 1 || last.Messages[0].QTS != 3 || last.Messages[0].File == nil || last.Messages[0].File.ID != 90 {
		t.Fatalf("last page = %+v", last)
	}
	if _, err = reader.GetDifference(context.Background(), userID, 4, 2); !errors.Is(err, ErrMaxQTSInvalid) {
		t.Fatalf("future qts error = %v", err)
	}
}
