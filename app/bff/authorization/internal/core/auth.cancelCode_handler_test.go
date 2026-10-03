package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestAuthCancelCodeRejectsNilRequest(t *testing.T) {
	var c *AuthorizationCore
	if result, err := c.AuthCancelCode(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("AuthCancelCode(nil) = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}
}

func TestAuthCancelCodeFailsClosedWithoutProviders(t *testing.T) {
	ctx := context.Background()
	c := &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 42},
		svcCtx: &svc.ServiceContext{},
	}
	result, err := c.AuthCancelCode(&mtproto.TLAuthCancelCode{
		PhoneNumber:   "+15551234567",
		PhoneCodeHash: "hash",
	})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AuthCancelCode() = (%v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
}
