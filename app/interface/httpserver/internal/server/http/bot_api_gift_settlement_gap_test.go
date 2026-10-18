package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func TestGiftSettlementMethodsRequireConfiguredServices(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"user_id":                json.RawMessage("7"),
		"gift_id":                json.RawMessage("101"),
		"star_count":             json.RawMessage("25"),
		"business_connection_id": json.RawMessage(`"business-1"`),
	}}
	for _, method := range []string{"sendGift", "giftPremiumSubscription", "transferBusinessAccountStars"} {
		t.Run(method, func(t *testing.T) {
			result, status, code, description := dispatchBotAPIMethod(
				&svc.ServiceContext{},
				httptest.NewRequest("POST", "/bot/token/"+method, nil),
				bot,
				strings.ToUpper(method),
				request,
			)
			want := "Not Implemented: payment service is not configured"
			if result != nil || status != 501 || code != 501 || description != want {
				t.Fatalf("dispatch = (%#v, %d, %d, %q), want service-config 501 %q", result, status, code, description, want)
			}
		})
	}
}

type giftSettlementPaymentsStub struct {
	mtproto.RPCPaymentsClient
}

type giftSettlementStarsStub struct {
	mtproto.RPCStarsClient
}

type giftSettlementPaymentsFormStub struct {
	mtproto.RPCPaymentsClient
	form *mtproto.Payments_PaymentForm
}

func (s *giftSettlementPaymentsFormStub) PaymentsGetPaymentForm(context.Context, *mtproto.TLPaymentsGetPaymentForm, ...grpc.CallOption) (*mtproto.Payments_PaymentForm, error) {
	return s.form, nil
}

type giftSettlementPaymentsUnsupportedStub struct {
	mtproto.RPCPaymentsClient
}

func (giftSettlementPaymentsUnsupportedStub) PaymentsGetPaymentForm(context.Context, *mtproto.TLPaymentsGetPaymentForm, ...grpc.CallOption) (*mtproto.Payments_PaymentForm, error) {
	return nil, mtproto.ErrPaymentUnsupported
}

type giftSettlementStarsFormStub struct {
	mtproto.RPCStarsClient
	calls   int
	callCtx context.Context
	invoice *mtproto.InputInvoice
}

func (s *giftSettlementStarsFormStub) PaymentsSendStarsForm(ctx context.Context, in *mtproto.TLPaymentsSendStarsForm, _ ...grpc.CallOption) (*mtproto.Payments_PaymentResult, error) {
	s.calls++
	s.callCtx = ctx
	s.invoice = in.GetInvoice()
	return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{Updates: mtproto.MakeEmptyUpdates()}).To_Payments_PaymentResult(), nil
}

type giftSettlementGiftsStub struct {
	mtproto.RPCGiftsClient
	upgradeCalls int
}

func (s *giftSettlementGiftsStub) PaymentsUpgradeStarGift(context.Context, *mtproto.TLPaymentsUpgradeStarGift, ...grpc.CallOption) (*mtproto.Updates, error) {
	s.upgradeCalls++
	return mtproto.MakeEmptyUpdates(), nil
}

type giftSettlementBusinessBotsStub struct {
	mtproto.RPCBusinessConnectedBotsClient
	updates *mtproto.Updates
}

func (s *giftSettlementBusinessBotsStub) AccountGetBotBusinessConnection(context.Context, *mtproto.TLAccountGetBotBusinessConnection, ...grpc.CallOption) (*mtproto.Updates, error) {
	return s.updates, nil
}

func giftSettlementStarsForm(stars int64) *mtproto.Payments_PaymentForm {
	invoice := mtproto.MakeTLInvoice(&mtproto.Invoice{
		Currency: "XTR",
		Prices: []*mtproto.LabeledPrice{mtproto.MakeTLLabeledPrice(&mtproto.LabeledPrice{
			Label:  "Stars",
			Amount: stars,
		}).To_LabeledPrice()},
	}).To_Invoice()
	return mtproto.MakeTLPaymentsPaymentFormStars(&mtproto.Payments_PaymentForm{FormId: 19, Invoice: invoice}).To_Payments_PaymentForm()
}

func giftSettlementGiftForm() *mtproto.Payments_PaymentForm {
	return mtproto.MakeTLPaymentsPaymentFormStarGift(&mtproto.Payments_PaymentForm{FormId: 19}).To_Payments_PaymentForm()
}

func giftSettlementBusinessConnection() *mtproto.Updates {
	connection := mtproto.MakeTLBotBusinessConnection(&mtproto.BotBusinessConnection{
		ConnectionId: "business-1",
		UserId:       99,
		Rights:       &mtproto.BusinessBotRights{TransferStars: true},
	}).To_BotBusinessConnection()
	update := mtproto.MakeTLUpdateBotBusinessConnect(&mtproto.Update{Connection: connection}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{Updates: []*mtproto.Update{update}}).To_Updates()
}

