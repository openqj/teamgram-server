package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestBotPaymentCallbacksUsePostgresPendingQuery(t *testing.T) {
	if !persist.PostgresEnabled() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	const (
		queryID = int64(970201)
		botID   = int64(970202)
		buyerID = int64(970203)
	)
	t.Cleanup(func() {
		raw, getErr := persist.Default.Get("bot-payment-query:970201")
		if getErr == nil && raw != "" {
			_, _ = persist.CompareAndDelete("bot-payment-query:970201", raw)
		}
	})
	if err := persist.CreateBotPaymentQuery(persist.BotPaymentQuery{
		QueryID: queryID, BotUserID: botID, BuyerUserID: buyerID,
		Stage: persist.BotPaymentStageShippingPending, ExpiresAt: time.Now().Unix() + 300,
	}); err != nil {
		t.Fatalf("create shipping query: %v", err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: botID}}
	shipping := &mtproto.TLMessagesSetBotShippingResults{
		QueryId: queryID,
		ShippingOptions: []*mtproto.ShippingOption{{
			Id: "standard",
		}},
	}
	result, err := c.MessagesSetBotShippingResults(shipping)
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("set shipping = (%v, %v)", result, err)
	}
	if result, err = c.MessagesSetBotShippingResults(shipping); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("repeat shipping = (%v, %v)", result, err)
	}
	if err = persist.AdvanceBotPaymentQuery(queryID, botID, persist.BotPaymentStagePrecheckoutPending); err != nil {
		t.Fatalf("advance to precheckout: %v", err)
	}
	precheckout, err := c.MessagesSetBotPrecheckoutResults(&mtproto.TLMessagesSetBotPrecheckoutResults{
		QueryId: queryID, Success: true,
	})
	if err != nil || precheckout != mtproto.BoolTrue {
		t.Fatalf("set precheckout = (%v, %v)", precheckout, err)
	}
	state, found, err := persist.LoadBotPaymentQuery(queryID)
	if err != nil || !found || state.Stage != persist.BotPaymentStagePrecheckoutAnswered || state.PrecheckoutSuccess == nil || !*state.PrecheckoutSuccess {
		t.Fatalf("query state = (%+v, %v, %v)", state, found, err)
	}
	wrongCaller := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: botID + 1}}
	if _, err = wrongCaller.MessagesSetBotPrecheckoutResults(&mtproto.TLMessagesSetBotPrecheckoutResults{QueryId: queryID, Success: true}); !errors.Is(err, mtproto.ErrQueryIdInvalid) {
		t.Fatalf("wrong caller error = %v", err)
	}

}
