package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/postgres_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func TestMakeBotCommandsDOListValidation(t *testing.T) {
	valid := func() *mtproto.BotCommand {
		return &mtproto.BotCommand{Command: "start_1", Description: "Start the bot"}
	}

	tests := []struct {
		name string
		in   *user.TLUserSetBotCommands
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &user.TLUserSetBotCommands{BotId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing bot", in: &user.TLUserSetBotCommands{UserId: 1}, want: mtproto.ErrInputRequestInvalid},
		{name: "nil command", in: &user.TLUserSetBotCommands{UserId: 1, BotId: 2, Commands: []*mtproto.BotCommand{nil}}, want: mtproto.ErrInputRequestInvalid},
		{name: "invalid command name", in: &user.TLUserSetBotCommands{UserId: 1, BotId: 2, Commands: []*mtproto.BotCommand{{Command: "Start", Description: "Start"}}}, want: mtproto.ErrBotCommandInvalid},
		{name: "invalid description", in: &user.TLUserSetBotCommands{UserId: 1, BotId: 2, Commands: []*mtproto.BotCommand{{Command: "start"}}}, want: mtproto.ErrBotCommandDescriptionInvalid},
		{name: "duplicate command", in: &user.TLUserSetBotCommands{UserId: 1, BotId: 2, Commands: []*mtproto.BotCommand{valid(), valid()}}, want: mtproto.ErrBotCommandInvalid},
		{name: "ephemeral command", in: &user.TLUserSetBotCommands{UserId: 1, BotId: 2, Commands: []*mtproto.BotCommand{{Command: "start", Description: "Start", Ephemeral: true}}}, want: mtproto.ErrBotCommandInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBotCommandsDOList(tt.in)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("makeBotCommandsDOList() = (%v, %v), want (%v, %v)", got, err, nil, tt.want)
			}
		})
	}

	commands := make([]*mtproto.BotCommand, maxBotCommands+1)
	for i := range commands {
		commands[i] = &mtproto.BotCommand{Command: "cmd", Description: "Command"}
	}
	if got, err := makeBotCommandsDOList(&user.TLUserSetBotCommands{UserId: 1, BotId: 2, Commands: commands}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("makeBotCommandsDOList(too many) = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}

	doList, err := makeBotCommandsDOList(&user.TLUserSetBotCommands{UserId: 1, BotId: 2, Commands: []*mtproto.BotCommand{valid()}})
	if err != nil || len(doList) != 1 || doList[0].BotId != 2 || doList[0].Command != "start_1" {
		t.Fatalf("makeBotCommandsDOList(valid) = (%v, %v), want one bot command DO", doList, err)
	}
}

func TestUserSetBotCommandsPropagatesTransactionBeginError(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://bot_commands:bot_commands@127.0.0.1:1/teamgram_bot_commands_error_test")
	if err != nil {
		t.Fatalf("create lazy test database pool: %v", err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	core := &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Postgres: &userdao.Postgres{
			Pool:  pool,
			Store: postgres_dao.NewStore(pool),
		}}},
	}

	got, err := core.UserSetBotCommands(&user.TLUserSetBotCommands{
		UserId: 1,
		BotId:  2,
		Commands: []*mtproto.BotCommand{{
			Command:     "start",
			Description: "Start",
		}},
	})
	if got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("UserSetBotCommands() = (%v, %v), want canceled bot lookup error", got, err)
	}
}

func TestUserSetBotCommandsPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEAMGRAM_POSTGRES_DSN must point to a PostgreSQL 18 test database")
	}
	pg, err := userdao.NewPostgres(postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	t.Cleanup(pg.Close)

	var version int
	if err := pg.Pool.QueryRow(context.Background(), `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL version is %d, want 18.x", version)
	}

	ctx := context.Background()
	creatorID := time.Now().UnixNano()
	botID := creatorID + 1
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO users(id, phone, is_bot) VALUES
($1, $2, FALSE), ($3, $4, TRUE)`, creatorID, fmt.Sprintf("pg-test-user-%d", creatorID), botID, fmt.Sprintf("pg-test-bot-%d", botID)); err != nil {
		t.Fatalf("insert bot owner and bot users: %v", err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO bots(bot_id, creator_user_id, token) VALUES($1, $2, $3)`, botID, creatorID, fmt.Sprintf("pg-test-token-%d", botID)); err != nil {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::bigint[])`, []int64{creatorID, botID})
		t.Fatalf("insert bot registry row: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM bot_commands WHERE bot_id = $1`, botID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM bots WHERE bot_id = $1`, botID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::bigint[])`, []int64{creatorID, botID})
	})

	core := &UserCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}}}
	request := &user.TLUserSetBotCommands{
		UserId: creatorID,
		BotId:  botID,
		Commands: []*mtproto.BotCommand{
			{Command: "start", Description: "Start"},
			{Command: "help", Description: "Help"},
		},
	}
	if result, err := core.UserSetBotCommands(request); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set commands = (%v, %v), want true", result, err)
	}
	stored, err := pg.Store.BotCommands.SelectList(ctx, botID)
	if err != nil || len(stored) != 2 || stored[0].Command != "start" || stored[1].Command != "help" {
		t.Fatalf("stored commands = (%v, %v), want ordered start/help", stored, err)
	}

	request.UserId = creatorID + 2
	if result, err := core.UserSetBotCommands(request); result != nil || !errors.Is(err, mtproto.ErrForbiddenUserBotInvalid) {
		t.Fatalf("unauthorized replacement = (%v, %v), want forbidden", result, err)
	}
	stored, err = pg.Store.BotCommands.SelectList(ctx, botID)
	if err != nil || len(stored) != 2 {
		t.Fatalf("commands after rejected replacement = (%v, %v), want unchanged", stored, err)
	}

	request.UserId = creatorID
	request.Commands = nil
	if result, err := core.UserSetBotCommands(request); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("clear commands = (%v, %v), want true", result, err)
	}
	stored, err = pg.Store.BotCommands.SelectList(ctx, botID)
	if err != nil || len(stored) != 0 {
		t.Fatalf("commands after clear = (%v, %v), want empty", stored, err)
	}
}
