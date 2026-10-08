package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
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
