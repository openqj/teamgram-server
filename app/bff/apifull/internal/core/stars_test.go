package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStarsStatusUnavailableWithoutCanonicalLedger(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	self := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	result, err := c.PaymentsGetStarsStatus(&mtproto.TLPaymentsGetStarsStatus{Peer: self})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("status = (%#v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
}

func TestStarsStatusRejectsOtherPeer(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	other := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer()
	result, err := c.PaymentsGetStarsStatus(&mtproto.TLPaymentsGetStarsStatus{Peer: other})
	if result != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("status = (%#v, %v), want (nil, PEER_ID_INVALID)", result, err)
	}
}

func TestStarsStatusRejectsNilRequest(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	result, err := c.PaymentsGetStarsStatus(nil)
	if result != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("status = (%#v, %v), want (nil, INPUT_CONSTRUCTOR_INVALID)", result, err)
	}
}

func TestStarsTopupOptionsFailClosedWithoutPaymentProcessor(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	result, err := c.PaymentsGetStarsTopupOptions(nil)
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("top-up options = (%#v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
}

func TestStarsTransactionReadsRejectNilRequests(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	calls := []struct {
		name   string
		invoke func() (bool, error)
	}{
		{"transactions", func() (bool, error) {
			result, err := c.PaymentsGetStarsTransactions(nil)
			return result != nil, err
		}},
		{"transactions by id", func() (bool, error) {
			result, err := c.PaymentsGetStarsTransactionsByID(nil)
			return result != nil, err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.invoke()
			if result || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
				t.Fatalf("result present=%v err=%v, want nil result and INPUT_CONSTRUCTOR_INVALID", result, err)
			}
		})
	}
}

func TestStarsTransactionMappingUsesLedgerFieldsOnly(t *testing.T) {
	got := makeStarsTransaction(domain.StarsTransaction{ID: 7, Amount: -12, Idem: "debit-7"})
	if got == nil || got.GetId() != "debit-7" || got.GetStars_INT64() != -12 {
		t.Fatalf("transaction mapping = %#v, want id/amount from ledger", got)
	}
	if got.GetDate() != 0 || got.GetTitle() != nil || got.GetPeer() != nil {
		t.Fatalf("transaction mapping synthesized provider metadata: %#v", got)
	}
}

func TestStarsOffsetRejectsMalformedValues(t *testing.T) {
	for _, offset := range []string{"-1", "letters"} {
		if _, err := parseStarsOffset(offset); !errors.Is(err, mtproto.ErrOffsetInvalid) {
			t.Fatalf("offset %q error = %v, want OFFSET_INVALID", offset, err)
		}
	}
}

func TestStarsStatusMappingUsesLedgerBalance(t *testing.T) {
	status := makeStarsStatus(37, nil, nil)
	if status == nil || status.GetBalance_INT64() != 37 || status.GetBalance_STARSAMOUNT().GetAmount() != 37 {
		t.Fatalf("stars status = %#v, want balance 37", status)
	}
	if len(status.GetSubscriptions()) != 0 || len(status.GetChats()) != 0 || len(status.GetUsers()) != 0 {
		t.Fatalf("stars status synthesized related records: %#v", status)
	}
}

