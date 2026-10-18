package persist

import (
	"errors"
	"testing"
	"time"
)

func TestBotCallbackQueryAnswerLifecycleMemory(t *testing.T) {
	previous := Default
	Default = &mem{}
	t.Cleanup(func() { Default = previous })

	query := BotCallbackQuery{
		QueryID: 992001, BotID: 992002, UserID: 992003,
		PeerType: 2, PeerID: 992002, MessageID: 17,
		ChatInstance: 992004, Data: []byte("ping"),
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	if err := CreateBotCallbackQuery(query); err != nil {
		t.Fatal(err)
	}
	if err := SetBotCallbackAnswer(query.BotID, query.QueryID, BotCallbackAnswer{Message: "ok", CacheTime: 3}); err != nil {
		t.Fatal(err)
	}
	got, err := FindBotCallbackQuery(query.QueryID)
	if err != nil || got == nil || got.BotID != query.BotID || string(got.Data) != "ping" || got.Answer == nil || got.Answer.Message != "ok" {
		t.Fatalf("callback query = %+v, err=%v", got, err)
	}
	if err := SetBotCallbackAnswer(query.BotID+1, query.QueryID, BotCallbackAnswer{}); !errors.Is(err, ErrBotCallbackQueryNotFound) {
		t.Fatalf("cross-bot answer = %v, want not found", err)
	}
}
