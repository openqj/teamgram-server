package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/botapi"
)

func TestBotCommandsAndLifecyclePostgres(t *testing.T) {
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

	schema := pgx.Identifier{fmt.Sprintf("botapi_commands_test_%d", time.Now().UnixNano())}.Sanitize()
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
	for _, migration := range []string{"003_biz_user.sql", "081_biz_bot_api.sql", "105_biz_bot_api_commands_lifecycle.sql"} {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../teamgramd/deploy/sql/postgres", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}

	const botID int64 = 920301
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,phone,is_bot) VALUES($1,$2,TRUE)`, botID, "+19999200301"); err != nil {
		t.Fatal(err)
	}
	botStore := NewWithPool(pool)
	commands := []botapi.BotCommand{{Command: "start", Description: "Start"}, {Command: "help", Description: "Help"}}
	if err := botStore.SetBotCommands(ctx, botID, "chat:-100", "zh", commands); err != nil {
		t.Fatal(err)
	}
	got, found, err := botStore.GetBotCommands(ctx, botID, "chat:-100", "zh")
	if err != nil || !found || !reflect.DeepEqual(got, commands) {
		t.Fatalf("commands = (%#v, %v, %v), want persisted variant", got, found, err)
	}
	if err := botStore.DeleteBotCommands(ctx, botID, "chat:-100", "zh"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := botStore.GetBotCommands(ctx, botID, "chat:-100", "zh"); err != nil || found {
		t.Fatalf("commands after delete = (found=%v, err=%v), want absent", found, err)
	}

	state, err := botStore.GetBotLifecycle(ctx, botID)
	if err != nil || state != BotLifecycleActive {
		t.Fatalf("initial lifecycle = (%q, %v), want active", state, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bot_api_updates(bot_id,update_id,update_data) VALUES($1,1,'{}'::jsonb)`, botID); err != nil {
		t.Fatal(err)
	}
	allowed, err := botStore.TransitionBotLifecycle(ctx, botID, BotLifecycleLoggedOut)
	if err != nil || !allowed {
		t.Fatalf("logOut transition = (%v, %v), want allowed", allowed, err)
	}
	if state, err = botStore.GetBotLifecycle(ctx, botID); err != nil || state != BotLifecycleLoggedOut {
		t.Fatalf("logged out lifecycle = (%q, %v)", state, err)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bot_api_updates WHERE bot_id=$1`, botID).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending updates after logOut = (%d, %v), want cleared", pending, err)
	}
	allowed, err = botStore.TransitionBotLifecycle(ctx, botID, BotLifecycleClosed)
	if err != nil || allowed {
		t.Fatalf("close after logOut = (%v, %v), want unauthorized transition", allowed, err)
	}
	if err := botStore.SetBotLifecycle(ctx, botID, BotLifecycleActive); err != nil {
		t.Fatal(err)
	}
	allowed, err = botStore.TransitionBotLifecycle(ctx, botID, BotLifecycleClosed)
	if err != nil || !allowed {
		t.Fatalf("close transition = (%v, %v), want allowed", allowed, err)
	}
	if state, err = botStore.GetBotLifecycle(ctx, botID); err != nil || state != BotLifecycleClosed {
		t.Fatalf("closed lifecycle = (%q, %v)", state, err)
	}
}