func TestStarsReadMethodsUseDurableLedger(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	cleanup, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	t.Cleanup(func() { _ = cleanup.Close() })

	uid := time.Now().UnixNano()
	idem := fmt.Sprintf("stars-read-%d", uid)
	t.Cleanup(func() {
		_, _ = cleanup.Exec(`DELETE FROM apifull_star_tx WHERE user_id=$1`, uid)
		_, _ = cleanup.Exec(`DELETE FROM apifull_stars WHERE user_id=$1`, uid)
	})
	if _, err = domain.ApplyStars(uid, 73, idem); err != nil {
		t.Fatalf("seed stars ledger: %v", err)
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	status, err := c.PaymentsGetStarsStatus(&mtproto.TLPaymentsGetStarsStatus{})
	if err != nil || status == nil || status.GetBalance_INT64() != 73 {
		t.Fatalf("status = (%#v, %v), want durable balance 73", status, err)
	}
	history, err := c.PaymentsGetStarsTransactions(&mtproto.TLPaymentsGetStarsTransactions{Limit: 10})
	if err != nil || history == nil || history.GetBalance_INT64() != 73 || len(history.GetHistory()) != 1 {
		t.Fatalf("history = (%#v, %v), want one durable transaction", history, err)
	}
	if got := history.GetHistory()[0].GetId(); got != idem {
		t.Fatalf("transaction id = %q, want %q", got, idem)
	}
	byID, err := c.PaymentsGetStarsTransactionsByID(&mtproto.TLPaymentsGetStarsTransactionsByID{
		Id: []*mtproto.InputStarsTransaction{
			mtproto.MakeTLInputStarsTransaction(&mtproto.InputStarsTransaction{Id: idem}).To_InputStarsTransaction(),
		},
	})
	if err != nil || byID == nil || len(byID.GetHistory()) != 1 || byID.GetHistory()[0].GetId() != idem {
		t.Fatalf("by id = (%#v, %v), want the durable transaction", byID, err)
	}
}

func TestStarsPageDefaultsAndValidation(t *testing.T) {
	start, limit, err := starsPage("", 0)
	if err != nil || start != 0 || limit != 100 {
		t.Fatalf("starsPage default = (%d, %d, %v), want (0, 100, nil)", start, limit, err)
	}
	if _, _, err = starsPage("-1", 10); !errors.Is(err, mtproto.ErrOffsetInvalid) {
		t.Fatalf("starsPage invalid offset = %v, want OFFSET_INVALID", err)
	}
	if _, _, err = starsPage("0", -1); !errors.Is(err, mtproto.ErrLimitInvalid) {
		t.Fatalf("starsPage invalid limit = %v, want LIMIT_INVALID", err)
	}
}

func TestStarsRevenueMethodsUseVerifiedProvider(t *testing.T) {
	const uid int64 = 902701
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	graph := mtproto.MakeTLStatsGraph(&mtproto.StatsGraph{
		Json: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"points":[1,2]}`}).To_DataJSON(),
	}).To_StatsGraph()
	status := mtproto.MakeTLStarsRevenueStatus(&mtproto.StarsRevenueStatus{
		WithdrawalEnabled:            true,
		CurrentBalance_STARSAMOUNT:   mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: 50}).To_StarsAmount(),
		AvailableBalance_STARSAMOUNT: mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: 40}).To_StarsAmount(),
		OverallRevenue_STARSAMOUNT:   mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: 80}).To_StarsAmount(),
	}).To_StarsRevenueStatus()
	stats := mtproto.MakeTLPaymentsStarsRevenueStats(&mtproto.Payments_StarsRevenueStats{
		RevenueGraph: graph,
		Status:       status,
		UsdRate:      1.25,
	}).To_Payments_StarsRevenueStats()
	var operations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Operation string `json:"operation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode revenue request: %v", err)
			return
		}
		operations = append(operations, request.Operation)
		switch request.Operation {
		case "get_stars_revenue_stats":
			writeSignedPaymentProviderResponse(t, w, starsRevenueStatsProviderResponse{Verified: true, UserID: uid, Stats: stats})
		case "get_stars_revenue_withdrawal_url", "get_stars_revenue_ads_account_url":
			writeSignedPaymentProviderResponse(t, w, starsRevenueURLProviderResponse{Verified: true, UserID: uid, URL: "https://payments.example.test/account"})
		default:
			http.Error(w, "unexpected operation", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	gotStats, err := c.PaymentsGetStarsRevenueStats(&mtproto.TLPaymentsGetStarsRevenueStats{Peer: peer})
	if err != nil || gotStats == nil || gotStats.GetStatus() == nil || gotStats.GetStatus().GetOverallRevenue_STARSAMOUNT() == nil || gotStats.GetStatus().GetOverallRevenue_STARSAMOUNT().GetAmount() != 80 {
		t.Fatalf("revenue stats = (%#v, %v)", gotStats, err)
	}
	if err = gotStats.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("revenue stats did not encode: %v", err)
	}
	password := &mtproto.InputCheckPasswordSRP{SrpId: 1, A: []byte{1}, M1: []byte{1}}
	gotURL, err := c.PaymentsGetStarsRevenueWithdrawalUrl(&mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl{
		Peer: peer, Amount: wrapperspb.Int64(10), Password: password,
	})
	if err != nil || gotURL == nil || gotURL.GetUrl() != "https://payments.example.test/account" {
		t.Fatalf("withdrawal URL = (%#v, %v)", gotURL, err)
	}
	if err = gotURL.Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("withdrawal URL did not encode: %v", err)
	}
	adsURL, err := c.PaymentsGetStarsRevenueAdsAccountUrl(&mtproto.TLPaymentsGetStarsRevenueAdsAccountUrl{Peer: peer})
	if err != nil || adsURL == nil || adsURL.GetUrl() != "https://payments.example.test/account" {
		t.Fatalf("ads URL = (%#v, %v)", adsURL, err)
	}
	if err = adsURL.Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("ads URL did not encode: %v", err)
	}
	want := []string{"get_stars_revenue_stats", "get_stars_revenue_withdrawal_url", "get_stars_revenue_ads_account_url"}
	if fmt.Sprint(operations) != fmt.Sprint(want) {
		t.Fatalf("provider operations = %v, want %v", operations, want)
	}
}

