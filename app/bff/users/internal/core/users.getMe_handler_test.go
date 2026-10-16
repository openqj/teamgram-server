package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type getMeUserClientStub struct {
	userclient.UserClient
	user    *mtproto.ImmutableUser
	err     error
	request *userpb.TLUserGetImmutableUserByToken
}

func (s *getMeUserClientStub) UserGetImmutableUserByToken(_ context.Context, in *userpb.TLUserGetImmutableUserByToken) (*mtproto.ImmutableUser, error) {
	s.request = in
	return s.user, s.err
}

func newGetMeTestCore(client userclient.UserClient) *UsersCore {
	ctx := context.Background()
	return &UsersCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
	}
}

func TestUsersGetMeReturnsTokenUser(t *testing.T) {
	client := &getMeUserClientStub{user: &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42, FirstName: "bot"}}}
	core := newGetMeTestCore(client)

	result, err := core.UsersGetMe(&mtproto.TLUsersGetMe{Id: 42, Token: "token"})
	if err != nil || result == nil || result.GetId() != 42 {
		t.Fatalf("users.getMe = (%v, %v), want user 42", result, err)
	}
	if client.request == nil || client.request.GetToken() != "token" {
		t.Fatalf("provider request = %v, want token", client.request)
	}
}

func TestUsersGetMeRejectsMalformedProviderUser(t *testing.T) {
	for name, user := range map[string]*mtproto.ImmutableUser{
		"nil user":        nil,
		"nil user record": {},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := newGetMeTestCore(&getMeUserClientStub{user: user}).UsersGetMe(&mtproto.TLUsersGetMe{Id: 42, Token: "token"})
			if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("malformed provider user = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
			}
		})
	}
}

func TestUsersGetMePreservesTokenMismatchError(t *testing.T) {
	result, err := newGetMeTestCore(&getMeUserClientStub{user: &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}}).UsersGetMe(&mtproto.TLUsersGetMe{Id: 43, Token: "token"})
	if result != nil || !errors.Is(err, mtproto.ErrTokenInvalid) {
		t.Fatalf("token mismatch = (%v, %v), want (nil, TOKEN_INVALID)", result, err)
	}
}

func TestUsersGetMeRejectsMalformedRequestAndDependencies(t *testing.T) {
	var nilCore *UsersCore
	if result, err := nilCore.UsersGetMe(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil core/request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}

	if result, err := newGetMeTestCore(nil).UsersGetMe(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}

	core := &UsersCore{}
	if result, err := core.UsersGetMe(&mtproto.TLUsersGetMe{Id: 42, Token: "token"}); result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing provider = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
}

func TestUsersGetMePropagatesProviderError(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	result, err := newGetMeTestCore(&getMeUserClientStub{err: wantErr}).UsersGetMe(&mtproto.TLUsersGetMe{Id: 42, Token: "token"})
	if result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("provider error = (%v, %v), want nil and %v", result, err, wantErr)
	}
}