func giftSettlementGiftBusinessConnection() *mtproto.Updates {
	connection := mtproto.MakeTLBotBusinessConnection(&mtproto.BotBusinessConnection{
		ConnectionId: "business-1",
		UserId:       99,
		Rights:       &mtproto.BusinessBotRights{TransferAndUpgradeGifts: true, TransferStars: true},
	}).To_BotBusinessConnection()
	update := mtproto.MakeTLUpdateBotBusinessConnect(&mtproto.Update{Connection: connection}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{Updates: []*mtproto.Update{update}}).To_Updates()
}

func TestTransferBusinessAccountStarsRejectsMismatchedPaymentFormAmount(t *testing.T) {
	stars := &giftSettlementStarsFormStub{}
	service := &svc.ServiceContext{
		PaymentsClient:     &giftSettlementPaymentsFormStub{form: giftSettlementStarsForm(24)},
		StarsClient:        stars,
		BusinessBotsClient: &giftSettlementBusinessBotsStub{updates: giftSettlementBusinessConnection()},
	}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"business_connection_id": json.RawMessage(`"business-1"`),
		"star_count":             json.RawMessage("25"),
	}}
	result, status, code, description := transferBusinessAccountStars(service, httptest.NewRequest("POST", "/bot/token/transferBusinessAccountStars", nil), 42, request)
	if result != nil || status != 400 || code != 400 || description != "Bad Request: payment form Stars amount did not match star_count" {
		t.Fatalf("transfer with mismatched form = (%#v, %d, %d, %q), want amount rejection", result, status, code, description)
	}
	if stars.calls != 0 {
		t.Fatalf("PaymentsSendStarsForm calls = %d, want no settlement after amount mismatch", stars.calls)
	}
}

func TestGiftPremiumSubscriptionRejectsMismatchedPaymentFormAmount(t *testing.T) {
	stars := &giftSettlementStarsFormStub{}
	service := &svc.ServiceContext{
		PaymentsClient: &giftSettlementPaymentsFormStub{form: giftSettlementStarsForm(999)},
		StarsClient:    stars,
	}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"user_id":     json.RawMessage("42"),
		"month_count": json.RawMessage("3"),
		"star_count":  json.RawMessage("1000"),
	}}
	result, status, code, description := giftPremiumSubscription(service, httptest.NewRequest("POST", "/bot/token/giftPremiumSubscription", nil), 42, request)
	if result != nil || status != 400 || code != 400 || description != "Bad Request: payment form Stars amount did not match star_count" {
		t.Fatalf("premium gift with mismatched form = (%#v, %d, %d, %q), want amount rejection", result, status, code, description)
	}
	if stars.calls != 0 {
		t.Fatalf("PaymentsSendStarsForm calls = %d, want no settlement after amount mismatch", stars.calls)
	}
}