func TestStarsRevenueMethodsRejectMalformedProviderData(t *testing.T) {
	const uid int64 = 902702
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, starsRevenueURLProviderResponse{Verified: true, UserID: uid, URL: "http://insecure.example.test/account"})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetStarsRevenueAdsAccountUrl(&mtproto.TLPaymentsGetStarsRevenueAdsAccountUrl{Peer: peer})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("ads URL = (%#v, %v), want provider-invalid refusal", got, err)
	}
}

func TestStarsRefundChargeUsesVerifiedProviderAndPostgresLedger(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	cleanup, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	t.Cleanup(func() { _ = cleanup.Close() })
	uid := time.Now().UnixNano()
	chargeID := fmt.Sprintf("refund-charge-%d", uid)
	requestKey := fmt.Sprintf("refund-request-%d", uid)
	fingerprint := fmt.Sprintf("refund-fingerprint-%d", uid)
	t.Cleanup(func() {
		for _, table := range []string{"apifull_stars_refund", "apifull_star_tx", "apifull_stars", "apifull_payment_receipt", "apifull_payment_ledger", "apifull_payment_request"} {
			if _, cleanupErr := cleanup.Exec(`DELETE FROM `+table+` WHERE user_id=$1`, uid); cleanupErr != nil {
				t.Errorf("clean %s fixture: %v", table, cleanupErr)
			}
		}
	})
	if _, err = domain.BeginPaymentRequest(uid, requestKey, "external", fingerprint, "USD", 199, 0, 0); err != nil {
		t.Fatalf("begin payment: %v", err)
	}
	if _, _, err = domain.SettleStarsPaymentRequest(uid, requestKey, fingerprint, chargeID, "USD", "Stars", 199, 40, []byte(`{"provider":true}`)); err != nil {
		t.Fatalf("settle payment: %v", err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
			ChargeID  string `json:"charge_id"`
			Stars     int64  `json:"stars"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode refund request: %v", err)
			return
		}
		if request.Operation != "refund_stars_charge" || request.UserID != uid || request.ChargeID != chargeID || request.Stars != 40 {
			t.Errorf("refund request = %+v", request)
		}
		writeSignedPaymentProviderResponse(t, w, starsRefundProviderResponse{
			Verified: true, CallerUserID: uid, UserID: uid, ChargeID: chargeID, Provider: "external", Stars: 40,
			RefundTransactionID: "refund-transaction-1", Receipt: json.RawMessage(`{"provider_refund":true}`),
		})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	for attempt := 0; attempt < 2; attempt++ {
		updates, callErr := c.PaymentsRefundStarsCharge(&mtproto.TLPaymentsRefundStarsCharge{UserId: self, ChargeId: chargeID})
		if callErr != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates {
			t.Fatalf("refund attempt %d = (%#v, %v)", attempt, updates, callErr)
		}
		if err = updates.Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
			t.Fatalf("refund updates did not encode: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want one idempotent call", calls.Load())
	}
	if balance, balanceErr := domain.StarsBalance(uid); balanceErr != nil || balance != 0 {
		t.Fatalf("refunded Stars balance = %d err=%v", balance, balanceErr)
	}
}

func TestStarsFormSettlementUsesSignedProviderAndPostgresLedger(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	t.Cleanup(func() { _ = domain.Close() })
	cleanup, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup.Close() })

	uid := time.Now().UnixNano()
	product := fmt.Sprintf("stars-form-%d", uid)
	formID := int64(7401)
	const stars int64 = 100
	const amount int64 = 199
	if err = domain.UpsertStarsOffer("topup", stars, product, "USD", amount, false); err != nil {
		t.Fatalf("publish Stars offer: %v", err)
	}
	t.Cleanup(func() {
		for _, table := range []string{"apifull_payment_receipt", "apifull_payment_ledger", "apifull_payment_request", "apifull_star_tx", "apifull_stars"} {
			if _, cleanupErr := cleanup.Exec(`DELETE FROM `+table+` WHERE user_id=$1`, uid); cleanupErr != nil {
				t.Errorf("clean %s fixture: %v", table, cleanupErr)
			}
		}
		if _, cleanupErr := cleanup.Exec(`DELETE FROM apifull_stars_offer WHERE store_product=$1`, product); cleanupErr != nil {
			t.Errorf("clean Stars offer fixture: %v", cleanupErr)
		}
	})

	option := mtproto.MakeTLStarsTopupOption(&mtproto.StarsTopupOption{
		Stars:        stars,
		StoreProduct: wrapperspb.String(product),
		Currency:     "USD",
		Amount:       amount,
	}).To_StarsTopupOption()
	invoice := mtproto.MakeTLInputInvoiceStars(&mtproto.InputInvoice{
		Option_STARSTOPUPOPTION: option,
	}).To_InputInvoice()
	request := &mtproto.TLPaymentsSendStarsForm{
		Constructor: mtproto.TLConstructor_CRC32_payments_sendStarsForm_7998c914,
		FormId:      formID,
		Invoice:     invoice,
	}
	if err = request.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("Stars form request did not encode at Layer 229: %v", err)
	}
	fingerprint := starsPaymentFingerprint(formID, invoice)
	requestKey := starsPaymentRequestKey(uid, formID, fingerprint)
	offer, found, err := domain.FindActiveStarsTopupOffer(stars, product, "USD", amount, nil)
	if err != nil || !found {
		t.Fatalf("load published Stars offer: %+v found=%v err=%v", offer, found, err)
	}
	transactionID := fmt.Sprintf("stars-form-transaction-%d", uid)
	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		var submitted struct {
			Operation         string            `json:"operation"`
			UserID            int64             `json:"user_id"`
			BeneficiaryUserID int64             `json:"beneficiary_user_id"`
			RequiredProduct   string            `json:"required_product"`
			RequestKey        string            `json:"request_key"`
			Fingerprint       string            `json:"fingerprint"`
			FormID            int64             `json:"form_id"`
			OfferID           int64             `json:"offer_id"`
			Offer             domain.StarsOffer `json:"offer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
			t.Errorf("decode Stars settlement request: %v", err)
			http.Error(w, "invalid fixture", http.StatusBadRequest)
			return
		}
		if submitted.Operation != "settle_stars" || submitted.UserID != uid || submitted.BeneficiaryUserID != uid ||
			submitted.RequiredProduct != "stars_topup" || submitted.RequestKey != requestKey || submitted.Fingerprint != fingerprint ||
			submitted.FormID != formID || submitted.OfferID != offer.ID || submitted.Offer.Stars != stars ||
			submitted.Offer.StoreProduct != product || r.Header.Get("Idempotency-Key") != paymentProviderIdempotencyKey(uid, requestKey) {
			t.Errorf("Stars settlement request binding = %+v, idempotency=%q", submitted, r.Header.Get("Idempotency-Key"))
		}
		writeSignedPaymentProviderResponse(t, w, paymentProviderResponse{
			Verified: true, UserID: uid, BeneficiaryUserID: uid, RequestKey: requestKey, Fingerprint: fingerprint,
			FormID: formID, ProductType: "stars_topup", OfferID: offer.ID, Stars: stars, StoreProduct: product,
			TransactionID: transactionID, Currency: "USD", Amount: amount, Title: "Stars top-up",
			Receipt: json.RawMessage(`{"provider_transaction":true}`),
		})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, callErr := c.PaymentsSendStarsForm(request)
		if callErr != nil || result == nil || result.GetPredicateName() != mtproto.Predicate_payments_paymentResult {
			t.Fatalf("Stars settlement attempt %d = (%#v, %v)", attempt, result, callErr)
		}
		if err = result.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
			t.Fatalf("Stars payment result did not encode at Layer 229: %v", err)
		}
	}
	if providerCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want one idempotent settlement", providerCalls.Load())
	}
	if balance, balanceErr := domain.StarsBalance(uid); balanceErr != nil || balance != stars {
		t.Fatalf("Stars balance = %d err=%v, want %d", balance, balanceErr, stars)
	}
	var transactions, receipts, ledgerEntries int
	if err = cleanup.QueryRow(`SELECT COUNT(*) FROM apifull_star_tx WHERE user_id=$1`, uid).Scan(&transactions); err != nil || transactions != 1 {
		t.Fatalf("Stars transaction rows = %d err=%v, want one", transactions, err)
	}
	if err = cleanup.QueryRow(`SELECT COUNT(*) FROM apifull_payment_receipt WHERE user_id=$1`, uid).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("payment receipt rows = %d err=%v, want one", receipts, err)
	}
	if err = cleanup.QueryRow(`SELECT COUNT(*) FROM apifull_payment_ledger WHERE user_id=$1`, uid).Scan(&ledgerEntries); err != nil || ledgerEntries != 2 {
		t.Fatalf("payment ledger rows = %d err=%v, want pending+settled", ledgerEntries, err)
	}
}
