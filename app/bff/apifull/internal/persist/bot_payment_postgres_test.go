package persist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestBotPaymentPostgresPersistenceAndOutbox(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	store, ok := Default.(*postgresStore)
	if !ok {
		t.Fatal("postgres store was not installed")
	}
	queryID := 990000000000 + time.Now().UnixNano()%1000000000
	botID := int64(980000000 + os.Getpid())
	buyerID := botID + 1
	cleanup := func() {
		if store != nil && store.db != nil {
			_, _ = store.db.Exec(`DELETE FROM apifull_bot_payment_query WHERE query_id = $1`, queryID)
		}
		_ = ClosePostgres()
		Default = previous
	}
	t.Cleanup(cleanup)

	query := BotPaymentQuery{
		QueryID:     queryID,
		BotUserID:   botID,
		BuyerUserID: buyerID,
		Stage:       BotPaymentStageShippingPending,
		Payload:     []byte(`{"invoice":"payload"}`),
		Currency:    "USD",
		Amount:      1299,
		ExpiresAt:   time.Now().UTC().Add(5 * time.Minute).Unix(),
	}
	if err := CreateBotPaymentQueryAndDelivery(query); err != nil {
		t.Fatal(err)
	}
	initial, err := ClaimBotPaymentOutbox(context.Background(), 10)
	if err != nil || len(initial) != 1 || initial[0].EventType != BotPaymentOutboxEventShippingQuery {
		t.Fatalf("initial shipping claim = %+v err=%v", initial, err)
	}
	if initial[0].ClaimToken == "" {
		t.Fatal("initial shipping claim did not include a lease token")
	}
	if err := CompleteBotPaymentOutbox(context.Background(), initial[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := CreateBotPaymentQueryAndDelivery(query); err != nil {
		t.Fatalf("idempotent create: %v", err)
	}

	loaded, found, err := LoadBotPaymentQuery(queryID)
	if err != nil || !found {
		t.Fatalf("load after create: found=%v err=%v", found, err)
	}
	if loaded.BotUserID != botID || loaded.BuyerUserID != buyerID || loaded.Stage != BotPaymentStageShippingPending ||
		loaded.Currency != query.Currency || loaded.Amount != query.Amount || string(loaded.Payload) != string(query.Payload) {
		t.Fatalf("loaded query = %+v", loaded)
	}

	if err := SetBotShippingResults(botID+100, queryID, "", []byte(`[]`)); !errors.Is(err, ErrBotPaymentQueryOwner) {
		t.Fatalf("wrong owner error = %v", err)
	}
	shippingOptions := []byte(`[{"id":"standard","title":"Standard","prices":[]}]`)
	if err := SetBotShippingResults(botID, queryID, "", shippingOptions); err != nil {
		t.Fatal(err)
	}
	if err := SetBotShippingResults(botID, queryID, "", []byte(` [ { "id": "standard", "title": "Standard", "prices": [] } ] `)); err != nil {
		t.Fatalf("idempotent shipping callback: %v", err)
	}
	if err := SetBotShippingResults(botID, queryID, "different", shippingOptions); !errors.Is(err, ErrBotPaymentQueryState) {
		t.Fatalf("conflicting shipping callback error = %v", err)
	}

	claimed, err := ClaimBotPaymentOutbox(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].QueryID != queryID || claimed[0].EventType != BotPaymentOutboxEventShipping ||
		claimed[0].State != BotPaymentOutboxStateInflight || claimed[0].Attempts != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("shipping claim = %+v", claimed)
	}
	var shippingPayload struct {
		QueryPayload    json.RawMessage `json:"query_payload"`
		Currency        string          `json:"currency"`
		Amount          int64           `json:"amount"`
		ShippingOptions json.RawMessage `json:"shipping_options"`
	}
	if err := json.Unmarshal(claimed[0].Payload, &shippingPayload); err != nil || !json.Valid(shippingPayload.ShippingOptions) {
		t.Fatalf("shipping outbox payload = %s, err=%v", claimed[0].Payload, err)
	}
	if !jsonEqual(shippingPayload.QueryPayload, query.Payload) || shippingPayload.Currency != query.Currency || shippingPayload.Amount != query.Amount {
		t.Fatalf("shipping context = payload %s currency %q amount %d", shippingPayload.QueryPayload, shippingPayload.Currency, shippingPayload.Amount)
	}
	if next, err := ClaimBotPaymentOutbox(context.Background(), 10); err != nil || len(next) != 0 {
		t.Fatalf("claimed leased item again: items=%+v err=%v", next, err)
	}
	if err := RetryBotPaymentOutbox(context.Background(), claimed[0].ID, fmt.Errorf("temporary delivery failure")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE apifull_bot_payment_outbox SET next_attempt_at = CURRENT_TIMESTAMP - INTERVAL '1 second' WHERE id = $1`, claimed[0].ID); err != nil {
		t.Fatal(err)
	}
	retried, err := ClaimBotPaymentOutbox(context.Background(), 10)
	if err != nil || len(retried) != 1 || retried[0].ID != claimed[0].ID || retried[0].Attempts != 2 {
		t.Fatalf("retried claim = %+v err=%v", retried, err)
	}
	if err := CompleteBotPaymentOutboxWithToken(context.Background(), retried[0].ID, claimed[0].ClaimToken); !errors.Is(err, ErrBotPaymentOutboxState) {
		t.Fatalf("stale lease completion error = %v", err)
	}
	if err := RetryBotPaymentOutboxWithToken(context.Background(), retried[0].ID, claimed[0].ClaimToken, errors.New("stale worker")); !errors.Is(err, ErrBotPaymentOutboxState) {
		t.Fatalf("stale lease retry error = %v", err)
	}
	if err := CompleteBotPaymentOutbox(context.Background(), retried[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := CompleteBotPaymentOutbox(context.Background(), retried[0].ID); err != nil {
		t.Fatalf("idempotent complete: %v", err)
	}

	if err := AdvanceBotPaymentQuery(queryID, botID+100, BotPaymentStagePrecheckoutPending); !errors.Is(err, ErrBotPaymentQueryOwner) {
		t.Fatalf("advance wrong owner error = %v", err)
	}
	precheckoutPayload := []byte(`{"invoice":"payload","requested_info":{"shipping_address":{"country":"US"}},"shipping_option_id":"standard"}`)
	if err := AdvanceBotPaymentQueryWithContext(queryID, botID, precheckoutPayload, query.Currency, query.Amount); err != nil {
		t.Fatal(err)
	}
	initialPrecheckout, err := ClaimBotPaymentOutbox(context.Background(), 10)
	if err != nil || len(initialPrecheckout) != 1 || initialPrecheckout[0].EventType != BotPaymentOutboxEventPrecheckoutQuery {
		t.Fatalf("initial precheckout claim = %+v err=%v", initialPrecheckout, err)
	}
	if err := CompleteBotPaymentOutbox(context.Background(), initialPrecheckout[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := SetBotPrecheckoutResults(botID, queryID, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := SetBotPrecheckoutResults(botID, queryID, true, ""); err != nil {
		t.Fatalf("idempotent precheckout callback: %v", err)
	}
	precheckout, err := ClaimBotPaymentOutbox(context.Background(), 10)
	if err != nil || len(precheckout) != 1 || precheckout[0].EventType != BotPaymentOutboxEventPrecheckout {
		t.Fatalf("precheckout claim = %+v err=%v", precheckout, err)
	}
	if err := CompleteBotPaymentOutbox(context.Background(), precheckout[0].ID); err != nil {
		t.Fatal(err)
	}

	if err := ClosePostgres(); err != nil {
		t.Fatal(err)
	}
	Default = previous
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	store, ok = Default.(*postgresStore)
	if !ok {
		t.Fatal("postgres store was not restored after reopen")
	}
	loaded, found, err = LoadBotPaymentQuery(queryID)
	if err != nil || !found || loaded.Stage != BotPaymentStagePrecheckoutAnswered || loaded.PrecheckoutSuccess == nil || !*loaded.PrecheckoutSuccess {
		t.Fatalf("load after reopen: query=%+v found=%v err=%v", loaded, found, err)
	}
	if !jsonEqual(loaded.Payload, precheckoutPayload) || loaded.Currency != query.Currency || loaded.Amount != query.Amount {
		t.Fatalf("precheckout context after reopen = payload %s currency %q amount %d", loaded.Payload, loaded.Currency, loaded.Amount)
	}
}