func TestGiftSettlementBuildsLayer229Invoices(t *testing.T) {
	t.Run("sendGift", func(t *testing.T) {
		stars := &giftSettlementStarsFormStub{}
		service := &svc.ServiceContext{
			PaymentsClient: &giftSettlementPaymentsFormStub{form: giftSettlementGiftForm()},
			StarsClient:    stars,
		}
		request := &botAPIRequest{values: map[string]json.RawMessage{
			"gift_id": json.RawMessage("101"),
			"user_id": json.RawMessage("42"),
		}}
		result, status, code, description := sendGift(service, httptest.NewRequest("POST", "/bot/token/sendGift", nil), 42, request)
		if result != true || status != 200 || code != 0 || description != "" {
			t.Fatalf("sendGift = (%#v, %d, %d, %q), want success", result, status, code, description)
		}
		if stars.calls != 1 || stars.invoice == nil || stars.invoice.GetPredicateName() != mtproto.Predicate_inputInvoiceStarGift || stars.invoice.GetGiftId() != 101 || stars.invoice.GetUserId() == nil || stars.invoice.GetUserId().GetPredicateName() != mtproto.Predicate_inputUserSelf {
			t.Fatalf("sendGift invoice/calls = (%d, %#v), want star gift to self", stars.calls, stars.invoice)
		}
	})

	t.Run("giftPremiumSubscription", func(t *testing.T) {
		stars := &giftSettlementStarsFormStub{}
		service := &svc.ServiceContext{
			PaymentsClient: &giftSettlementPaymentsFormStub{form: giftSettlementStarsForm(1000)},
			StarsClient:    stars,
		}
		request := &botAPIRequest{values: map[string]json.RawMessage{
			"user_id":     json.RawMessage("42"),
			"month_count": json.RawMessage("3"),
			"star_count":  json.RawMessage("1000"),
		}}
		result, status, code, description := giftPremiumSubscription(service, httptest.NewRequest("POST", "/bot/token/giftPremiumSubscription", nil), 42, request)
		if result != true || status != 200 || code != 0 || description != "" {
			t.Fatalf("giftPremiumSubscription = (%#v, %d, %d, %q), want success", result, status, code, description)
		}
		if stars.calls != 1 || stars.invoice == nil || stars.invoice.GetPredicateName() != mtproto.Predicate_inputInvoicePremiumGiftStars || stars.invoice.GetMonths() != 3 || stars.invoice.GetUserId() == nil || stars.invoice.GetUserId().GetPredicateName() != mtproto.Predicate_inputUserSelf {
			t.Fatalf("premium invoice/calls = (%d, %#v), want 3-month gift to self", stars.calls, stars.invoice)
		}
	})

	t.Run("transferBusinessAccountStars", func(t *testing.T) {
		stars := &giftSettlementStarsFormStub{}
		service := &svc.ServiceContext{
			PaymentsClient:     &giftSettlementPaymentsFormStub{form: giftSettlementStarsForm(25)},
			StarsClient:        stars,
			BusinessBotsClient: &giftSettlementBusinessBotsStub{updates: giftSettlementBusinessConnection()},
		}
		request := &botAPIRequest{values: map[string]json.RawMessage{
			"business_connection_id": json.RawMessage(`"business-1"`),
			"star_count":             json.RawMessage("25"),
		}}
		result, status, code, description := transferBusinessAccountStars(service, httptest.NewRequest("POST", "/bot/token/transferBusinessAccountStars", nil), 42, request)
		if result != true || status != 200 || code != 0 || description != "" {
			t.Fatalf("transferBusinessAccountStars = (%#v, %d, %d, %q), want success", result, status, code, description)
		}
		scope, ok := businessctx.ScopeFromContext(stars.callCtx)
		if !ok || scope.ConnectionID != "business-1" || scope.OwnerID != 99 || !scope.CanTransferStars {
			t.Fatalf("Stars call scope = (%+v, %t), want business owner 99 and transfer right", scope, ok)
		}
		if stars.invoice == nil || stars.invoice.GetPredicateName() != mtproto.Predicate_inputInvoiceBusinessBotTransferStars || stars.invoice.GetStars() != 25 || stars.invoice.GetBot() == nil || stars.invoice.GetBot().GetPredicateName() != mtproto.Predicate_inputUserSelf {
			t.Fatalf("transfer invoice = %#v, want 25 Stars business transfer", stars.invoice)
		}
	})
}

func TestUpgradeGiftMapsUnsupportedPaidInvoiceTo501(t *testing.T) {
	stars := &giftSettlementStarsFormStub{}
	gifts := &giftSettlementGiftsStub{}
	service := &svc.ServiceContext{
		PaymentsClient:     &giftSettlementPaymentsFormStub{form: nil},
		StarsClient:        stars,
		GiftsClient:        gifts,
		BusinessBotsClient: &giftSettlementBusinessBotsStub{updates: giftSettlementGiftBusinessConnection()},
	}
	// The payment form client returns the same Layer 229 error that the real
	// APIFull adapter returns for inputInvoiceStarGiftUpgrade.
	service.PaymentsClient = &giftSettlementPaymentsUnsupportedStub{}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"business_connection_id": json.RawMessage(`"business-1"`),
		"owned_gift_id":          json.RawMessage(`"gift"`),
		"star_count":             json.RawMessage("25"),
	}}
	result, statusCode, code, description := upgradeGift(service, httptest.NewRequest("POST", "/bot/token/upgradeGift", nil), 42, request)
	if result != nil || statusCode != http.StatusNotImplemented || code != 501 || description != "Not Implemented: paid gift upgrades are not supported by the Layer 229 payment adapter" {
		t.Fatalf("upgradeGift unsupported payment = (%#v, %d, %d, %q), want explicit 501", result, statusCode, code, description)
	}
	if stars.calls != 0 || gifts.upgradeCalls != 0 {
		t.Fatalf("unsupported upgrade settled or mutated gift: stars=%d upgrades=%d", stars.calls, gifts.upgradeCalls)
	}
}

func TestPaymentUnsupportedFromRPCRecognizesDecodedStatus(t *testing.T) {
	err := status.Error(status.Code(mtproto.ErrPaymentUnsupported), "PAYMENT_UNSUPPORTED")
	if !paymentUnsupportedFromRPC(err) {
		t.Fatalf("paymentUnsupportedFromRPC(%v) = false, want true", err)
	}
}

