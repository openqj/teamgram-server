package http

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/teamgram-server/app/botapi"
)

func TestBotAPICommandVariant(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		language    string
		wantScope   string
		wantLang    string
		wantDefault bool
		wantErr     string
	}{
		{name: "default", wantScope: "default", wantDefault: true},
		{name: "all private", scope: `{"type":"all_private_chats"}`, wantScope: "all_private_chats"},
		{name: "numeric chat", scope: `{"type":"chat","chat_id":-100123}`, wantScope: "chat:-100123"},
		{name: "username chat", scope: `{"type":"chat_administrators","chat_id":"@ExampleBot"}`, wantScope: "chat_administrators:@examplebot"},
		{name: "chat member", scope: `{"type":"chat_member","chat_id":-42,"user_id":7}`, wantScope: "chat_member:-42:7"},
		{name: "localized", language: `"ZH"`, wantScope: "default", wantLang: "zh"},
		{name: "missing chat id", scope: `{"type":"chat"}`, wantErr: "scope.chat_id is invalid"},
		{name: "empty chat id", scope: `{"type":"chat","chat_id":""}`, wantErr: "scope.chat_id is invalid"},
		{name: "empty username", scope: `{"type":"chat","chat_id":"@"}`, wantErr: "scope.chat_id is invalid"},
		{name: "invalid language", language: `"zh-hans"`, wantErr: "language_code is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]json.RawMessage{}
			if test.scope != "" {
				values["scope"] = json.RawMessage(test.scope)
			}
			if test.language != "" {
				values["language_code"] = json.RawMessage(test.language)
			}
			scope, language, defaultVariant, err := botAPICommandVariant(&botAPIRequest{values: values})
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("variant error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil || scope != test.wantScope || language != test.wantLang || defaultVariant != test.wantDefault {
				t.Fatalf("variant = (%q, %q, %t, %v), want (%q, %q, %t, nil)", scope, language, defaultVariant, err, test.wantScope, test.wantLang, test.wantDefault)
			}
		})
	}
}

func TestValidateBotAPICommandVariant(t *testing.T) {
	valid := []botapi.BotCommand{{Command: "start", Description: "Start"}}
	if err := validateBotAPICommandVariant(valid); err != nil {
		t.Fatalf("valid commands rejected: %v", err)
	}
	tests := []struct {
		name     string
		commands []botapi.BotCommand
	}{
		{name: "duplicate", commands: []botapi.BotCommand{{Command: "start", Description: "one"}, {Command: "start", Description: "two"}}},
		{name: "bad command", commands: []botapi.BotCommand{{Command: "Start", Description: "one"}}},
		{name: "empty description", commands: []botapi.BotCommand{{Command: "start", Description: " "}}},
		{name: "ephemeral", commands: []botapi.BotCommand{{Command: "start", Description: "one", IsEphemeral: true}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateBotAPICommandVariant(test.commands); err == nil {
				t.Fatal("invalid command variant was accepted")
			}
		})
	}
}
