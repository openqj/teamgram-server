package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	accountdao "github.com/teamgram/teamgram-server/app/bff/account/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/logic"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/code"
	"github.com/zeromicro/go-zero/core/logx"
)

type sendChangePhoneUserClient struct {
	userclient.UserClient
	user *mtproto.ImmutableUser
	err  error
}

func (c *sendChangePhoneUserClient) UserGetImmutableUserByPhone(context.Context, *userpb.TLUserGetImmutableUserByPhone) (*mtproto.ImmutableUser, error) {
	return c.user, c.err
}

func newSendChangePhoneCodeCore(client userclient.UserClient) *AccountCore {
	ctx := context.Background()
	return &AccountCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 91, UserId: 7},
		svcCtx: &svc.ServiceContext{
			Dao:        &accountdao.Dao{UserClient: client},
			AuthLogic:  &logic.AuthLogic{},
			Challenges: &code.ChallengeService{},
		},
	}
}

func TestAccountSendChangePhoneCodeRejectsMalformedUserProviderResult(t *testing.T) {
	request := &mtproto.TLAccountSendChangePhoneCode{
		PhoneNumber: "+14155552671",
		Settings:    &mtproto.CodeSettings{},
	}

	for name, client := range map[string]userclient.UserClient{
		"nil user":        &sendChangePhoneUserClient{},
		"nil user record": &sendChangePhoneUserClient{user: &mtproto.ImmutableUser{}},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := newSendChangePhoneCodeCore(client).AccountSendChangePhoneCode(request)
			if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("send change-phone code = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
			}
		})
	}
}

func TestAccountSendChangePhoneCodePreservesOccupiedProviderResult(t *testing.T) {
	client := &sendChangePhoneUserClient{user: &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}}
	result, err := newSendChangePhoneCodeCore(client).AccountSendChangePhoneCode(&mtproto.TLAccountSendChangePhoneCode{
		PhoneNumber: "+14155552671",
		Settings:    &mtproto.CodeSettings{},
	})
	if result != nil || !errors.Is(err, mtproto.ErrPhoneNumberOccupied) {
		t.Fatalf("send change-phone code = (%v, %v), want (nil, PHONE_NUMBER_OCCUPIED)", result, err)
	}
}
