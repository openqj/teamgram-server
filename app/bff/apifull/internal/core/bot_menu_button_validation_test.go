package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestValidateBotMenuButton(t *testing.T) {
	tests := []struct {
		name   string
		button *mtproto.BotMenuButton
		want   error
	}{
		{name: "default", button: mtproto.MakeTLBotMenuButtonDefault(nil).To_BotMenuButton()},
		{name: "commands", button: mtproto.MakeTLBotMenuButtonCommands(nil).To_BotMenuButton()},
		{name: "url", button: mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Text: "Open", Url: "https://example.test/app"}).To_BotMenuButton()},
		{name: "nil", want: mtproto.ErrInputConstructorInvalid},
		{name: "unknown constructor", button: &mtproto.BotMenuButton{Text: "Open", Url: "https://example.test", PredicateName: "botMenuButtonUnknown"}, want: mtproto.ErrInputConstructorInvalid},
		{name: "default with fields", button: &mtproto.BotMenuButton{Text: "Open", PredicateName: mtproto.Predicate_botMenuButtonDefault}, want: mtproto.ErrInputRequestInvalid},
		{name: "url missing text", button: mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Url: "https://example.test"}).To_BotMenuButton(), want: mtproto.ErrInputRequestInvalid},
		{name: "url must be https", button: mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Text: "Open", Url: "http://example.test"}).To_BotMenuButton(), want: mtproto.ErrWebdocumentUrlInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBotMenuButton(tt.button)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("validateBotMenuButton() = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("validateBotMenuButton() = %v, want %v", err, tt.want)
			}
		})
	}
}
