package core

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestBotPaymentOutboxBuildsCanonicalQueryUpdates(t *testing.T) {
	contextPayload, err := encodeBotPaymentContext(botPaymentContext{
		Version:        botPaymentContextVersion,
		QueryID:        910001,
		BuyerUserID:    910002,
		BotUserID:      910003,
		MsgID:          77,
		Peer:           mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(),
		InvoicePayload: []byte("invoice-payload"),
		Invoice: mtproto.MakeTLInvoice(&mtproto.Invoice{
			Currency: "USD",
			Prices:   []*mtproto.LabeledPrice{{Label: "item", Amount: 1299}},
		}).To_Invoice(),
		Currency: "USD",
		Amount:   1299,
		Info: mtproto.MakeTLPaymentRequestedInfo(&mtproto.PaymentRequestedInfo{
			ShippingAddress: mtproto.MakeTLPostAddress(&mtproto.PostAddress{CountryIso2: "US"}).To_PostAddress(),
		}).To_PaymentRequestedInfo(),
	})
	if err != nil {
		t.Fatalf("encode context: %v", err)
	}

	shippingPayload, err := json.Marshal(botPaymentOutboxPayload{
		QueryID: 910001, BotUserID: 910003, BuyerUserID: 910002,
		QueryPayload: contextPayload, Currency: "USD", Amount: 1299,
	})
	if err != nil {
		t.Fatalf("encode shipping payload: %v", err)
	}
	shipping, err := botPaymentUpdate(persist.BotPaymentOutbox{
		QueryID: 910001, BotUserID: 910003, BuyerUserID: 910002,
		EventType: persist.BotPaymentOutboxEventShippingQuery, Payload: shippingPayload,
	})
	if err != nil {
		t.Fatalf("build shipping update: %v", err)
	}
	if shipping == nil || len(shipping.GetUpdates()) != 1 || shipping.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateBotShippingQuery {
		t.Fatalf("shipping update = %#v", shipping)
	}
	shippingQuery := shipping.GetUpdates()[0]
	if shippingQuery.GetQueryId_INT64() != 910001 || shippingQuery.GetUserId() != 910002 || string(shippingQuery.GetPayload_BYTES()) != "invoice-payload" || shippingQuery.GetShippingAddress() == nil || shippingQuery.GetShippingAddress().GetCountryIso2() != "US" {
		t.Fatalf("shipping query = %#v", shippingQuery)
	}

	context, err := decodeBotPaymentContext(contextPayload)
	if err != nil {
		t.Fatalf("decode context: %v", err)
	}
	context.ShippingOptionID = "standard"
	contextPayload, err = encodeBotPaymentContext(context)
	if err != nil {
		t.Fatalf("encode precheckout context: %v", err)
	}
	precheckoutPayload, err := json.Marshal(botPaymentOutboxPayload{
		QueryID: 910001, BotUserID: 910003, BuyerUserID: 910002,
		QueryPayload: contextPayload, Currency: "USD", Amount: 1299,
	})
	if err != nil {
		t.Fatalf("encode precheckout payload: %v", err)
	}
	precheckout, err := botPaymentUpdate(persist.BotPaymentOutbox{
		QueryID: 910001, BotUserID: 910003, BuyerUserID: 910002,
		EventType: persist.BotPaymentOutboxEventPrecheckoutQuery, Payload: precheckoutPayload,
	})
	if err != nil {
		t.Fatalf("build precheckout update: %v", err)
	}
	if precheckout == nil || len(precheckout.GetUpdates()) != 1 || precheckout.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateBotPrecheckoutQuery {
		t.Fatalf("precheckout update = %#v", precheckout)
	}
	precheckoutQuery := precheckout.GetUpdates()[0]
	if precheckoutQuery.GetQueryId_INT64() != 910001 || precheckoutQuery.GetUserId() != 910002 || string(precheckoutQuery.GetPayload_BYTES()) != "invoice-payload" || precheckoutQuery.GetCurrency() != "USD" || precheckoutQuery.GetTotalAmount() != 1299 || precheckoutQuery.GetInfo_FLAGPAYMENTREQUESTEDINFO() == nil || precheckoutQuery.GetShippingOptionId() == nil || precheckoutQuery.GetShippingOptionId().GetValue() != "standard" {
		t.Fatalf("precheckout query = %#v", precheckoutQuery)
	}
}
