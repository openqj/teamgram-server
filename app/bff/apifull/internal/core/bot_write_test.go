package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestBotResetCommandsRejectsNilRequest(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.BotsResetBotCommands(nil); !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("BotsResetBotCommands(nil) = %v, want INPUT_CONSTRUCTOR_INVALID", err)
	}
}

func TestBotCommandMethodsRejectUnsupportedScopeOrLanguage(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	unsupported := mtproto.MakeTLBotCommandScopeUsers(&mtproto.BotCommandScope{}).To_BotCommandScope()
	defaultScope := mtproto.MakeTLBotCommandScopeDefault(&mtproto.BotCommandScope{}).To_BotCommandScope()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "set unsupported scope",
			call: func() error {
				_, err := c.BotsSetBotCommands(&mtproto.TLBotsSetBotCommands{Scope: unsupported})
				return err
			},
		},
		{
			name: "reset localized commands",
			call: func() error {
				_, err := c.BotsResetBotCommands(&mtproto.TLBotsResetBotCommands{Scope: defaultScope, LangCode: "en"})
				return err
			},
		},
		{
			name: "get unsupported scope",
			call: func() error {
				_, err := c.BotsGetBotCommands(&mtproto.TLBotsGetBotCommands{Scope: unsupported})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("got %v, want METHOD_NOT_IMPL", err)
			}
		})
	}
}

func TestBotCommandMethodsRejectHumanCallers(t *testing.T) {
	const userID = int64(81024001)
	client := &botCommandsUserClient{profile: &mtproto.ImmutableUser{User: &mtproto.UserData{Id: userID}}}
	c := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
	scope := mtproto.MakeTLBotCommandScopeDefault(&mtproto.BotCommandScope{}).To_BotCommandScope()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "set", call: func() error {
			_, err := c.BotsSetBotCommands(&mtproto.TLBotsSetBotCommands{Scope: scope})
			return err
		}},
		{name: "reset", call: func() error {
			_, err := c.BotsResetBotCommands(&mtproto.TLBotsResetBotCommands{Scope: scope})
			return err
		}},
		{name: "get", call: func() error {
			_, err := c.BotsGetBotCommands(&mtproto.TLBotsGetBotCommands{Scope: scope})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, mtproto.ErrBotMethodInvalid) {
				t.Fatalf("human caller error = %v, want BOT_METHOD_INVALID", err)
			}
		})
	}
	if client.setCalls != 0 || client.infoCalls != 0 {
		t.Fatalf("human caller reached command provider: set=%d info=%d", client.setCalls, client.infoCalls)
	}
}
