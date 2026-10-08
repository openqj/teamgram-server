package core

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func isolatedAuditDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured; PostgreSQL runtime tests cover production storage")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse APIFULL_MYSQL_DSN: %v", err)
	}
	if cfg.DBName != "teamgram_audit" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13306" {
		t.Fatalf("test requires the isolated audit database, got network=%q addr=%q db=%q", cfg.Net, cfg.Addr, cfg.DBName)
	}
	return dsn
}

type unavailablePaymentStore struct {
	reads  int
	writes int
}

func (s *unavailablePaymentStore) Get(string) (string, error) {
	s.reads++
	return "", nil
}

func (s *unavailablePaymentStore) Set(string, string) error {
	s.writes++
	return nil
}

func TestPaymentsSendPaymentForm(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.PaymentsSendPaymentForm(nil); err != mtproto.ErrInvoicePayloadInvalid {
		t.Fatalf("PaymentsSendPaymentForm: got %v", err)
	}
}

func TestPaymentReceiptReadbackUsesSettledLedger(t *testing.T) {
	dsn := isolatedAuditDSN(t)
	cleanup, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	defer cleanup.Close()

	uid := time.Now().UnixNano()
	requestKey := fmt.Sprintf("receipt-read-%d", uid)
	fingerprint := fmt.Sprintf("invoice-%d", uid)
	const msgID int32 = 17
	if _, err = domain.BeginPaymentRequest(uid, requestKey, "test-provider", fingerprint, "USD", 499, uid, msgID); err != nil {
		t.Fatalf("begin payment: %v", err)
	}
	transactionID := fmt.Sprintf("tx-receipt-%d", uid)
	if _, _, err = domain.SettlePaymentRequest(uid, requestKey, fingerprint, transactionID, "USD", "Test invoice", 499, uid, msgID, []byte("receipt"), true); err != nil {
		t.Fatalf("settle payment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = cleanup.Exec(`DELETE FROM apifull_payment_receipt WHERE user_id=?`, uid)
		_, _ = cleanup.Exec(`DELETE FROM apifull_payment_ledger WHERE user_id=?`, uid)
		_, _ = cleanup.Exec(`DELETE FROM apifull_payment_request WHERE user_id=?`, uid)
	})

	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	receipt, err := c.PaymentsGetPaymentReceipt(&mtproto.TLPaymentsGetPaymentReceipt{Peer: peer, MsgId: msgID})
	if err != nil || receipt == nil {
		t.Fatalf("receipt = (%#v, %v), want settled receipt", receipt, err)
	}
	if receipt.GetTransactionId() != transactionID || receipt.GetCurrency() != "USD" || receipt.GetTotalAmount() != 499 || receipt.GetTitle() != "Test invoice" {
		t.Fatalf("receipt = %#v, want provider-settled fields", receipt)
	}
}

func TestStarsUnavailableDoesNotAccessPersistence(t *testing.T) {
	store := &unavailablePaymentStore{}
	previous := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(previous) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	calls := []struct {
		name   string
		invoke func() (bool, error)
	}{
		{"send stars form", func() (bool, error) {
			result, err := c.PaymentsSendStarsForm(&mtproto.TLPaymentsSendStarsForm{})
			return result != nil, err
		}},
		{"refund stars charge", func() (bool, error) {
			result, err := c.PaymentsRefundStarsCharge(&mtproto.TLPaymentsRefundStarsCharge{ChargeId: "charge"})
			return result != nil, err
		}},
		{"top-up options", func() (bool, error) {
			result, err := c.PaymentsGetStarsTopupOptions(nil)
			return result != nil, err
		}},
		{"gift options", func() (bool, error) {
			self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
			result, err := c.PaymentsGetStarsGiftOptions(&mtproto.TLPaymentsGetStarsGiftOptions{UserId: self})
			return result != nil, err
		}},
		{"revenue stats", func() (bool, error) {
			result, err := c.PaymentsGetStarsRevenueStats(nil)
			return result != nil, err
		}},
		{"revenue withdrawal URL", func() (bool, error) {
			result, err := c.PaymentsGetStarsRevenueWithdrawalUrl(nil)
			return result != nil, err
		}},
		{"revenue ads account URL", func() (bool, error) {
			result, err := c.PaymentsGetStarsRevenueAdsAccountUrl(nil)
			return result != nil, err
		}},
	}

	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			resultPresent, err := tc.invoke()
			if resultPresent || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result present=%v err=%v, want nil result and METHOD_NOT_IMPL", resultPresent, err)
			}
		})
	}
	if store.reads != 0 || store.writes != 0 {
		t.Fatalf("unavailable Stars methods accessed persistence: reads=%d writes=%d", store.reads, store.writes)
	}
}
