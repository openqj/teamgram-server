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

type authorizationTTLUserClientStub struct {
	userclient.UserClient
	err     error
	result  *mtproto.Bool
	request *userpb.TLUserSetAuthorizationTTL
}

func (s *authorizationTTLUserClientStub) UserSetAuthorizationTTL(_ context.Context, in *userpb.TLUserSetAuthorizationTTL) (*mtproto.Bool, error) {
	s.request = in
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return mtproto.BoolTrue, nil
}

func newAuthorizationTTLTestCore(stub *authorizationTTLUserClientStub) *AuthorizationCore {
	return &AuthorizationCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}},
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 901001, UserId: 229001},
		Logger: logx.WithContext(context.Background()),
	}
}

func TestAccountSetAuthorizationTTLRejectsInvalidBoundary(t *testing.T) {
	tests := []struct {
		name string
		core *AuthorizationCore
		in   *mtproto.TLAccountSetAuthorizationTTL
		err  error
	}{
		{name: "nil core", core: nil, in: &mtproto.TLAccountSetAuthorizationTTL{}, err: mtproto.ErrAuthKeyUnregistered},
		{name: "missing metadata", core: &AuthorizationCore{}, in: &mtproto.TLAccountSetAuthorizationTTL{}, err: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: newAuthorizationTTLTestCore(&authorizationTTLUserClientStub{}), err: mtproto.ErrInputRequestInvalid},
		{name: "missing user client", core: &AuthorizationCore{
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}},
			MD:     &metadata.RpcMetadata{PermAuthKeyId: 901001, UserId: 229001},
			Logger: logx.WithContext(context.Background()),
		}, in: &mtproto.TLAccountSetAuthorizationTTL{AuthorizationTtlDays: 30}, err: mtproto.ErrInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.core.AccountSetAuthorizationTTL(tt.in)
			if result != nil || !errors.Is(err, tt.err) {
				t.Fatalf("result=(%v,%v), want nil and %v", result, err, tt.err)
			}
		})
	}
}

func TestAccountSetAuthorizationTTLRejectsNilProviderResult(t *testing.T) {
	core := newAuthorizationTTLTestCore(&authorizationTTLUserClientStub{result: nil})
	// The default stub result is BoolTrue; use an explicit implementation that
	// returns nil to exercise the service boundary.
	core.svcCtx.Dao.UserClient = nilResultAuthorizationTTLClient{}
	result, err := core.AccountSetAuthorizationTTL(&mtproto.TLAccountSetAuthorizationTTL{AuthorizationTtlDays: 30})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil provider result = (%v,%v), want internal error", result, err)
	}
}

type nilResultAuthorizationTTLClient struct {
	userclient.UserClient
}

func (nilResultAuthorizationTTLClient) UserSetAuthorizationTTL(context.Context, *userpb.TLUserSetAuthorizationTTL) (*mtproto.Bool, error) {
	return nil, nil
}

func TestAccountSetAuthorizationTTLRejectsInvalidDays(t *testing.T) {
	core := newAuthorizationTTLTestCore(&authorizationTTLUserClientStub{})
	result, err := core.AccountSetAuthorizationTTL(&mtproto.TLAccountSetAuthorizationTTL{AuthorizationTtlDays: 31})
	if result != nil || err != mtproto.ErrTtlDaysInvalid {
		t.Fatalf("invalid authorization TTL = (%v, %v), want TTL_DAYS_INVALID", result, err)
	}
}

func TestAccountSetAuthorizationTTLPropagatesWriteFailure(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	stub := &authorizationTTLUserClientStub{err: wantErr}
	core := newAuthorizationTTLTestCore(stub)
	result, err := core.AccountSetAuthorizationTTL(&mtproto.TLAccountSetAuthorizationTTL{AuthorizationTtlDays: 30})
	if result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("set authorization TTL = (%v, %v), want nil and %v", result, err, wantErr)
	}
	if stub.request == nil || stub.request.GetUserId() != 229001 || stub.request.GetTtl() != 30 {
		t.Fatalf("provider request = %v, want user 229001 and TTL 30", stub.request)
	}
}
