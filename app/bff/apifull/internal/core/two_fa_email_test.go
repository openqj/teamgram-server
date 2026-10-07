package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestPasswordEmailFailsClosedWithoutIssuedCode(t *testing.T) {
	userID := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	original := acctPassword{Email: "recovery@example.com", EmailPending: true}
	if err := saveAcctPassword(userID, original); err != nil {
		t.Fatal(err)
	}

	if _, err := c.AccountConfirmPasswordEmail(&mtproto.TLAccountConfirmPasswordEmail{Code: "client-invented"}); !errors.Is(err, mtproto.ErrCodeInvalid) {
		t.Fatalf("confirm unissued code: got %v", err)
	}
	if _, err := c.AccountConfirmPasswordEmail(&mtproto.TLAccountConfirmPasswordEmail{}); !errors.Is(err, mtproto.ErrCodeEmpty) {
		t.Fatalf("confirm empty code: got %v", err)
	}
	if _, err := c.AccountResendPasswordEmail(nil); !errors.Is(err, mtproto.ErrSendCodeUnavailable) {
		t.Fatalf("resend without email transport: got %v", err)
	}

	got, err := loadAcctPassword(userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != original.Email || !got.EmailPending || got.EmailCode != "" {
		t.Fatalf("email state changed without an issued code: %+v", got)
	}
}

func TestPasswordEmailRequiresConfiguredAddress(t *testing.T) {
	userID := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	if _, err := c.AccountConfirmPasswordEmail(&mtproto.TLAccountConfirmPasswordEmail{Code: "any"}); !errors.Is(err, mtproto.ErrEmailNotSetup) {
		t.Fatalf("confirm without configured email: got %v", err)
	}
	if _, err := c.AccountResendPasswordEmail(nil); !errors.Is(err, mtproto.ErrEmailNotSetup) {
		t.Fatalf("resend without configured email: got %v", err)
	}
}

func TestPasswordEmailCancelClearsPendingState(t *testing.T) {
	userID := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	if err := saveAcctPassword(userID, acctPassword{
		Email: "pending@example.com", EmailPending: true, EmailCode: "issued-challenge",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := c.AccountCancelPasswordEmail(&mtproto.TLAccountCancelPasswordEmail{})
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("cancel password email = (%v, %v), want BoolTrue", result, err)
	}
	got, err := loadAcctPassword(userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "" || got.EmailPending || got.EmailCode != "" {
		t.Fatalf("pending email state remains after cancel: %+v", got)
	}
}
