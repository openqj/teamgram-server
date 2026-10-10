package persist

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestInlineBotQueryLifecycleMemory(t *testing.T) {
	previous := Default
	Default = &mem{}
	t.Cleanup(func() { Default = previous })

	botID, requesterID, queryID := int64(88001), int64(88002), int64(88003)
	if err := CreateInlineBotQuery(InlineBotQuery{
		QueryID: queryID, BotID: botID, RequesterID: requesterID,
		PeerKey: "self:88002",
		Query:   "hello", Offset: "", ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"id":"r1","type":"article"}`)
	if err := SetInlineBotResults(botID, queryID, false, false, 30, "next", []json.RawMessage{result}); err != nil {
		t.Fatal(err)
	}
	got, err := FindInlineBotQuery(botID, requesterID, "self:88002", "hello", "")
	if err != nil || got == nil || !got.Ready || got.QueryID != queryID || got.NextOffset != "next" || len(got.Results) != 1 {
		t.Fatalf("query = %+v, err=%v", got, err)
	}
	if err := SetInlineBotResults(botID, queryID, false, false, 30, "next", []json.RawMessage{result}); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if err := SetInlineBotResults(botID, queryID, true, false, 30, "next", []json.RawMessage{result}); !errors.Is(err, ErrInlineQueryConflict) {
		t.Fatalf("conflicting retry = %v, want conflict", err)
	}
	if err := SetInlineBotResults(botID, queryID+1, false, false, 30, "", nil); !errors.Is(err, ErrInlineQueryNotFound) {
		t.Fatalf("unknown query = %v, want not found", err)
	}
}

func TestInlineBotQueryExpiryAndIsolation(t *testing.T) {
	previous := Default
	Default = &mem{}
	t.Cleanup(func() { Default = previous })

	if err := CreateInlineBotQuery(InlineBotQuery{
		QueryID: 99001, BotID: 99002, RequesterID: 99003,
		PeerKey: "self:99003",
		Query:   "q", ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := FindInlineBotQuery(99002, 99003, "self:99003", "q", ""); err != nil || got != nil {
		t.Fatalf("expired query = %+v, err=%v", got, err)
	}
	if err := CreateInlineBotQuery(InlineBotQuery{
		QueryID: 99004, BotID: 99002, RequesterID: 99003,
		PeerKey: "chat:7001",
		Query:   "q", Offset: "1", ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"id":"r2","type":"article"}`)
	if err := SetInlineBotResultsForBot(99003, 99004, false, false, 30, "", []json.RawMessage{result}); !errors.Is(err, ErrInlineQueryNotFound) {
		t.Fatalf("cross-bot query = %v, want not found", err)
	}
	if err := SetInlineBotResults(99002, 99004, false, false, 30, "", []json.RawMessage{result}); err != nil {
		t.Fatal(err)
	}
	if got, err := FindInlineBotQuery(99002, 99999, "chat:7001", "q", "1"); err != nil || got != nil {
		t.Fatalf("cross-requester query = %+v, err=%v", got, err)
	}
	if got, err := FindInlineBotQuery(99002, 99003, "chat:7002", "q", "1"); err != nil || got != nil {
		t.Fatalf("cross-peer query = %+v, err=%v", got, err)
	}
}

func TestSetInlineBotResultsByQueryUsesStoredBotPartition(t *testing.T) {
	previous := Default
	Default = &mem{}
	t.Cleanup(func() { Default = previous })
	query := InlineBotQuery{
		QueryID: 991101, BotID: 991102, RequesterID: 991103,
		PeerKey: "self:991103", Query: "partition", ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	if err := CreateInlineBotQuery(query); err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"id":"partition-result","type":"article"}`)
	if err := SetInlineBotResultsByQuery(query.QueryID, false, false, 30, "", []json.RawMessage{result}); err != nil {
		t.Fatal(err)
	}
	got, err := FindInlineBotQuery(query.BotID, query.RequesterID, query.PeerKey, query.Query, "")
	if err != nil || got == nil || !got.Ready || len(got.Results) != 1 {
		t.Fatalf("query = %+v, err=%v", got, err)
	}
}

func TestInlineBotQueryPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	t.Cleanup(func() {
		if store, ok := Default.(*postgresStore); ok {
			_, _ = store.db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, inlineBotQueryKey(991002))
		}
		_ = ClosePostgres()
		Default = previous
	})
	query := InlineBotQuery{QueryID: 991003, BotID: 991002, RequesterID: 991004, PeerKey: "self:991004", Query: "pg", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	if err := CreateInlineBotQuery(query); err != nil {
		t.Fatal(err)
	}
	payload := []json.RawMessage{json.RawMessage(`{"id":"pg-result","type":"article"}`)}
	if err := SetInlineBotResults(query.BotID, query.QueryID, true, true, 45, "pg-next", payload); err != nil {
		t.Fatal(err)
	}
	got, err := FindInlineBotQuery(query.BotID, query.RequesterID, query.PeerKey, query.Query, "")
	if err != nil || got == nil || !got.Ready || !got.Gallery || !got.Private || got.NextOffset != "pg-next" || len(got.Results) != 1 {
		t.Fatalf("postgres query = %+v, err=%v", got, err)
	}
}
