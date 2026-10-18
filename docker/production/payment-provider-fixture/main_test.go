package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func testFixture() *fixture {
	return &fixture{
		key:      []byte("fixture-test-key"),
		settled:  map[string]settledCacheEntry{},
		requests: map[string]int{},
	}
}

func TestPaymentFixtureReadinessRequiresPostgresControlPlane(t *testing.T) {
	if err := (&controlPlane{}).Ping(context.Background()); err == nil {
		t.Fatal("nil PostgreSQL control plane unexpectedly passed readiness")
	}
	f := testFixture()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	f.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing control plane health status=%d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestResponseEntitlementRequiresSettledTransaction(t *testing.T) {
	form, err := json.Marshal(map[string]any{
		"verified":       true,
		"product_type":   "premium_subscription",
		"premium_months": 1,
		"form":           map[string]any{"form_id": 990001},
	})
	if err != nil {
		t.Fatal(err)
	}
	if product, months, stars := responseEntitlement(form); product != "" || months != 0 || stars != 0 {
		t.Fatalf("form response entitlement = (%q, %d, %d), want empty", product, months, stars)
	}

	settled, err := json.Marshal(map[string]any{
		"verified":       true,
		"product_type":   "premium_subscription",
		"premium_months": 1,
		"transaction_id": "fixture-premium-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if product, months, stars := responseEntitlement(settled); product != "premium_subscription" || months != 1 || stars != 0 {
		t.Fatalf("settled response entitlement = (%q, %d, %d), want premium/1/0", product, months, stars)
	}

	botInvoice, err := json.Marshal(map[string]any{
		"verified":       true,
		"product_type":   "bot_invoice",
		"transaction_id": "fixture-bot-1",
		"receipt":        map[string]any{"provider": "fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if product, months, stars := responseEntitlement(botInvoice); product != "" || months != 0 || stars != 0 {
		t.Fatalf("bot invoice entitlement = (%q, %d, %d), want empty", product, months, stars)
	}
}

func TestProviderTransactionIdentityRejectsCrossUserReuse(t *testing.T) {
	if !providerTransactionIdentityMatches(123, "request-1", 123, "request-1") {
		t.Fatal("same provider transaction identity was rejected")
	}
	for _, tc := range []struct {
		name             string
		storedUserID     int64
		storedRequestKey string
		userID           int64
		requestKey       string
	}{
		{name: "different user", storedUserID: 123, storedRequestKey: "request-1", userID: 456, requestKey: "request-1"},
		{name: "different request", storedUserID: 123, storedRequestKey: "request-1", userID: 123, requestKey: "request-2"},
		{name: "empty stored key", storedUserID: 123, storedRequestKey: "", userID: 123, requestKey: "request-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if providerTransactionIdentityMatches(tc.storedUserID, tc.storedRequestKey, tc.userID, tc.requestKey) {
				t.Fatal("conflicting provider transaction identity was accepted")
			}
		})
	}
}

func TestProviderUserIDUsesCallerForPaidMessages(t *testing.T) {
	fields := testFields(t, map[string]any{
		"caller_user_id": int64(123),
		"user_id":        int64(456),
	})
	if got := providerUserID(fields, "get_paid_messages_revenue"); got != 123 {
		t.Fatalf("paid-message provider user id = %d, want caller 123", got)
	}
	if got := providerUserID(fields, "toggle_no_paid_messages_exception"); got != 123 {
		t.Fatalf("paid-message exception provider user id = %d, want caller 123", got)
	}
	if got := providerUserID(fields, "get_form"); got != 456 {
		t.Fatalf("premium provider user id = %d, want target 456", got)
	}
}

func TestPaymentFixtureRequiresBearerAuthorization(t *testing.T) {
	f := testFixture()
	f.authKey = "provider-key"
	body, err := json.Marshal(map[string]any{
		"request_id": "auth-request",
		"operation":  "payments.canPurchasePremium",
		"user_id":    int64(123),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, authorization := range []string{"", "provider-key", "Bearer wrong", "Bearer  provider-key"} {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Authorization", authorization)
		req.Header.Set("X-Teamgram-Payment-Provider-Request-Signature", f.sign(body))
		recorder := httptest.NewRecorder()
		f.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q status=%d body=%s", authorization, recorder.Code, recorder.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer provider-key")
	req.Header.Set("X-Teamgram-Payment-Provider-Request-Signature", f.sign(body))
	recorder := httptest.NewRecorder()
	f.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid authorization status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestFixturePaidMessageProviderOperations(t *testing.T) {
	f := testFixture()
	revenueFields := testFields(t, map[string]any{
		"operation":      "get_paid_messages_revenue",
		"caller_user_id": int64(123),
		"user_id":        int64(456),
		"parent_peer":    map[string]any{"predicate_name": "inputPeerUser", "user_id": int64(456)},
	})
	revenueBody, revenueHeader, err := f.response(revenueFields)
	if err != nil {
		t.Fatalf("paid-message revenue response: %v", err)
	}
	if revenueHeader != "X-Teamgram-Payment-Signature" {
		t.Fatalf("revenue signature header = %q", revenueHeader)
	}
	assertFixtureSignature(t, f.key, revenueBody, f.sign(revenueBody))
	var revenue struct {
		Verified     bool  `json:"verified"`
		CallerUserID int64 `json:"caller_user_id"`
		UserID       int64 `json:"user_id"`
		StarsAmount  int64 `json:"stars_amount"`
	}
	if err = json.Unmarshal(revenueBody, &revenue); err != nil {
		t.Fatalf("decode paid-message revenue: %v", err)
	}
	if !revenue.Verified || revenue.CallerUserID != 123 || revenue.UserID != 456 || revenue.StarsAmount != 37 {
		t.Fatalf("paid-message revenue = %+v", revenue)
	}

	toggleFields := testFields(t, map[string]any{
		"operation":       "toggle_no_paid_messages_exception",
		"caller_user_id":  int64(123),
		"user_id":         int64(456),
		"refund_charged":  true,
		"require_payment": true,
	})
	toggleBody, toggleHeader, err := f.response(toggleFields)
	if err != nil {
		t.Fatalf("paid-message exception response: %v", err)
	}
	if toggleHeader != "X-Teamgram-Payment-Signature" {
		t.Fatalf("toggle signature header = %q", toggleHeader)
	}
	assertFixtureSignature(t, f.key, toggleBody, f.sign(toggleBody))
	var toggle struct {
		Verified     bool  `json:"verified"`
		CallerUserID int64 `json:"caller_user_id"`
		UserID       int64 `json:"user_id"`
		Result       bool  `json:"result"`
	}
	if err = json.Unmarshal(toggleBody, &toggle); err != nil {
		t.Fatalf("decode paid-message exception: %v", err)
	}
	if !toggle.Verified || toggle.CallerUserID != 123 || toggle.UserID != 456 || !toggle.Result {
		t.Fatalf("paid-message exception = %+v", toggle)
	}
}

func TestFixtureStarsRefundProviderOperation(t *testing.T) {
	f := testFixture()
	fields := testFields(t, map[string]any{
		"operation":      "refund_stars_charge",
		"caller_user_id": int64(123),
		"user_id":        int64(123),
		"charge_id":      "fixture-stars-charge",
		"provider":       "local-fixture",
		"transaction_id": "fixture-stars-charge",
		"stars":          int64(25),
	})
	body, header, err := f.response(fields)
	if err != nil {
		t.Fatalf("Stars refund response: %v", err)
	}
	if header != "X-Teamgram-Payment-Signature" {
		t.Fatalf("Stars refund signature header = %q", header)
	}
	assertFixtureSignature(t, f.key, body, f.sign(body))
	var result struct {
		Verified            bool            `json:"verified"`
		CallerUserID        int64           `json:"caller_user_id"`
		UserID              int64           `json:"user_id"`
		ChargeID            string          `json:"charge_id"`
		Provider            string          `json:"provider"`
		Stars               int64           `json:"stars"`
		RefundTransactionID string          `json:"refund_transaction_id"`
		Receipt             json.RawMessage `json:"receipt"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode Stars refund response: %v", err)
	}
	if !result.Verified || result.CallerUserID != 123 || result.UserID != 123 || result.ChargeID != "fixture-stars-charge" ||
		result.Provider != "local-fixture" || result.Stars != 25 || result.RefundTransactionID == "" || len(result.Receipt) == 0 {
		t.Fatalf("Stars refund result = %+v", result)
	}
}

func testFields(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func testStarsOffer() map[string]any {
	return map[string]any{
		"ID":           int64(17),
		"Kind":         "topup",
		"Stars":        int64(100),
		"StoreProduct": "stars-100",
		"Currency":     "USD",
		"Amount":       int64(199),
		"Extended":     false,
	}
}

func assertFixtureSignature(t *testing.T, key []byte, body []byte, signature string) {
	t.Helper()
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if signature != want {
		t.Fatalf("signature = %q, want %q", signature, want)
	}
}

func TestFixtureStarsFormIsSignedAndLayer229Encodable(t *testing.T) {
	f := testFixture()
	fields := testFields(t, map[string]any{
		"operation":           "get_stars_form",
		"user_id":             int64(123),
		"beneficiary_user_id": int64(123),
		"required_product":    "stars_topup",
		"offer_id":            int64(17),
		"offer":               testStarsOffer(),
	})
	body, header, err := f.response(fields)
	if err != nil {
		t.Fatalf("Stars form response: %v", err)
	}
	if header != "X-Teamgram-Payment-Signature" {
		t.Fatalf("signature header = %q", header)
	}
	assertFixtureSignature(t, f.key, body, f.sign(body))
	var result struct {
		Verified          bool                          `json:"verified"`
		UserID            int64                         `json:"user_id"`
		BeneficiaryUserID int64                         `json:"beneficiary_user_id"`
		OfferID           int64                         `json:"offer_id"`
		Stars             int64                         `json:"stars"`
		StoreProduct      string                        `json:"store_product"`
		Currency          string                        `json:"currency"`
		Amount            int64                         `json:"amount"`
		Form              *mtproto.Payments_PaymentForm `json:"form"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode Stars form response: %v", err)
	}
	if !result.Verified || result.UserID != 123 || result.BeneficiaryUserID != 123 || result.OfferID != 17 || result.Stars != 100 || result.StoreProduct != "stars-100" || result.Currency != "USD" || result.Amount != 199 {
		t.Fatalf("Stars form metadata = %+v", result)
	}
	if result.Form == nil || result.Form.GetPredicateName() != mtproto.Predicate_payments_paymentFormStars || result.Form.GetFormId() <= 0 || result.Form.GetInvoice() == nil || len(result.Form.GetInvoice().GetPrices()) != 1 || result.Form.GetInvoice().GetPrices()[0].GetAmount() != 199 {
		t.Fatalf("Stars form = %+v", result.Form)
	}
	if err = result.Form.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("Stars form does not encode at Layer 229: %v", err)
	}
}

func TestFixtureBotInvoiceFormAndValidation(t *testing.T) {
	f := testFixture()
	invoice := map[string]any{
		"predicate_name": "invoice",
		"test":           true,
		"currency":       "USD",
		"prices": []any{
			map[string]any{"predicate_name": "labeledPrice", "label": "Item", "amount": 125},
		},
	}
	formFields := testFields(t, map[string]any{
		"operation":           "get_bot_invoice_form",
		"user_id":             int64(123),
		"beneficiary_user_id": int64(456),
		"bot_user_id":         int64(456),
		"required_product":    "bot_invoice",
		"title":               "Item",
		"description":         "A paid item",
		"invoice":             invoice,
	})
	body, header, err := f.response(formFields)
	if err != nil {
		t.Fatalf("bot invoice form response: %v", err)
	}
	assertFixtureSignature(t, f.key, body, f.sign(body))
	var formResult struct {
		Verified          bool                          `json:"verified"`
		UserID            int64                         `json:"user_id"`
		BeneficiaryUserID int64                         `json:"beneficiary_user_id"`
		BotUserID         int64                         `json:"bot_user_id"`
		ProductType       string                        `json:"product_type"`
		Form              *mtproto.Payments_PaymentForm `json:"form"`
	}
	if err = json.Unmarshal(body, &formResult); err != nil {
		t.Fatalf("decode bot invoice form: %v", err)
	}
	if !formResult.Verified || formResult.UserID != 123 || formResult.BeneficiaryUserID != 456 || formResult.BotUserID != 456 || formResult.ProductType != "bot_invoice" || formResult.Form == nil || formResult.Form.GetPredicateName() != mtproto.Predicate_payments_paymentForm || formResult.Form.GetBotId() != 456 || formResult.Form.GetTitle() != "Item" || formResult.Form.GetDescription() != "A paid item" || formResult.Form.GetInvoice() == nil || len(formResult.Form.GetInvoice().GetPrices()) != 1 || formResult.Form.GetInvoice().GetPrices()[0].GetAmount() != 125 {
		t.Fatalf("bot invoice form = %+v", formResult)
	}
	if err = formResult.Form.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("bot invoice form does not encode at Layer 229: %v", err)
	}

	validationFields := testFields(t, map[string]any{
		"operation":           "validate_bot_invoice_info",
		"user_id":             int64(123),
		"beneficiary_user_id": int64(456),
		"bot_user_id":         int64(456),
		"required_product":    "bot_invoice",
	})
	body, header, err = f.response(validationFields)
	if err != nil {
		t.Fatalf("bot invoice validation response: %v", err)
	}
	assertFixtureSignature(t, f.key, body, f.sign(body))
	var validationResult struct {
		Verified          bool              `json:"verified"`
		UserID            int64             `json:"user_id"`
		BeneficiaryUserID int64             `json:"beneficiary_user_id"`
		BotUserID         int64             `json:"bot_user_id"`
		ProductType       string            `json:"product_type"`
		ID                string            `json:"id"`
		ShippingOptions   []json.RawMessage `json:"shipping_options"`
	}
	if err = json.Unmarshal(body, &validationResult); err != nil {
		t.Fatalf("decode bot invoice validation: %v", err)
	}
	if !validationResult.Verified || validationResult.UserID != 123 || validationResult.BeneficiaryUserID != 456 || validationResult.BotUserID != 456 || validationResult.ProductType != "bot_invoice" || validationResult.ID == "" || len(validationResult.ShippingOptions) != 0 {
		t.Fatalf("bot invoice validation = %+v", validationResult)
	}
	_ = header
}

func TestFixtureExportInvoiceReturnsSignedHTTPSURL(t *testing.T) {
	f := testFixture()
	fields := testFields(t, map[string]any{
		"operation": "export_invoice",
		"user_id":   int64(123),
		"invoice_media": map[string]any{
			"predicate_name": "inputMediaInvoice",
			"title":          "Acceptance invoice",
		},
	})
	body, header, err := f.response(fields)
	if err != nil {
		t.Fatalf("export invoice response: %v", err)
	}
	if header != "X-Teamgram-Payment-Signature" {
		t.Fatalf("signature header = %q", header)
	}
	assertFixtureSignature(t, f.key, body, f.sign(body))
	var result struct {
		Verified bool   `json:"verified"`
		UserID   int64  `json:"user_id"`
		URL      string `json:"url"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode export invoice response: %v", err)
	}
	if !result.Verified || result.UserID != 123 || result.URL == "" || !strings.HasPrefix(result.URL, "https://") {
		t.Fatalf("export invoice response = %+v", result)
	}
}

func TestFixtureStarGiftFormAndSettlementAreSignedAndIdempotent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		product    string
		targetID   int64
		requestKey string
	}{
		{name: "upgrade", product: "star_gift_upgrade", requestKey: "gift:upgrade:request"},
		{name: "transfer", product: "star_gift_transfer", targetID: 808, requestKey: "gift:transfer:request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := testFixture()
			invoice := map[string]any{
				"predicate_name": "inputInvoiceStarGiftUpgrade",
				"stargift":       map[string]any{"predicate_name": "inputSavedStarGiftSlug", "slug": "gift-77"},
			}
			fields := testFields(t, map[string]any{
				"operation":           "get_gift_form",
				"user_id":             int64(123),
				"beneficiary_user_id": int64(456),
				"required_product":    tc.product,
				"gift_id":             int64(77),
				"gift_slug":           "gift-77",
				"target_id":           tc.targetID,
				"stars":               int64(250),
				"invoice":             invoice,
			})
			body, header, err := f.response(fields)
			if err != nil {
				t.Fatalf("gift form response: %v", err)
			}
			if header != "X-Teamgram-Payment-Signature" {
				t.Fatalf("gift form signature header = %q", header)
			}
			assertFixtureSignature(t, f.key, body, f.sign(body))
			var formResult struct {
				Verified          bool                          `json:"verified"`
				UserID            int64                         `json:"user_id"`
				BeneficiaryUserID int64                         `json:"beneficiary_user_id"`
				ProductType       string                        `json:"product_type"`
				Stars             int64                         `json:"stars"`
				Currency          string                        `json:"currency"`
				Amount            int64                         `json:"amount"`
				Form              *mtproto.Payments_PaymentForm `json:"form"`
			}
			if err = json.Unmarshal(body, &formResult); err != nil {
				t.Fatalf("decode gift form response: %v", err)
			}
			if !formResult.Verified || formResult.UserID != 123 || formResult.BeneficiaryUserID != 456 || formResult.ProductType != tc.product || formResult.Stars != 250 || formResult.Currency != "XTR" || formResult.Amount != 250 || formResult.Form == nil || formResult.Form.GetPredicateName() != mtproto.Predicate_payments_paymentFormStars || formResult.Form.GetFormId() <= 0 || formResult.Form.GetInvoice() == nil || len(formResult.Form.GetInvoice().GetPrices()) != 1 || formResult.Form.GetInvoice().GetPrices()[0].GetAmount() != 250 {
				t.Fatalf("gift form = %+v", formResult)
			}
			if err = formResult.Form.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
				t.Fatalf("gift form does not encode at Layer 229: %v", err)
			}

			settleFields := testFields(t, map[string]any{
				"operation":           "settle_gift",
				"user_id":             int64(123),
				"beneficiary_user_id": int64(456),
				"required_product":    tc.product,
				"request_key":         tc.requestKey,
				"fingerprint":         "gift-fingerprint",
				"form_id":             formResult.Form.GetFormId(),
				"gift_id":             int64(77),
				"gift_slug":           "gift-77",
				"target_id":           tc.targetID,
				"stars":               int64(250),
				"invoice":             invoice,
			})
			settled, settledHeader, err := f.response(settleFields)
			if err != nil {
				t.Fatalf("gift settlement: %v", err)
			}
			replay, replayHeader, err := f.response(settleFields)
			if err != nil {
				t.Fatalf("replayed gift settlement: %v", err)
			}
			if !bytes.Equal(settled, replay) || settledHeader != replayHeader {
				t.Fatalf("gift settlement replay changed response or signature header")
			}
			assertFixtureSignature(t, f.key, settled, f.sign(settled))
			var settlementResult struct {
				Verified          bool            `json:"verified"`
				UserID            int64           `json:"user_id"`
				BeneficiaryUserID int64           `json:"beneficiary_user_id"`
				RequestKey        string          `json:"request_key"`
				Fingerprint       string          `json:"fingerprint"`
				FormID            int64           `json:"form_id"`
				ProductType       string          `json:"product_type"`
				Stars             int64           `json:"stars"`
				Currency          string          `json:"currency"`
				Amount            int64           `json:"amount"`
				TransactionID     string          `json:"transaction_id"`
				Receipt           json.RawMessage `json:"receipt"`
			}
			if err = json.Unmarshal(settled, &settlementResult); err != nil {
				t.Fatalf("decode gift settlement response: %v", err)
			}
			if !settlementResult.Verified || settlementResult.UserID != 123 || settlementResult.BeneficiaryUserID != 456 || settlementResult.RequestKey != tc.requestKey || settlementResult.Fingerprint != "gift-fingerprint" || settlementResult.FormID != formResult.Form.GetFormId() || settlementResult.ProductType != tc.product || settlementResult.Stars != 250 || settlementResult.Currency != "XTR" || settlementResult.Amount != 250 || settlementResult.TransactionID == "" || len(settlementResult.Receipt) == 0 || bytes.Equal(bytes.TrimSpace(settlementResult.Receipt), []byte("null")) {
				t.Fatalf("gift settlement = %+v", settlementResult)
			}
		})
	}
}

func TestFixtureBotInvoiceSettlementIsIdempotent(t *testing.T) {
	f := testFixture()
	fields := testFields(t, map[string]any{
		"operation":           "settle_bot_invoice",
		"user_id":             int64(123),
		"beneficiary_user_id": int64(456),
		"bot_user_id":         int64(456),
		"required_product":    "bot_invoice",
		"request_key":         "bot:123:456:payment",
		"fingerprint":         "bot-fingerprint",
		"form_id":             int64(990003),
		"currency":            "USD",
		"amount":              int64(125),
	})
	body, header, err := f.response(fields)
	if err != nil {
		t.Fatalf("bot invoice settlement: %v", err)
	}
	replay, _, err := f.response(fields)
	if err != nil {
		t.Fatalf("replayed bot invoice settlement: %v", err)
	}
	if !bytes.Equal(body, replay) {
		t.Fatalf("bot invoice settlement replay changed response")
	}
	assertFixtureSignature(t, f.key, body, f.sign(body))
	if header != "X-Teamgram-Payment-Signature" {
		t.Fatalf("signature header = %q", header)
	}
	var result struct {
		Verified          bool            `json:"verified"`
		UserID            int64           `json:"user_id"`
		BeneficiaryUserID int64           `json:"beneficiary_user_id"`
		BotUserID         int64           `json:"bot_user_id"`
		RequestKey        string          `json:"request_key"`
		Fingerprint       string          `json:"fingerprint"`
		FormID            int64           `json:"form_id"`
		ProductType       string          `json:"product_type"`
		Currency          string          `json:"currency"`
		Amount            int64           `json:"amount"`
		TransactionID     string          `json:"transaction_id"`
		Receipt           json.RawMessage `json:"receipt"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode bot invoice settlement: %v", err)
	}
	if !result.Verified || result.UserID != 123 || result.BeneficiaryUserID != 456 || result.BotUserID != 456 || result.RequestKey != "bot:123:456:payment" || result.Fingerprint != "bot-fingerprint" || result.FormID != 990003 || result.ProductType != "bot_invoice" || result.Currency != "USD" || result.Amount != 125 || result.TransactionID == "" || len(result.Receipt) == 0 {
		t.Fatalf("bot invoice settlement = %+v", result)
	}
}

func TestFixtureStarsSettlementIsIdempotent(t *testing.T) {
	f := testFixture()
	fields := testFields(t, map[string]any{
		"operation":           "settle_stars",
		"user_id":             int64(123),
		"beneficiary_user_id": int64(123),
		"required_product":    "stars_topup",
		"request_key":         "stars:123:990019:fingerprint",
		"fingerprint":         "fingerprint",
		"form_id":             int64(990019),
		"offer_id":            int64(17),
		"offer":               testStarsOffer(),
	})
	body, header, err := f.response(fields)
	if err != nil {
		t.Fatalf("first Stars settlement: %v", err)
	}
	replay, replayHeader, err := f.response(fields)
	if err != nil {
		t.Fatalf("replayed Stars settlement: %v", err)
	}
	if !bytes.Equal(body, replay) || header != replayHeader {
		t.Fatalf("replay changed response or signature header")
	}
	assertFixtureSignature(t, f.key, body, f.sign(body))
	var result struct {
		Verified          bool            `json:"verified"`
		UserID            int64           `json:"user_id"`
		BeneficiaryUserID int64           `json:"beneficiary_user_id"`
		RequestKey        string          `json:"request_key"`
		Fingerprint       string          `json:"fingerprint"`
		FormID            int64           `json:"form_id"`
		ProductType       string          `json:"product_type"`
		OfferID           int64           `json:"offer_id"`
		Stars             int64           `json:"stars"`
		TransactionID     string          `json:"transaction_id"`
		Receipt           json.RawMessage `json:"receipt"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode Stars settlement response: %v", err)
	}
	if !result.Verified || result.UserID != 123 || result.BeneficiaryUserID != 123 || result.RequestKey == "" || result.RequestKey != "stars:123:990019:fingerprint" || result.Fingerprint != "fingerprint" || result.FormID != 990019 || result.ProductType != "stars_topup" || result.OfferID != 17 || result.Stars != 100 || result.TransactionID == "" || len(result.Receipt) == 0 || bytes.Equal(bytes.TrimSpace(result.Receipt), []byte("null")) {
		t.Fatalf("Stars settlement = %+v", result)
	}
}

func TestFixtureSettlementCacheSeparatesOperations(t *testing.T) {
	f := testFixture()
	premium := testFields(t, map[string]any{
		"user_id":          int64(123),
		"required_product": "premium_subscription",
		"request_key":      "shared-request-key",
		"fingerprint":      "premium-fingerprint",
		"form_id":          int64(990001),
	})
	stars := testFields(t, map[string]any{
		"operation":           "settle_stars",
		"user_id":             int64(123),
		"beneficiary_user_id": int64(123),
		"required_product":    "stars_topup",
		"request_key":         "shared-request-key",
		"fingerprint":         "stars-fingerprint",
		"form_id":             int64(990019),
		"offer":               testStarsOffer(),
	})
	premiumBody, _, err := f.response(premium)
	if err != nil {
		t.Fatalf("premium settlement: %v", err)
	}
	starsBody, _, err := f.response(stars)
	if err != nil {
		t.Fatalf("Stars settlement: %v", err)
	}
	if bytes.Equal(premiumBody, starsBody) {
		t.Fatalf("settlement cache mixed different operations")
	}
	var premiumResult, starsResult struct {
		ProductType string `json:"product_type"`
	}
	if err = json.Unmarshal(premiumBody, &premiumResult); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(starsBody, &starsResult); err != nil {
		t.Fatal(err)
	}
	if premiumResult.ProductType != "premium_subscription" || starsResult.ProductType != "stars_topup" {
		t.Fatalf("product types = %q and %q", premiumResult.ProductType, starsResult.ProductType)
	}
}

func TestFixtureSettlementCacheSeparatesUsers(t *testing.T) {
	f := testFixture()
	base := map[string]any{
		"required_product": "premium_subscription",
		"request_key":      "shared-request-key",
		"fingerprint":      "premium-fingerprint",
		"form_id":          int64(990001),
	}
	firstFields := testFields(t, map[string]any{
		"user_id":          int64(123),
		"required_product": base["required_product"],
		"request_key":      base["request_key"],
		"fingerprint":      base["fingerprint"],
		"form_id":          base["form_id"],
	})
	secondFields := testFields(t, map[string]any{
		"user_id":          int64(456),
		"required_product": base["required_product"],
		"request_key":      base["request_key"],
		"fingerprint":      base["fingerprint"],
		"form_id":          base["form_id"],
	})
	firstBody, _, err := f.response(firstFields)
	if err != nil {
		t.Fatalf("first user settlement: %v", err)
	}
	secondBody, _, err := f.response(secondFields)
	if err != nil {
		t.Fatalf("second user settlement: %v", err)
	}
	if bytes.Equal(firstBody, secondBody) {
		t.Fatal("settlement cache reused a different user's response")
	}
	var first, second struct {
		UserID        int64  `json:"user_id"`
		TransactionID string `json:"transaction_id"`
	}
	if err = json.Unmarshal(firstBody, &first); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(secondBody, &second); err != nil {
		t.Fatal(err)
	}
	if first.UserID != 123 || second.UserID != 456 || first.TransactionID == "" || second.TransactionID == "" || first.TransactionID == second.TransactionID {
		t.Fatalf("cross-user settlement identity = (%+v, %+v)", first, second)
	}
}

func TestFixtureSettlementCacheRejectsPayloadConflict(t *testing.T) {
	f := testFixture()
	fields := testFields(t, map[string]any{
		"required_product": "premium_subscription",
		"user_id":          int64(123),
		"request_key":      "same-request-key",
		"fingerprint":      "first-fingerprint",
		"form_id":          int64(990001),
	})
	if _, _, err := f.response(fields); err != nil {
		t.Fatalf("first Premium settlement: %v", err)
	}
	fields["fingerprint"] = json.RawMessage(`"different-fingerprint"`)
	if _, _, err := f.response(fields); err == nil {
		t.Fatal("settlement cache reused a response for a different payload")
	}
}

func TestFixtureRejectsUnsupportedSettlementOperation(t *testing.T) {
	f := testFixture()
	fields := testFields(t, map[string]any{
		"operation":   "apply_gift_code",
		"user_id":     int64(123),
		"request_key": "gift-code:123:unsupported",
		"fingerprint": "fingerprint",
		"form_id":     int64(1),
	})
	if _, _, err := f.response(fields); err == nil {
		t.Fatalf("unsupported operation unexpectedly succeeded")
	}
}

func TestFixtureServeHTTPSignsStarsSettlement(t *testing.T) {
	f := testFixture()
	request := testFields(t, map[string]any{
		"operation":           "settle_stars",
		"user_id":             int64(123),
		"beneficiary_user_id": int64(123),
		"required_product":    "stars_topup",
		"request_key":         "stars:123:990019:serve",
		"fingerprint":         "serve",
		"form_id":             int64(990019),
		"offer":               testStarsOffer(),
	})
	requestBody, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", bytes.NewReader(requestBody))
	w := httptest.NewRecorder()
	f.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	assertFixtureSignature(t, f.key, w.Body.Bytes(), w.Header().Get("X-Teamgram-Payment-Signature"))
}

func TestFixtureRejectsBusinessProviderRequests(t *testing.T) {
	f := testFixture()
	body, err := json.Marshal(map[string]any{
		"request_id": "business-request",
		"operation":  "payments.getResaleStarGifts",
		"user_id":    int64(123),
		"payload":    map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/", bytes.NewReader(body))
	req.Header.Set("X-Teamgram-Business-Provider-Request-Signature", "sha256=fixture")
	rec := httptest.NewRecorder()
	f.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("business request status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFixturePremiumRetryHitsPostgresControlPlane(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEAMGRAM_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	store, err := openControlPlane(dsn)
	if err != nil {
		t.Skipf("PostgreSQL control-plane schema unavailable: %v", err)
	}
	defer func() { _ = store.close() }()

	requestKey := "premium-retry-control-plane-" + time.Now().UTC().Format("20060102150405.000000000")
	const userID = int64(90218)
	// Keep the integration row isolated and remove the entitlement left by the
	// fixture's authoritative save transaction after the assertion completes.
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_entitlement WHERE user_id=$1 AND transaction_id LIKE 'fixture-premium-%'`, userID)
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_operation WHERE user_id=$1 AND request_key=$2`, userID, "premium_subscription:"+requestKey)
	})

	fixture := &fixture{
		key:      []byte("fixture-test-key"),
		settled:  map[string]settledCacheEntry{},
		requests: map[string]int{},
		store:    store,
	}
	request := map[string]any{
		// Legacy Premium callers omit operation. ServeHTTP must normalize it
		// before loading and saving the PostgreSQL control-plane row.
		"user_id":          userID,
		"required_product": "premium_subscription",
		"request_key":      requestKey,
		"fingerprint":      "premium-control-plane-fingerprint",
		"form_id":          int64(990001),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	firstRequest := httptest.NewRequest("POST", "/", bytes.NewReader(body))
	first := httptest.NewRecorder()
	fixture.ServeHTTP(first, firstRequest)
	if first.Code != 200 {
		t.Fatalf("first Premium settlement status = %d, body = %s", first.Code, first.Body.String())
	}
	secondRequest := httptest.NewRequest("POST", "/", bytes.NewReader(body))
	second := httptest.NewRecorder()
	fixture.ServeHTTP(second, secondRequest)
	if second.Code != 200 {
		t.Fatalf("Premium retry status = %d, body = %s", second.Code, second.Body.String())
	}
	var firstResult, secondResult map[string]any
	if err = json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(second.Body.Bytes(), &secondResult); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstResult, secondResult) {
		t.Fatalf("Premium retry changed response: first=%s second=%s", first.Body.String(), second.Body.String())
	}
	if got := fixture.requests["premium_subscription"]; got != 2 {
		t.Fatalf("normalized operation request count = %d, want 2", got)
	}

	var operationCount int
	if err = store.db.QueryRow(`SELECT count(*) FROM apifull_payment_provider_operation WHERE operation=$1 AND user_id=$2 AND request_key=$3`, "premium_subscription", userID, "premium_subscription:"+requestKey).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 {
		t.Fatalf("Premium control-plane rows = %d, want 1", operationCount)
	}
}

func TestFixtureBotInvoiceRetrySurvivesProviderRestart(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEAMGRAM_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	store, err := openControlPlane(dsn)
	if err != nil {
		t.Skipf("PostgreSQL control-plane schema unavailable: %v", err)
	}
	defer func() { _ = store.close() }()

	requestKey := "bot-restart-control-plane-" + time.Now().UTC().Format("20060102150405.000000000")
	const userID = int64(90219)
	const botID = int64(90220)
	operationKey := "settle_bot_invoice:" + requestKey
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_entitlement WHERE user_id=$1 AND transaction_id LIKE 'fixture-bot-%'`, userID)
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_receipt WHERE user_id=$1 AND request_key=$2`, userID, operationKey)
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_operation WHERE user_id=$1 AND request_key=$2`, userID, operationKey)
	})

	request := map[string]any{
		"operation":           "settle_bot_invoice",
		"user_id":             userID,
		"beneficiary_user_id": botID,
		"bot_user_id":         botID,
		"required_product":    "bot_invoice",
		"request_key":         requestKey,
		"fingerprint":         "bot-restart-fingerprint",
		"form_id":             int64(990003),
		"currency":            "USD",
		"amount":              int64(125),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	firstFixture := &fixture{
		key:      []byte("fixture-test-key"),
		settled:  map[string]settledCacheEntry{},
		requests: map[string]int{},
		store:    store,
	}
	first := httptest.NewRecorder()
	firstFixture.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if first.Code != http.StatusOK {
		t.Fatalf("first bot settlement status = %d, body = %s", first.Code, first.Body.String())
	}
	firstBody := append([]byte(nil), first.Body.Bytes()...)
	firstSignature := first.Header().Get("X-Teamgram-Payment-Signature")
	if firstSignature == "" {
		t.Fatal("first bot settlement did not include a signature")
	}

	// Reopen the control plane and create a fresh fixture to model a provider
	// process restart. The second instance has no in-memory settlement cache.
	if err = store.close(); err != nil {
		t.Fatal(err)
	}
	store, err = openControlPlane(dsn)
	if err != nil {
		t.Fatal(err)
	}
	secondFixture := &fixture{
		key:      []byte("fixture-test-key"),
		settled:  map[string]settledCacheEntry{},
		requests: map[string]int{},
		store:    store,
	}
	second := httptest.NewRecorder()
	secondFixture.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if second.Code != http.StatusOK || !bytes.Equal(firstBody, second.Body.Bytes()) || second.Header().Get("X-Teamgram-Payment-Signature") != firstSignature {
		t.Fatalf("bot replay after provider restart changed response: first=%s second=%s", firstBody, second.Body.Bytes())
	}

	var operationCount, receiptCount, entitlementCount int
	if err = store.db.QueryRow(`SELECT count(*) FROM apifull_payment_provider_operation WHERE operation='settle_bot_invoice' AND user_id=$1 AND request_key=$2`, userID, operationKey).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if err = store.db.QueryRow(`SELECT count(*) FROM apifull_payment_provider_receipt WHERE operation='settle_bot_invoice' AND user_id=$1 AND request_key=$2`, userID, operationKey).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if err = store.db.QueryRow(`SELECT count(*) FROM apifull_payment_provider_entitlement WHERE user_id=$1 AND product='bot_invoice'`, userID).Scan(&entitlementCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 || receiptCount != 1 || entitlementCount != 0 {
		t.Fatalf("bot control-plane rows = operation %d receipt %d entitlement %d", operationCount, receiptCount, entitlementCount)
	}

	conflicting := map[string]any{}
	if err = json.Unmarshal(body, &conflicting); err != nil {
		t.Fatal(err)
	}
	conflicting["amount"] = int64(126)
	conflictBody, err := json.Marshal(conflicting)
	if err != nil {
		t.Fatal(err)
	}
	conflict := httptest.NewRecorder()
	secondFixture.ServeHTTP(conflict, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(conflictBody)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflicting bot replay status = %d, body = %s", conflict.Code, conflict.Body.String())
	}
}
