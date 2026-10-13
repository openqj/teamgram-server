package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
)

func TestAppendOncePaymentQueryDedupPostgres(t *testing.T) {
	dsn := os.Getenv("BOTAPI_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("TEAMGRAM_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("BOTAPI_POSTGRES_DSN or TEAMGRAM_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
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

	schema := pgx.Identifier{fmt.Sprintf("botapi_test_%d", time.Now().UnixNano())}.Sanitize()
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
	for _, migration := range []string{"003_biz_user.sql", "081_biz_bot_api.sql", "091_biz_bot_api_update_dedup.sql"} {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../teamgramd/deploy/sql/postgres", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}

	const botID int64 = 920001
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,phone,is_bot) VALUES($1,$2,TRUE)`, botID, "+19999200001"); err != nil {
		t.Fatal(err)
	}
	botStore := NewWithPool(pool)
	shipping := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateBotShippingQuery(&mtproto.Update{
		QueryId_INT64:   920101,
		UserId:          920002,
		Payload_BYTES:   []byte("invoice"),
		ShippingAddress: mtproto.MakeTLPostAddress(&mtproto.PostAddress{CountryIso2: "US"}).To_PostAddress(),
	}).To_Update())
	inserted, err := botStore.AppendOnce(ctx, botID, shipping)
	if err != nil || !inserted {
		t.Fatalf("first shipping append inserted=%v err=%v", inserted, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM bot_api_updates WHERE bot_id=$1`, botID); err != nil {
		t.Fatal(err)
	}
	inserted, err = botStore.AppendOnce(ctx, botID, shipping)
	if err != nil || inserted {
		t.Fatalf("consumed shipping retry inserted=%v err=%v", inserted, err)
	}
	var dedupRows, updateRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bot_api_update_dedup WHERE bot_id=$1`, botID).Scan(&dedupRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bot_api_updates WHERE bot_id=$1`, botID).Scan(&updateRows); err != nil {
		t.Fatal(err)
	}
	if dedupRows != 1 || updateRows != 0 {
		t.Fatalf("after consumed retry dedup rows=%d update rows=%d", dedupRows, updateRows)
	}

	precheckout := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateBotPrecheckoutQuery(&mtproto.Update{
		QueryId_INT64:                 920102,
		UserId:                        920002,
		Payload_BYTES:                 []byte("invoice"),
		Currency:                      "USD",
		TotalAmount:                   1200,
		Info_FLAGPAYMENTREQUESTEDINFO: mtproto.MakeTLPaymentRequestedInfo(&mtproto.PaymentRequestedInfo{}).To_PaymentRequestedInfo(),
	}).To_Update())
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := botStore.AppendOnce(ctx, botID, precheckout)
			if err != nil {
				t.Errorf("concurrent precheckout append: %v", err)
				return
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	insertedCount := 0
	for ok := range results {
		if ok {
			insertedCount++
		}
	}
	if insertedCount != 1 {
		t.Fatalf("concurrent precheckout inserted count=%d", insertedCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bot_api_update_dedup WHERE bot_id=$1`, botID).Scan(&dedupRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bot_api_updates WHERE bot_id=$1`, botID).Scan(&updateRows); err != nil {
		t.Fatal(err)
	}
	if dedupRows != 2 || updateRows != 1 {
		t.Fatalf("after concurrent precheckout dedup rows=%d update rows=%d", dedupRows, updateRows)
	}
}
