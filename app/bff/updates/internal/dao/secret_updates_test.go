package dao

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isolatedUpdatesPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("UPDATES_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("UPDATES_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var version int
	if err := admin.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil || version/10000 != 18 {
		t.Fatalf("PostgreSQL 18 required: version=%d err=%v", version, err)
	}
	schema := pgx.Identifier{fmt.Sprintf("secret_updates_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean secret update schema: %v", err)
		}
	})
	for _, table := range []string{"apifull_secret_message", "apifull_secret_user_state"} {
		name := pgx.Identifier{table}.Sanitize()
		if _, err := admin.Exec(ctx, `CREATE TABLE `+schema+`.`+name+` (LIKE public.`+name+` INCLUDING ALL)`); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSecretUpdatesReaderRequiresPostgres(t *testing.T) {
	reader, err := NewSecretUpdatesReader("")
	if reader != nil || !errors.Is(err, ErrSecretUpdatesDisabled) {
		t.Fatalf("reader = %v, err = %v", reader, err)
	}
}

func TestPostgresSecretUpdatesReader(t *testing.T) {
	pool := isolatedUpdatesPool(t)
	ctx := context.Background()
	const userID int64 = 101
	if _, err := pool.Exec(ctx, `INSERT INTO apifull_secret_user_state (user_id, last_qts, confirmed_qts) VALUES ($1,3,0)`, userID); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		qts     int32
		service bool
		file    bool
	}{{3, false, true}, {1, false, false}, {2, true, false}}
	for _, row := range rows {
		var fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint any
		if row.file {
			fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint = int64(90), int64(91), int64(92), int32(4), int32(93)
		}
		_, err := pool.Exec(ctx, `INSERT INTO apifull_secret_message
			(chat_id, sender_user_id, recipient_user_id, random_id, qts, date, encrypted_data, service,
			 file_id, file_access_hash, file_size, file_dc_id, file_key_fingerprint, acknowledged_at, read_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,0,0)`,
			20, userID+1, userID, userID+int64(row.qts), row.qts, 100+row.qts, []byte{byte(row.qts)}, row.service,
			fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint)
		if err != nil {
			t.Fatal(err)
		}
	}
	reader := &postgresSecretUpdatesReader{pool: pool}
	current, err := reader.CurrentQTS(ctx, userID)
	if err != nil || current != 3 {
		t.Fatalf("current qts = (%d, %v)", current, err)
	}
	first, err := reader.GetDifference(ctx, userID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.CurrentQTS != 3 || !first.HasMore || len(first.Messages) != 2 || first.Messages[0].QTS != 1 || first.Messages[1].QTS != 2 || !first.Messages[1].Service {
		t.Fatalf("first page = %+v", first)
	}
	last, err := reader.GetDifference(ctx, userID, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if last.HasMore || len(last.Messages) != 1 || last.Messages[0].QTS != 3 || last.Messages[0].File == nil || last.Messages[0].File.ID != 90 {
		t.Fatalf("last page = %+v", last)
	}
	for _, qts := range []int32{-1, 4} {
		if _, err := reader.GetDifference(ctx, userID, qts, 2); !errors.Is(err, ErrMaxQTSInvalid) {
			t.Fatalf("invalid qts %d error = %v", qts, err)
		}
	}
	if current, err := reader.CurrentQTS(ctx, 202); err != nil || current != 0 {
		t.Fatalf("unused current qts = (%d, %v)", current, err)
	}
}

func TestSecretUpdatesStartupRequiresCompleteSchema(t *testing.T) {
	pool := isolatedUpdatesPool(t)
	databaseURL, err := url.Parse(os.Getenv("UPDATES_POSTGRES_DSN"))
	if err != nil || databaseURL.Host == "" {
		t.Fatalf("test requires PostgreSQL URL: %v", err)
	}
	query := databaseURL.Query()
	query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	databaseURL.RawQuery = query.Encode()
	reader, err := NewSecretUpdatesReader(databaseURL.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.(*postgresSecretUpdatesReader).Close()
	if _, err := pool.Exec(context.Background(), `ALTER TABLE apifull_secret_message DROP COLUMN file_key_fingerprint`); err != nil {
		t.Fatal(err)
	}
	reader, err = NewSecretUpdatesReader(databaseURL.String())
	if err == nil || reader != nil {
		t.Fatalf("secret updates startup accepted an incomplete schema: %v %v", reader, err)
	}
}