func TestGiftSettlementHTTPValidation(t *testing.T) {
	service := &svc.ServiceContext{
		PaymentsClient: &giftSettlementPaymentsStub{},
		StarsClient:    &giftSettlementStarsStub{},
	}
	request := func(values map[string]json.RawMessage) *botAPIRequest {
		return &botAPIRequest{values: values}
	}
	call := httptest.NewRequest("POST", "/bot/token/sendGift", nil)

	t.Run("gift id is parsed from a string", func(t *testing.T) {
		result, status, code, description := sendGift(service, call, 42, request(map[string]json.RawMessage{
			"gift_id": json.RawMessage(`"101"`),
			"user_id": json.RawMessage("42"),
			"text":    json.RawMessage(`"` + strings.Repeat("界", 129) + `"`),
		}))
		if result != nil || status != 400 || code != 400 || description != "Bad Request: text is too long" {
			t.Fatalf("sendGift validation = (%#v, %d, %d, %q), want text length rejection", result, status, code, description)
		}
	})

	t.Run("premium star count is required", func(t *testing.T) {
		result, status, code, description := giftPremiumSubscription(service, call, 42, request(map[string]json.RawMessage{
			"user_id":     json.RawMessage("42"),
			"month_count": json.RawMessage("3"),
		}))
		if result != nil || status != 400 || code != 400 || description != "Bad Request: star_count is required" {
			t.Fatalf("premium validation = (%#v, %d, %d, %q), want required star_count", result, status, code, description)
		}
	})

	t.Run("premium star count matches months", func(t *testing.T) {
		result, status, code, description := giftPremiumSubscription(service, call, 42, request(map[string]json.RawMessage{
			"user_id":     json.RawMessage("42"),
			"month_count": json.RawMessage("3"),
			"star_count":  json.RawMessage("1500"),
		}))
		if result != nil || status != 400 || code != 400 || description != "Bad Request: star_count does not match month_count" {
			t.Fatalf("premium validation = (%#v, %d, %d, %q), want pricing rejection", result, status, code, description)
		}
	})

	t.Run("premium text length", func(t *testing.T) {
		result, status, code, description := giftPremiumSubscription(service, call, 42, request(map[string]json.RawMessage{
			"user_id":     json.RawMessage("42"),
			"month_count": json.RawMessage("3"),
			"star_count":  json.RawMessage("1000"),
			"text":        json.RawMessage(`"` + strings.Repeat("界", 129) + `"`),
		}))
		if result != nil || status != 400 || code != 400 || description != "Bad Request: text is too long" {
			t.Fatalf("premium text validation = (%#v, %d, %d, %q), want text length rejection", result, status, code, description)
		}
	})
}

func TestSavedGiftHTTPFiltersFailClosedWhenLedgerCannotRepresentThem(t *testing.T) {
	for _, name := range []string{
		"exclude_unlimited",
		"exclude_unique",
		"sort_by_value",
		"sort_by_price",
		"exclude_upgradable",
		"exclude_unupgradable",
		"exclude_limited_upgradable",
		"exclude_limited_non_upgradable",
		"exclude_from_blockchain",
	} {
		t.Run(name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{
				name: json.RawMessage("true"),
			}}
			if _, err := botAPISavedGiftRequest(request); err != errBotAPIGiftFilterUnsupported {
				t.Fatalf("filter error = %v, want %v", err, errBotAPIGiftFilterUnsupported)
			}
		})
	}
}

func TestOwnedGiftIDUsesLayer229SavedGiftReference(t *testing.T) {
	request := func(raw string) *botAPIRequest {
		return &botAPIRequest{values: map[string]json.RawMessage{
			"owned_gift_id": json.RawMessage(raw),
		}}
	}

	numeric, err := botAPIOwnedGift(request(`"12345"`))
	if err != nil || numeric == nil || numeric.GetPredicateName() != mtproto.Predicate_inputSavedStarGiftChat || numeric.GetSavedId() != 12345 || numeric.GetPeer() == nil || numeric.GetPeer().GetPredicateName() != mtproto.Predicate_inputPeerSelf {
		t.Fatalf("numeric owned_gift_id = %#v, %v; want inputSavedStarGiftChat(self, 12345)", numeric, err)
	}

	slug, err := botAPIOwnedGift(request(`"legacy-slug"`))
	if err != nil || slug == nil || slug.GetPredicateName() != mtproto.Predicate_inputSavedStarGiftSlug || slug.GetSlug() != "legacy-slug" {
		t.Fatalf("slug owned_gift_id = %#v, %v; want inputSavedStarGiftSlug", slug, err)
	}
	if _, err = botAPIOwnedGift(request(`"9223372036854775808"`)); err == nil {
		t.Fatal("overflowing numeric owned_gift_id should be rejected")
	}
}
