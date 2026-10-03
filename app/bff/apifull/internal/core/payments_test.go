package core

import (
	"context"
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
)

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
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if request.Operation != "get_form" || request.UserID != userID {
			t.Errorf("provider request = %+v, want get_form for user %d", request, userID)
		}
		_ = json.NewEncoder(w).Encode(paymentFormProviderResponse{Verified: true, Form: form})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
	got, err := c.PaymentsGetPaymentForm(&mtproto.TLPaymentsGetPaymentForm{
		Invoice: &mtproto.InputInvoice{Slug: "invoice-slug"},
	})
	if err != nil || got == nil || got.GetFormId() != form.GetFormId() || got.GetInvoice() == nil {
		t.Fatalf("PaymentsGetPaymentForm() = (%#v, %v), want verified provider form", got, err)
	}
}
