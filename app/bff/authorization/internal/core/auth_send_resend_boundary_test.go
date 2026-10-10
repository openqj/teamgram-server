package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestAuthSendCodeRejectsInvalidBoundary(t *testing.T) {
	var nilCore *AuthorizationCore
	if result, err := nilCore.AuthSendCode(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil send-code core = (%v,%v), want INPUT_REQUEST_INVALID", result, err)
	}
	core := &AuthorizationCore{ctx: context.Background(), Logger: logx.WithContext(context.Background())}
	if result, err := core.AuthSendCode(&mtproto.TLAuthSendCode{}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("missing send-code metadata = (%v,%v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	core.MD = &metadata.RpcMetadata{PermAuthKeyId: 901001}
	if result, err := core.AuthSendCode(&mtproto.TLAuthSendCode{}); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil send-code settings = (%v,%v), want INPUT_REQUEST_INVALID", result, err)
	}
	core.svcCtx = &svc.ServiceContext{Dao: &dao.Dao{}}
	request := &mtproto.TLAuthSendCode{Settings: mtproto.MakeTLCodeSettings(nil).To_CodeSettings()}
	if result, err := core.AuthSendCode(request); result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing send-code runtime = (%v,%v), want INTERNAL_SERVER_ERROR", result, err)
	}
}

func TestAuthResendCodeRejectsInvalidBoundary(t *testing.T) {
	var nilCore *AuthorizationCore
	if result, err := nilCore.AuthResendCode(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil resend-code core = (%v,%v), want INPUT_REQUEST_INVALID", result, err)
	}
	core := &AuthorizationCore{ctx: context.Background(), Logger: logx.WithContext(context.Background())}
	if result, err := core.AuthResendCode(&mtproto.TLAuthResendCode{}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("missing resend-code metadata = (%v,%v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	core.MD = &metadata.RpcMetadata{PermAuthKeyId: 901001}
	if result, err := core.AuthResendCode(&mtproto.TLAuthResendCode{}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing resend-code runtime = (%v,%v), want METHOD_NOT_IMPL", result, err)
	}
}
