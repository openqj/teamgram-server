package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const paymentProviderTestSigningKey = "provider-signing-test-key"

func premiumSelfSubscriptionInvoiceForTest(slug string) *mtproto.InputInvoice {
	purpose := mtproto.MakeTLInputStorePaymentPremiumSubscription(&mtproto.InputStorePaymentPurpose{}).To_InputStorePaymentPurpose()
	return mtproto.MakeTLInputInvoiceSlug(&mtproto.InputInvoice{Slug: slug, Purpose: purpose}).To_InputInvoice()
}

func writeSignedPaymentProviderResponse(t *testing.T, w http.ResponseWriter, response any) {
	t.Helper()
	body, err := json.Marshal(response)
	if err != nil {
		t.Errorf("marshal payment provider response: %v", err)
		http.Error(w, "invalid fixture", http.StatusInternalServerError)
		return
	}
	mac := hmac.New(sha256.New, []byte(paymentProviderTestSigningKey))
	_, _ = mac.Write(body)
	w.Header().Set("X-Teamgram-Payment-Signature", hex.EncodeToString(mac.Sum(nil)))
	if _, err = w.Write(body); err != nil {
		t.Errorf("write payment provider response: %v", err)
	}
}

func TestPaymentsUnauthed(t *testing.T) {
	handlers := []func(*ApiFullCore) error{
		func(c *ApiFullCore) error { _, err := c.AccountGetTmpPassword(nil); return err },
		func(c *ApiFullCore) error { _, err := c.MessagesSetBotShippingResults(nil); return err },
		func(c *ApiFullCore) error { _, err := c.MessagesSetBotPrecheckoutResults(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetPaymentForm(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetPaymentReceipt(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsValidateRequestedInfo(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsSendPaymentForm(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetSavedInfo(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsClearSavedInfo(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetBankCardData(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsExportInvoice(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsRequestRecurringPayment(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsRestorePlayMarketReceipt(nil); return err },
		func(c *ApiFullCore) error { _, err := c.MessagesGetExtendedMedia(nil); return err },
		func(c *ApiFullCore) error { _, err := c.AccountGetPaidMessagesRevenue(nil); return err },
		func(c *ApiFullCore) error { _, err := c.AccountToggleNoPaidMessagesException(nil); return err },
		func(c *ApiFullCore) error { _, err := c.ChannelsUpdatePaidMessagesPrice(nil); return err },
		func(c *ApiFullCore) error { _, err := c.AccountAddNoPaidMessagesException(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsTopupOptions(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsStatus(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsTransactions(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsSendStarsForm(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsRefundStarsCharge(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsRevenueStats(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsRevenueWithdrawalUrl(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsRevenueAdsAccountUrl(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsTransactionsByID(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsGiftOptions(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsGetStarsSubscriptions(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsChangeStarsSubscription(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsFulfillStarsSubscription(nil); return err },
		func(c *ApiFullCore) error { _, err := c.PaymentsBotCancelStarsSubscription(nil); return err },
	}
	cores := []*ApiFullCore{
		{},
		{MD: &metadata.RpcMetadata{UserId: 0}},
	}
	for _, c := range cores {
		for i, h := range handlers {
			if err := h(c); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("handler %d: got %v", i, err)
			}
		}
	}
}

func TestAccountGetTmpPasswordFailsClosed(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 902001}}
	result, err := c.AccountGetTmpPassword(&mtproto.TLAccountGetTmpPassword{
		Password: &mtproto.InputCheckPasswordSRP{SrpId: 1, A: []byte{1}, M1: []byte{1}},
		Period:   60,
	})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AccountGetTmpPassword() = (%#v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
}

type paymentPersistenceSpy struct {
	gets   int
	sets   int
	values map[string]string
}

func (s *paymentPersistenceSpy) Get(key string) (string, error) {
	s.gets++
	return s.values[key], nil
}

func (s *paymentPersistenceSpy) Set(key, value string) error {
	s.sets++
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func TestPaymentInfoMethodsUsePersistence(t *testing.T) {
	previous := persist.Default
	uid := int64(902002)
	spy := &paymentPersistenceSpy{values: map[string]string{
		payInfoKey(uid): `{"name":"Alice","email":"alice@example.com"}`,
		payCredKey(uid): "1",
	}}
	persist.Use(spy)
	defer persist.Use(previous)

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	result, err := c.PaymentsGetSavedInfo(&mtproto.TLPaymentsGetSavedInfo{})
	if err != nil || result == nil || !result.GetHasSavedCredentials() || result.GetSavedInfo() == nil || result.GetSavedInfo().GetEmail().GetValue() != "alice@example.com" {
		t.Fatalf("get saved info: result=%+v err=%v", result, err)
	}
	cleared, err := c.PaymentsClearSavedInfo(&mtproto.TLPaymentsClearSavedInfo{Info: true, Credentials: true})
	if err != nil || cleared != mtproto.BoolTrue || spy.values[payInfoKey(uid)] != "" || spy.values[payCredKey(uid)] != "0" {
		t.Fatalf("clear saved info: result=%v err=%v values=%v", cleared, err, spy.values)
	}
}

func TestPaymentInfoMethodsValidateMalformedRequests(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 902003}}
	if result, err := c.PaymentsValidateRequestedInfo(nil); result != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("nil validation request = (%#v, %v), want nil and INPUT_CONSTRUCTOR_INVALID", result, err)
	}
	if result, err := c.PaymentsValidateRequestedInfo(&mtproto.TLPaymentsValidateRequestedInfo{}); result != nil || !errors.Is(err, mtproto.ErrInvoicePayloadInvalid) {
		t.Fatalf("validation request without invoice = (%#v, %v), want nil and INVOICE_PAYLOAD_INVALID", result, err)
	}
	if result, err := c.PaymentsGetSavedInfo(nil); result != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("nil saved-info request = (%#v, %v), want nil and INPUT_CONSTRUCTOR_INVALID", result, err)
	}
	if result, err := c.PaymentsClearSavedInfo(nil); result != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("nil clear-info request = (%#v, %v), want nil and INPUT_CONSTRUCTOR_INVALID", result, err)
	}
}

func TestPaymentsGetPaymentReceiptFailsClosedBeforeEncoding(t *testing.T) {
	uid := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	self := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	valid := &mtproto.TLPaymentsGetPaymentReceipt{
		Constructor: mtproto.TLConstructor_CRC32_payments_getPaymentReceipt,
		Peer:        self,
		MsgId:       1,
	}
	if err := valid.Encode(mtproto.NewEncodeBuf(128), 229); err != nil {
		t.Fatalf("valid request did not encode: %v", err)
	}

	tests := []struct {
		name string
		in   *mtproto.TLPaymentsGetPaymentReceipt
		want error
	}{
		{"nil request", nil, mtproto.ErrInputConstructorInvalid},
		{"missing peer", &mtproto.TLPaymentsGetPaymentReceipt{MsgId: 1}, mtproto.ErrPeerIdInvalid},
		{"invalid peer", &mtproto.TLPaymentsGetPaymentReceipt{Peer: &mtproto.InputPeer{}, MsgId: 1}, mtproto.ErrPeerIdInvalid},
		{"missing message", &mtproto.TLPaymentsGetPaymentReceipt{Peer: self}, mtproto.ErrMsgIdInvalid},
		{"unavailable receipt", valid, mtproto.ErrPaymentUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := c.PaymentsGetPaymentReceipt(tc.in)
			if result != nil || !errors.Is(err, tc.want) {
				t.Fatalf("PaymentsGetPaymentReceipt() = (%#v, %v), want (nil, %v)", result, err, tc.want)
			}
		})
	}
}

func TestPaymentsProviderUnavailableIsRejected(t *testing.T) {
	uid := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if _, err := c.PaymentsSendPaymentForm(nil); err != mtproto.ErrInvoicePayloadInvalid {
		t.Fatalf("empty payment request: got %v", err)
	}
	invoice := &mtproto.InputInvoice{Purpose: &mtproto.InputStorePaymentPurpose{Currency: "USD"}}
	if _, err := c.PaymentsSendPaymentForm(&mtproto.TLPaymentsSendPaymentForm{Invoice: invoice}); err != mtproto.ErrPaymentProviderInvalid {
		t.Fatalf("missing credentials: got %v", err)
	}
	request := &mtproto.TLPaymentsSendPaymentForm{
		FormId:      1,
		Invoice:     invoice,
		Credentials: &mtproto.InputPaymentCredentials{PredicateName: mtproto.Predicate_inputPaymentCredentials},
	}
	if _, err := c.PaymentsSendPaymentForm(request); err != mtproto.ErrPaymentUnsupported {
		t.Fatalf("payment without processor: got %v", err)
	}
	if _, err := c.PaymentsGetPaymentForm(&mtproto.TLPaymentsGetPaymentForm{Invoice: invoice}); err != mtproto.ErrPaymentUnsupported {
		t.Fatalf("payment form without processor: got %v", err)
	}
	if _, err := c.PaymentsGetPaymentForm(&mtproto.TLPaymentsGetPaymentForm{}); err != mtproto.ErrInvoicePayloadInvalid {
		t.Fatalf("missing invoice: got %v", err)
	}
	if _, err := c.PaymentsExportInvoice(&mtproto.TLPaymentsExportInvoice{InvoiceMedia: mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia()}); err != mtproto.ErrPaymentUnsupported {
		t.Fatalf("invoice export without provider: got %v", err)
	}
	if _, err := c.PaymentsRequestRecurringPayment(&mtproto.TLPaymentsRequestRecurringPayment{}); err != mtproto.ErrPaymentUnsupported {
		t.Fatalf("recurring payment without processor: got %v", err)
	}
	if raw, err := persist.Default.Get(payReceiptKey(uid)); err != nil || raw != "" {
		t.Fatalf("failed payment created receipt: value=%q err=%v", raw, err)
	}
	if raw, err := persist.Default.Get(payCredKey(uid)); err != nil || raw != "" {
		t.Fatalf("failed payment saved credentials: value=%q err=%v", raw, err)
	}
	if raw, err := persist.Default.Get(fmt.Sprintf("%s%d", payLedgerPrefix, uid)); err != nil || raw != "" {
		t.Fatalf("failed payment created ledger entry: value=%q err=%v", raw, err)
	}
}

func TestPaymentsGetPaymentFormUsesVerifiedProviderForm(t *testing.T) {
	const userID int64 = 902004
	form := &mtproto.Payments_PaymentForm{
		FormId:  42,
		BotId:   7,
		Title:   "Premium",
		Invoice: &mtproto.Invoice{Currency: "USD"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("provider method = %s, want POST", r.Method)
		}
		var request struct {
			Operation         string `json:"operation"`
			UserID            int64  `json:"user_id"`
			BeneficiaryUserID int64  `json:"beneficiary_user_id"`
			RequiredProduct   string `json:"required_product"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if request.Operation != "get_form" || request.UserID != userID || request.BeneficiaryUserID != userID || request.RequiredProduct != paymentProductPremiumSubscription {
			t.Errorf("provider request = %+v, want get_form for user %d", request, userID)
		}
		writeSignedPaymentProviderResponse(t, w, paymentFormProviderResponse{
			Verified:          true,
			UserID:            userID,
			BeneficiaryUserID: userID,
			ProductType:       paymentProductPremiumSubscription,
			PremiumMonths:     3,
			Form:              mtproto.MakeTLPaymentsPaymentForm(form).To_Payments_PaymentForm(),
		})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
	got, err := c.PaymentsGetPaymentForm(&mtproto.TLPaymentsGetPaymentForm{
		Invoice: premiumSelfSubscriptionInvoiceForTest("invoice-slug"),
	})
	if err != nil || got == nil || got.GetFormId() != form.GetFormId() || got.GetInvoice() == nil {
		t.Fatalf("PaymentsGetPaymentForm() = (%#v, %v), want verified provider form", got, err)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("verified payment form did not encode: %v", err)
	}
}

func TestPaymentsValidateRequestedInfoUsesVerifiedProvider(t *testing.T) {
	const userID int64 = 902005
	previous := persist.Default
	spy := &paymentPersistenceSpy{values: map[string]string{}}
	persist.Use(spy)
	defer persist.Use(previous)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("provider method = %s, want POST", r.Method)
		}
		var request struct {
			Operation         string `json:"operation"`
			UserID            int64  `json:"user_id"`
			BeneficiaryUserID int64  `json:"beneficiary_user_id"`
			RequiredProduct   string `json:"required_product"`
			Save              bool   `json:"save"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if request.Operation != "validate_requested_info" || request.UserID != userID || request.BeneficiaryUserID != userID || request.RequiredProduct != paymentProductPremiumSubscription || !request.Save {
			t.Errorf("provider request = %+v, want validation for user %d", request, userID)
		}
		writeSignedPaymentProviderResponse(t, w, paymentValidationProviderResponse{
			Verified:          true,
			UserID:            userID,
			BeneficiaryUserID: userID,
			ProductType:       paymentProductPremiumSubscription,
			ID:                "validated-1",
			ShippingOptions: []*mtproto.ShippingOption{{
				Id:    "standard",
				Title: "Standard",
				Prices: []*mtproto.LabeledPrice{{
					Label:  "Shipping",
					Amount: 99,
				}},
			}},
		})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
	got, err := c.PaymentsValidateRequestedInfo(&mtproto.TLPaymentsValidateRequestedInfo{
		Invoice: premiumSelfSubscriptionInvoiceForTest("invoice-slug"),
		Info: &mtproto.PaymentRequestedInfo{
			Name: wrapperspb.String("Alice"),
		},
		Save: true,
	})
	if err != nil || got == nil || got.GetId().GetValue() != "validated-1" || len(got.GetShippingOptions()) != 1 || got.GetShippingOptions()[0].GetId() != "standard" {
		t.Fatalf("PaymentsValidateRequestedInfo() = (%#v, %v), want verified provider result", got, err)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("verified requested-info result did not encode: %v", err)
	}
	if spy.values[payInfoKey(userID)] != `{"name":"Alice"}` {
		t.Fatalf("saved info = %q, want provider-verified info", spy.values[payInfoKey(userID)])
	}
}

func TestPaymentsValidateRequestedInfoRejectsMalformedProviderResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, paymentValidationProviderResponse{
			Verified:          true,
			UserID:            902006,
			BeneficiaryUserID: 902006,
			ProductType:       paymentProductPremiumSubscription,
			ShippingOptions: []*mtproto.ShippingOption{{
				Title: "missing option id",
			}},
		})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: 902006},
	}
	got, err := c.PaymentsValidateRequestedInfo(&mtproto.TLPaymentsValidateRequestedInfo{
		Invoice: premiumSelfSubscriptionInvoiceForTest("invoice-slug"),
	})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("PaymentsValidateRequestedInfo() = (%#v, %v), want provider-invalid refusal", got, err)
	}
}

func TestPaymentsValidateRequestedInfoAllowsOptionalProviderFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, paymentValidationProviderResponse{
			Verified:          true,
			UserID:            902007,
			BeneficiaryUserID: 902007,
			ProductType:       paymentProductPremiumSubscription,
		})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: 902007},
	}
	got, err := c.PaymentsValidateRequestedInfo(&mtproto.TLPaymentsValidateRequestedInfo{
		Invoice: premiumSelfSubscriptionInvoiceForTest("invoice-slug"),
	})
	if err != nil || got == nil || got.GetId() != nil || got.GetShippingOptions() != nil {
		t.Fatalf("PaymentsValidateRequestedInfo() = (%#v, %v), want verified result with optional fields absent", got, err)
	}
}

func TestPaymentFormMatchesInvoicePurpose(t *testing.T) {
	request := &mtproto.InputInvoice{Purpose: &mtproto.InputStorePaymentPurpose{Currency: "USD", Amount: 499}}
	form := &mtproto.Payments_PaymentForm{Invoice: &mtproto.Invoice{
		Currency: "USD",
		Prices:   []*mtproto.LabeledPrice{{Amount: 499}},
	}}
	if !paymentFormMatchesInvoice(request, form) {
		t.Fatal("matching provider form was rejected")
	}
	form.Invoice.Currency = "EUR"
	if paymentFormMatchesInvoice(request, form) {
		t.Fatal("provider currency mismatch was accepted")
	}
	form.Invoice.Currency = "USD"
	form.Invoice.Prices[0].Amount = 500
	if paymentFormMatchesInvoice(request, form) {
		t.Fatal("provider amount mismatch was accepted")
	}
	if !paymentFormMatchesInvoice(&mtproto.InputInvoice{Slug: "provider-owned"}, form) {
		t.Fatal("slug invoice should defer amount validation to provider")
	}
	if paymentFormMatchesInvoice(&mtproto.InputInvoice{Purpose: &mtproto.InputStorePaymentPurpose{Currency: "USD", Amount: -1}}, form) {
		t.Fatal("negative store-payment amount was accepted")
	}
	form.Invoice.Prices[0].Amount = -1
	if paymentFormMatchesInvoice(&mtproto.InputInvoice{Purpose: &mtproto.InputStorePaymentPurpose{Currency: "USD"}}, form) {
		t.Fatal("negative provider price was accepted for provider-quoted purpose")
	}
}
