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
	request *userpb.TLUserSetAuthorizationTTL
}

func (s *authorizationTTLUserClientStub) UserSetAuthorizationTTL(_ context.Context, in *userpb.TLUserSetAuthorizationTTL) (*mtproto.Bool, error) {
	s.request = in
	if s.err != nil {
		return nil, s.err
	}
	return mtproto.BoolTrue, nil
}

func newAuthorizationTTLTestCore(stub *authorizationTTLUserClientStub) *AuthorizationCore {
	return &AuthorizationCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}},
		MD:     &metadata.RpcMetadata{UserId: 229001},
		Logger: logx.WithContext(context.Background()),
	}
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
