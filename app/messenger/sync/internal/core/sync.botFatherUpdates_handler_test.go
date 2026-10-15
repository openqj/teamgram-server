package core

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
)

func TestBotFatherCommandParsing(t *testing.T) {
	tests := []struct {
		text, command, args string
	}{
		{text: "/newbot", command: "newbot"},
		{text: "/setname@BotFather @mybot", command: "setname", args: "@mybot"},
		{text: "plain text", args: "plain text"},
		{text: "/cancel.", command: "cancel."},
	}
	for _, tt := range tests {
		command, args := parseBotFatherCommand(tt.text)
		if command != tt.command || args != tt.args {
			t.Fatalf("parseBotFatherCommand(%q) = (%q, %q), want (%q, %q)", tt.text, command, args, tt.command, tt.args)
		}
	}
}

func TestBotFatherSessionTransitions(t *testing.T) {
	session := &dao.BotFatherSession{UserID: 7, State: botFatherIdle}
	core := &SyncCore{}

	reply, err := core.processBotFatherText(context.Background(), session.UserID, session, "/newbot")
	if err != nil || session.State != botFatherAwaitName || !strings.Contains(reply, "choose a name") {
		t.Fatalf("/newbot = (%q, %v), state=%q", reply, err, session.State)
	}
	reply, err = core.processBotFatherText(context.Background(), session.UserID, session, "Example Bot")
	if err != nil || session.State != botFatherAwaitUsername || !strings.Contains(reply, "username") {
		t.Fatalf("name = (%q, %v), state=%q", reply, err, session.State)
	}
	reply, err = core.processBotFatherText(context.Background(), session.UserID, session, "/cancel.")
	if err != nil || session.State != botFatherAwaitUsername || !strings.Contains(reply, "/cancel") {
		t.Fatalf("/cancel. = (%q, %v), state=%q", reply, err, session.State)
	}
	reply, err = core.processBotFatherText(context.Background(), session.UserID, session, "/cancel")
	if err != nil || session.State != botFatherIdle || reply != "Canceled." {
		t.Fatalf("/cancel = (%q, %v), state=%q", reply, err, session.State)
	}
}

func TestParseBotFatherMenuButton(t *testing.T) {
	tests := []struct {
		name string
		text string
		want botapi.MenuButton
		ok   bool
	}{
		{name: "default", text: "/default", want: botapi.MenuButton{Type: "default"}, ok: true},
		{name: "commands", text: "commands", want: botapi.MenuButton{Type: "commands"}, ok: true},
		{name: "web app", text: "Open\nhttps://example.com/app", want: botapi.MenuButton{Type: "web_app", Text: "Open", WebApp: &botapi.WebAppInfo{URL: "https://example.com/app"}}, ok: true},
		{name: "insecure web app", text: "Open\nhttp://example.com/app", ok: false},
		{name: "missing URL", text: "Open", ok: false},
	}
	for _, tt := range tests {
		got, ok := parseBotFatherMenuButton(tt.text)
		if ok != tt.ok {
			t.Fatalf("%s: parseBotFatherMenuButton(%q) ok=%v, want %v", tt.name, tt.text, ok, tt.ok)
		}
		if ok && (got.Type != tt.want.Type || got.Text != tt.want.Text || menuButtonURL(got) != menuButtonURL(tt.want)) {
			t.Fatalf("%s: got %#v, want %#v", tt.name, got, tt.want)
		}
	}
}

func menuButtonURL(b botapi.MenuButton) string {
	if b.WebApp == nil {
		return ""
	}
	return b.WebApp.URL
}

func TestParseBotFatherCommandsLimitsAndDuplicates(t *testing.T) {
	if commands, ok := parseBotFatherCommands("start - Start the bot\nhelp - Show help"); !ok || len(commands) != 2 {
		t.Fatalf("valid command list = (%#v, %v)", commands, ok)
	}
	if _, ok := parseBotFatherCommands("start - Start\nstart - Duplicate"); ok {
		t.Fatal("duplicate command was accepted")
	}
	if _, ok := parseBotFatherCommands("start - " + strings.Repeat("x", 257)); ok {
		t.Fatal("overlong command description was accepted")
	}
}

func TestBotFatherProfileTextValidation(t *testing.T) {
	if !validBotFatherProfileText("line 1\nline 2", 128, true) {
		t.Fatal("line breaks should be allowed in about text")
	}
	if validBotFatherProfileText("bad\x00text", 128, true) {
		t.Fatal("NUL was accepted in profile text")
	}
	if validBotFatherProfileText(strings.Repeat("x", 129), 128, true) {
		t.Fatal("overlong profile text was accepted")
	}
}
