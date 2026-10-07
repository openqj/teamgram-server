package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func newBoundaryAccountCore() *AccountCore {
	ctx := context.Background()
	return &AccountCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{},
		Logger: logx.WithContext(ctx),
		MD: &metadata.RpcMetadata{
			PermAuthKeyId: 91,
			UserId:        7,
		},
	}
}

func TestAccountHandlersRejectNilRequests(t *testing.T) {
	c := newBoundaryAccountCore()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "change phone", call: func() error {
			_, err := c.AccountChangePhone(nil)
			return err
		}},
		{name: "confirm phone", call: func() error {
			_, err := c.AccountConfirmPhone(nil)
			return err
		}},
		{name: "send confirm phone code", call: func() error {
			_, err := c.AccountSendConfirmPhoneCode(nil)
			return err
		}},
		{name: "reset authorization", call: func() error {
			_, err := c.AccountResetAuthorization(nil)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("error = %v, want INPUT_REQUEST_INVALID", err)
			}
		})
	}
}

func TestAccountHandlersRejectMissingRuntimeDependencies(t *testing.T) {
	c := newBoundaryAccountCore()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "change phone", call: func() error {
			_, err := c.AccountChangePhone(&mtproto.TLAccountChangePhone{PhoneNumber: "+14155552671", PhoneCodeHash: "hash", PhoneCode: "12345"})
			return err
		}},
		{name: "confirm phone", call: func() error {
			_, err := c.AccountConfirmPhone(&mtproto.TLAccountConfirmPhone{PhoneCodeHash: "hash", PhoneCode: "12345"})
			return err
		}},
		{name: "send confirm phone code", call: func() error {
			_, err := c.AccountSendConfirmPhoneCode(&mtproto.TLAccountSendConfirmPhoneCode{Hash: "hash", Settings: &mtproto.CodeSettings{}})
			return err
		}},
		{name: "reset authorization", call: func() error {
			_, err := c.AccountResetAuthorization(&mtproto.TLAccountResetAuthorization{Hash: 1})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("error = %v, want INTERNAL_SERVER_ERROR", err)
			}
		})
	}
}

func TestAccountHandlersRejectUnauthenticatedRequests(t *testing.T) {
	c := newBoundaryAccountCore()
	c.MD = nil

	if result, err := c.AccountChangePhone(&mtproto.TLAccountChangePhone{PhoneCodeHash: "hash", PhoneCode: "12345"}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("change phone = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	if result, err := c.AccountConfirmPhone(&mtproto.TLAccountConfirmPhone{PhoneCodeHash: "hash", PhoneCode: "12345"}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("confirm phone = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	if result, err := c.AccountResetAuthorization(&mtproto.TLAccountResetAuthorization{Hash: 1}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("reset authorization = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
}
