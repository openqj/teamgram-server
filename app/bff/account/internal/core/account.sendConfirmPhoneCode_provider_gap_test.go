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

type sendConfirmPhoneUserClient struct {
	userclient.UserClient
	user *mtproto.ImmutableUser
	err  error
}

func (c *sendConfirmPhoneUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.user, c.err
}

func newSendConfirmPhoneCodeCore(client userclient.UserClient) *AccountCore {
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

func TestAccountSendConfirmPhoneCodeRejectsMalformedUserProviderResult(t *testing.T) {
	request := &mtproto.TLAccountSendConfirmPhoneCode{
		Hash:     "purpose-hash",
		Settings: &mtproto.CodeSettings{},
	}

	for name, client := range map[string]userclient.UserClient{
		"nil user":        &sendConfirmPhoneUserClient{},
		"nil user record": &sendConfirmPhoneUserClient{user: &mtproto.ImmutableUser{}},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := newSendConfirmPhoneCodeCore(client).AccountSendConfirmPhoneCode(request)
			if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("send confirm-phone code = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
			}
		})
	}
}

func TestAccountSendConfirmPhoneCodeRejectsMismatchedUserProviderResult(t *testing.T) {
	client := &sendConfirmPhoneUserClient{user: &mtproto.ImmutableUser{User: &mtproto.UserData{
		Id: 8, Phone: "+14155552671",
	}}}
	result, err := newSendConfirmPhoneCodeCore(client).AccountSendConfirmPhoneCode(&mtproto.TLAccountSendConfirmPhoneCode{
		Hash:     "purpose-hash",
		Settings: &mtproto.CodeSettings{},
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("send confirm-phone code = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
}

func TestAccountSendConfirmPhoneCodeRejectsUserWithoutPhone(t *testing.T) {
	client := &sendConfirmPhoneUserClient{user: &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 7}}}
	result, err := newSendConfirmPhoneCodeCore(client).AccountSendConfirmPhoneCode(&mtproto.TLAccountSendConfirmPhoneCode{
		Hash:     "purpose-hash",
		Settings: &mtproto.CodeSettings{},
	})
	if result != nil || !errors.Is(err, mtproto.ErrPhoneNumberInvalid) {
		t.Fatalf("send confirm-phone code = (%v, %v), want (nil, PHONE_NUMBER_INVALID)", result, err)
	}
}
