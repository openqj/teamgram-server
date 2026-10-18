package core

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	accountdao "github.com/teamgram/teamgram-server/app/bff/account/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type accountTTLUserClientStub struct {
	userclient.UserClient
	result  *mtproto.AccountDaysTTL
	err     error
	request *userpb.TLUserGetAccountDaysTTL
}

func (s *accountTTLUserClientStub) UserGetAccountDaysTTL(_ context.Context, in *userpb.TLUserGetAccountDaysTTL) (*mtproto.AccountDaysTTL, error) {
	s.request = in
	return s.result, s.err
}

func newAccountTTLTestCore(stub userclient.UserClient) *AccountCore {
	ctx := context.Background()
	return &AccountCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 91, UserId: 7},
		svcCtx: &svc.ServiceContext{Dao: &accountdao.Dao{UserClient: stub}},
	}
}

func TestAccountGetAccountTTLRejectsMalformedProviderDays(t *testing.T) {
	for _, days := range []int32{0, 31, -1} {
		t.Run(fmt.Sprintf("days_%d", days), func(t *testing.T) {
			stub := &accountTTLUserClientStub{result: &mtproto.AccountDaysTTL{Days: days}}
			result, err := newAccountTTLTestCore(stub).AccountGetAccountTTL(&mtproto.TLAccountGetAccountTTL{})
			if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("get account TTL days=%d = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", days, result, err)
			}
		})
	}
}

func TestAccountGetAccountTTLReturnsValidatedProviderDays(t *testing.T) {
	stub := &accountTTLUserClientStub{result: &mtproto.AccountDaysTTL{Days: 365}}
	result, err := newAccountTTLTestCore(stub).AccountGetAccountTTL(&mtproto.TLAccountGetAccountTTL{})
	if err != nil || result == nil || result.GetDays() != 365 {
		t.Fatalf("get account TTL = (%v, %v), want AccountDaysTTL(365)", result, err)
	}
	if stub.request == nil || stub.request.GetUserId() != 7 {
		t.Fatalf("provider request = %v, want user 7", stub.request)
	}
}
