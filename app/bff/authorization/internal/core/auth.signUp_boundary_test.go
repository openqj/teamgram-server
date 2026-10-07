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

func newBoundaryAuthorizationCore() *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{},
		Logger: logx.WithContext(ctx),
		MD: &metadata.RpcMetadata{
			PermAuthKeyId: 91,
		},
	}
}

func TestAuthSignUpRejectsNilRequest(t *testing.T) {
	c := newBoundaryAuthorizationCore()
	if result, err := c.AuthSignUp(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("sign up = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
}

func TestAuthSignUpRejectsUnauthenticatedRequest(t *testing.T) {
	c := newBoundaryAuthorizationCore()
	c.MD = nil
	if result, err := c.AuthSignUp(&mtproto.TLAuthSignUp{}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("sign up = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
}

func TestAuthSignUpRejectsMissingRuntimeDependencies(t *testing.T) {
	c := newBoundaryAuthorizationCore()
	if result, err := c.AuthSignUp(&mtproto.TLAuthSignUp{}); result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("sign up = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}
