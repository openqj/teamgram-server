package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type signupCleanupUserClient struct {
	userclient.UserClient
	deleted *userpb.TLUserDeleteUser
}

func (s *signupCleanupUserClient) UserDeleteUser(_ context.Context, in *userpb.TLUserDeleteUser) (*mtproto.Bool, error) {
	s.deleted = in
	return mtproto.BoolTrue, nil
}

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

func TestAuthSignUpCleanupCompensatesUnboundUser(t *testing.T) {
	users := &signupCleanupUserClient{}
	c := newBoundaryAuthorizationCore()
	c.svcCtx = &svc.ServiceContext{Dao: &dao.Dao{UserClient: users}}
	c.cleanupSignupUser(801, "+15550000001")
	if users.deleted == nil || users.deleted.GetUserId() != 801 || users.deleted.GetPhone() != "+15550000001" {
		t.Fatalf("cleanup request = %v, want user 801 and phone", users.deleted)
	}
}

func TestAuthSignInRejectsMissingRuntimeContext(t *testing.T) {
	var nilCore *AuthorizationCore
	if result, err := nilCore.AuthSignIn(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("nil sign in core = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	c := newBoundaryAuthorizationCore()
	if result, err := c.AuthSignIn(&mtproto.TLAuthSignIn{}); result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing sign-in runtime = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}
