package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func PostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SYNC_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("SYNC_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var version int
	if err := admin.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL 18 required, got %d", version)
	}
	schema := pgx.Identifier{fmt.Sprintf("sync_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean isolated schema: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, source, _, _ := runtime.Caller(0)
	for _, migration := range []string{"001_authsession.sql", "005_biz_updates.sql", "018_idgen_counters.sql", "019_sync_delivery_receipts.sql"} {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../../../teamgramd/deploy/sql/postgres", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	return pool
}

func SeedAuth(t *testing.T, pool *pgxpool.Pool, userID, authID int64, authType int32, deleted bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO auth_keys(auth_key_id,body,deleted) VALUES ($1,$2,$3)`, authID, []byte("fixture"), deleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth_key_infos(auth_key_id,auth_key_type,perm_auth_key_id,deleted) VALUES ($1,$2,$1,$3)`, authID, authType, deleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth_users(auth_key_id,user_id,deleted) VALUES ($1,$2,$3)`, authID, userID, deleted); err != nil {
		t.Fatal(err)
	}
}
