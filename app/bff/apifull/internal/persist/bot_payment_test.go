package persist

import (
	"errors"
	"testing"
	"time"
)

func TestBotPaymentQueryCallbacksAreAtomicAndIdempotent(t *testing.T) {
	previous := Default
	Default = &mem{}
	t.Cleanup(func() { Default = previous })

	queryID := int64(970001)
	query := BotPaymentQuery{
		QueryID:     queryID,
		BotUserID:   970002,
		BuyerUserID: 970003,
		Stage:       BotPaymentStageShippingPending,
		ExpiresAt:   time.Now().Unix() + 300,
	}
	if err := CreateBotPaymentQuery(query); err != nil {
		t.Fatalf("create query: %v", err)
	}
	if err := SetBotShippingResults(query.BotUserID, queryID, "", []byte(`[{"id":"standard"}]`)); err != nil {
		t.Fatalf("set shipping: %v", err)
	}
	if err := SetBotShippingResults(query.BotUserID, queryID, "", []byte(`[{"id":"standard"}]`)); err != nil {
		t.Fatalf("repeat shipping: %v", err)
	}
	if err := SetBotShippingResults(query.BotUserID+1, queryID, "", []byte(`[]`)); !errors.Is(err, ErrBotPaymentQueryOwner) {
		t.Fatalf("wrong owner error = %v", err)
	}
	if err := AdvanceBotPaymentQuery(queryID, query.BotUserID, BotPaymentStagePrecheckoutPending); err != nil {
		t.Fatalf("advance query: %v", err)
	}
	if err := SetBotPrecheckoutResults(query.BotUserID, queryID, true, ""); err != nil {
		t.Fatalf("set precheckout: %v", err)
	}
	if err := SetBotPrecheckoutResults(query.BotUserID, queryID, true, ""); err != nil {
		t.Fatalf("repeat precheckout: %v", err)
	}
	got, found, err := LoadBotPaymentQuery(queryID)
	if err != nil || !found {
		t.Fatalf("load query = (%+v, %v, %v)", got, found, err)
	}
	if got.Stage != BotPaymentStagePrecheckoutAnswered || got.PrecheckoutSuccess == nil || !*got.PrecheckoutSuccess {
		t.Fatalf("query state = %+v", got)
	}
}

func TestBotPaymentQueryExpiredAndUnknownFailClosed(t *testing.T) {
	previous := Default
	Default = &mem{}
	t.Cleanup(func() { Default = previous })

	if err := SetBotPrecheckoutResults(1, 970099, true, ""); !errors.Is(err, ErrBotPaymentQueryNotFound) {
		t.Fatalf("unknown query error = %v", err)
	}
	err := CreateBotPaymentQuery(BotPaymentQuery{
		QueryID:     970100,
		BotUserID:   1,
		BuyerUserID: 2,
		Stage:       BotPaymentStagePrecheckoutPending,
		ExpiresAt:   time.Now().Unix() - 1,
	})
	if !errors.Is(err, ErrBotPaymentQueryExpired) {
		t.Fatalf("expired query create error = %v", err)
	}
}
