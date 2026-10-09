package core

import (
	"context"
	"database/sql"
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
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type premiumPaymentTestClient struct {
	userclient.UserClient
	grant func(context.Context, *userpb.TLUserUpdatePremium) (*mtproto.Bool, error)
}

func (c *premiumPaymentTestClient) UserUpdatePremium(ctx context.Context, in *userpb.TLUserUpdatePremium) (*mtproto.Bool, error) {
	return c.grant(ctx, in)
}

func TestPostgresPremiumPaymentWireReplayDoesNotChargeAgain(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	cleanup, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup.Close() })
	uid := time.Now().UnixNano()
	transactionID := fmt.Sprintf("premium-wire-%d", uid)
	t.Cleanup(func() {
		for _, table := range []string{"apifull_payment_saved_info", "apifull_payment_entitlement_outbox", "apifull_payment_receipt", "apifull_payment_ledger", "apifull_payment_request"} {
			if _, err := cleanup.Exec(`DELETE FROM `+table+` WHERE user_id=$1`, uid); err != nil {
				t.Errorf("clean %s fixture: %v", table, err)
			}
		}
	})
	request := &mtproto.TLPaymentsSendPaymentForm{
		Constructor: mtproto.TLConstructor_CRC32_payments_sendPaymentForm_2d03522f,
		FormId:      42,
		Invoice: mtproto.MakeTLInputInvoiceSlug(&mtproto.InputInvoice{
			Slug: "premium-subscription",
		}).To_InputInvoice(),
		Credentials: mtproto.MakeTLInputPaymentCredentials(&mtproto.InputPaymentCredentials{
			Save: true,
			Data: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"fixture":true}`}).To_DataJSON(),
		}).To_InputPaymentCredentials(),
	}
	buf := mtproto.NewEncodeBuf(256)
	if err = request.Encode(buf, 229); err != nil {
		t.Fatal(err)
	}
	decode := mtproto.NewDecodeBuf(buf.GetBuf())
	object := decode.Object()
	if err = decode.GetError(); err != nil {
		t.Fatal(err)
	}
	wireRequest, ok := object.(*mtproto.TLPaymentsSendPaymentForm)
	if !ok || decode.GetOffset() != decode.GetSize() || wireRequest.GetInvoice().GetPurpose() != nil {
		t.Fatalf("wire request did not preserve the Layer 229 slug constructor: %T", object)
	}
	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		var submitted struct {
			UserID            int64  `json:"user_id"`
			BeneficiaryUserID int64  `json:"beneficiary_user_id"`
			RequiredProduct   string `json:"required_product"`
			RequestKey        string `json:"request_key"`
			Fingerprint       string `json:"fingerprint"`
			FormID            int64  `json:"form_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
			t.Errorf("provider request: %v", err)
			http.Error(w, "invalid fixture", http.StatusBadRequest)
			return
		}
		if submitted.UserID != uid || submitted.BeneficiaryUserID != uid || submitted.RequiredProduct != paymentProductPremiumSubscription ||
			submitted.FormID != request.FormId || r.Header.Get("Idempotency-Key") != paymentProviderIdempotencyKey(uid, submitted.RequestKey) {
			t.Errorf("provider binding is incorrect: %+v", submitted)
		}
		writeSignedPaymentProviderResponse(t, w, paymentProviderResponse{
			Verified: true, UserID: uid, BeneficiaryUserID: uid,
			RequestKey: submitted.RequestKey, Fingerprint: submitted.Fingerprint, FormID: submitted.FormID,
			ProductType: paymentProductPremiumSubscription, PremiumMonths: 3,
			TransactionID: transactionID, Currency: "USD", Amount: 499, Title: "Premium",
			Receipt: json.RawMessage(`{"provider_verified":true}`),
		})
	}))
	defer server.Close()
	grantUnavailable := errors.New("user service unavailable")
	grantCalls := 0
	client := &premiumPaymentTestClient{grant: func(_ context.Context, in *userpb.TLUserUpdatePremium) (*mtproto.Bool, error) {
		grantCalls++
		if in.GetUserId() != uid || in.GetTransactionId() != transactionID || in.GetProvider() != "external" || in.GetMonths().GetValue() != 3 || !mtproto.FromBool(in.GetPremium()) {
			t.Fatalf("Premium grant has incorrect binding: %+v", in)
		}
		if grantCalls == 1 {
			return nil, grantUnavailable
		}
		return mtproto.BoolTrue, nil
	}}
	c := &ApiFullCore{
		ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: uid},
		svcCtx: &svc.ServiceContext{
			Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey},
			Dao:    &dao.Dao{UserClient: client},
		},
	}
	if result, err := c.PaymentsSendPaymentForm(wireRequest); result != nil || !errors.Is(err, grantUnavailable) {
		t.Fatalf("first response: result=%+v err=%v", result, err)
	}
	key := paymentRequestKey(uid, wireRequest)
	settled, found, err := domain.LoadPaymentRequest(uid, key)
	if err != nil || !found || settled.State != domain.PaymentStateSettled {
		t.Fatalf("payment was lost when User RPC failed: %+v found=%v err=%v", settled, found, err)
	}
	var state string
	if err = cleanup.QueryRow(`SELECT state FROM apifull_payment_entitlement_outbox WHERE request_id=$1`, settled.ID).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("grant intent missing: state=%s err=%v", state, err)
	}
	var credentialsSaved bool
	if err = cleanup.QueryRow(`SELECT credentials_saved FROM apifull_payment_saved_info WHERE user_id=$1`, uid).Scan(&credentialsSaved); !errors.Is(err, sql.ErrNoRows) || credentialsSaved {
		t.Fatalf("failed entitlement unexpectedly saved credentials: saved=%v err=%v", credentialsSaved, err)
	}
	result, err := c.PaymentsSendPaymentForm(wireRequest)
	if err != nil || result == nil || result.GetPredicateName() != mtproto.Predicate_payments_paymentResult {
		t.Fatalf("retry response: result=%+v err=%v", result, err)
	}
	if err = result.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("payment result is not encodable at Layer 229: %v", err)
	}
	if providerCalls.Load() != 1 || grantCalls != 2 {
		t.Fatalf("retry side effects: provider calls=%d grant attempts=%d", providerCalls.Load(), grantCalls)
	}
	if err = cleanup.QueryRow(`SELECT state FROM apifull_payment_entitlement_outbox WHERE request_id=$1`, settled.ID).Scan(&state); err != nil || state != "complete" {
		t.Fatalf("successful grant did not complete its intent: state=%s err=%v", state, err)
	}
	if err = cleanup.QueryRow(`SELECT credentials_saved FROM apifull_payment_saved_info WHERE user_id=$1`, uid).Scan(&credentialsSaved); err != nil || !credentialsSaved {
		t.Fatalf("successful saved-credential marker missing: saved=%v err=%v", credentialsSaved, err)
	}
}
