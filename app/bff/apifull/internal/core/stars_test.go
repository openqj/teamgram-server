package core

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
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
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured")
	}
	cleanup, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	defer cleanup.Close()

	uid := time.Now().UnixNano()
	idem := fmt.Sprintf("stars-read-%d", uid)
	if _, err = domain.ApplyStars(uid, 73, idem); err != nil {
		t.Fatalf("seed stars ledger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = cleanup.Exec(`DELETE FROM apifull_star_tx WHERE user_id=?`, uid)
		_, _ = cleanup.Exec(`DELETE FROM apifull_stars WHERE user_id=?`, uid)
	})

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
