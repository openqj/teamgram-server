package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	qrcodeconfig "github.com/teamgram/teamgram-server/app/bff/qrcode/internal/config"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestLoginTokenMethodsRejectNilRequest(t *testing.T) {
	c := &QrCodeCore{Logger: logx.WithContext(context.Background())}
	if got, err := c.AuthImportLoginToken(nil); got != nil || !errors.Is(err, mtproto.ErrAuthTokenInvalid) {
		t.Fatalf("AuthImportLoginToken(nil) = (%#v, %v), want (nil, AUTH_TOKEN_INVALID)", got, err)
	}
	if got, err := c.AuthAcceptLoginToken(nil); got != nil || !errors.Is(err, mtproto.ErrAuthTokenInvalid) {
		t.Fatalf("AuthAcceptLoginToken(nil) = (%#v, %v), want (nil, AUTH_TOKEN_INVALID)", got, err)
	}
	var nilCore *QrCodeCore
	if got, err := nilCore.AuthImportLoginToken(nil); got != nil || !errors.Is(err, mtproto.ErrAuthTokenInvalid) {
		t.Fatalf("(*QrCodeCore)(nil).AuthImportLoginToken(nil) = (%#v, %v), want (nil, AUTH_TOKEN_INVALID)", got, err)
	}
	if got, err := nilCore.AuthAcceptLoginToken(nil); got != nil || !errors.Is(err, mtproto.ErrAuthTokenInvalid) {
		t.Fatalf("(*QrCodeCore)(nil).AuthAcceptLoginToken(nil) = (%#v, %v), want (nil, AUTH_TOKEN_INVALID)", got, err)
	}
}

func TestQrLoginTokenMigrationTarget(t *testing.T) {
	cfg := qrcodeconfig.Config{DcId: 2, KnownDcIds: []int32{1}}
	if got, err := qrLoginTokenMigrationTarget(cfg, 2, 1); err != nil || got != 1 {
		t.Fatalf("remote migration target = (%d, %v), want (1, nil)", got, err)
	}
	if got, err := qrLoginTokenMigrationTarget(cfg, 2, 2); err != nil || got != 0 {
		t.Fatalf("local migration target = (%d, %v), want (0, nil)", got, err)
	}
	if got, err := qrLoginTokenMigrationTarget(cfg, 2, 3); !errors.Is(err, mtproto.ErrDcIdInvalid) || got != 0 {
		t.Fatalf("unknown migration target = (%d, %v), want (0, DC_ID_INVALID)", got, err)
	}
	if got, err := qrLoginTokenMigrationTarget(qrcodeconfig.Config{}, 0, 0); err != nil || got != 0 {
		t.Fatalf("legacy migration target = (%d, %v), want (0, nil)", got, err)
	}
}

func TestLoginTokenMethodsFailClosedWhenCoreUnavailable(t *testing.T) {
	token := make([]byte, 24)
	requestImport := &mtproto.TLAuthImportLoginToken{Token: token}
	requestAccept := &mtproto.TLAuthAcceptLoginToken{Token: token}
	c := &QrCodeCore{}

	if got, err := c.AuthImportLoginToken(requestImport); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AuthImportLoginToken(unavailable) = (%#v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
	if got, err := c.AuthAcceptLoginToken(requestAccept); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AuthAcceptLoginToken(unavailable) = (%#v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}
