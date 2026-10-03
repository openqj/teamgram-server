package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
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

func TestUserSetBotCommandsPropagatesBotLookupError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{
		DSN: "bot_commands:bot_commands@tcp(127.0.0.1:1)/teamgram_bot_commands_error_test?timeout=50ms&readTimeout=50ms&writeTimeout=50ms",
	})
	if err != nil {
		t.Fatalf("open lazy test database connection: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	core := &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			DB:             db,
			BotsDAO:        mysql_dao.NewBotsDAO(db),
			BotCommandsDAO: mysql_dao.NewBotCommandsDAO(db),
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
