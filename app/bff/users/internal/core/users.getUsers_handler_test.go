package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type getUsersUserClientStub struct {
	userclient.UserClient
	users *userpb.Vector_ImmutableUser
	err   error
}

func (s *getUsersUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, s.err
}

func newGetUsersTestCore(stub *getUsersUserClientStub) *UsersCore {
	ctx := context.Background()
	return &UsersCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func TestUsersGetUsersReturnsProviderUsers(t *testing.T) {
	users := &getUsersUserClientStub{users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42, FirstName: "self"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 43, FirstName: "peer"}}).To_ImmutableUser(),
	}}}
	core := newGetUsersTestCore(users)
	result, err := core.UsersGetUsers(&mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{
		mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 143}).To_InputUser(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.GetDatas()) != 2 || result.GetDatas()[0].GetId() != 42 || result.GetDatas()[1].GetId() != 43 {
		t.Fatalf("users.getUsers result = %v, want self and provider user 43", result)
	}
}

func TestUsersGetUsersPropagatesProviderFailures(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	core := newGetUsersTestCore(&getUsersUserClientStub{err: wantErr})
	result, err := core.UsersGetUsers(&mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 143}).To_InputUser(),
	}})
	if result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("provider failure = (%v, %v), want nil and %v", result, err, wantErr)
	}
}

func TestUsersGetUsersRejectsMalformedInput(t *testing.T) {
	core := newGetUsersTestCore(&getUsersUserClientStub{})
	for name, input := range map[string]*mtproto.TLUsersGetUsers{
		"nil request": nil,
		"nil user":    {Id: []*mtproto.InputUser{nil}},
		"empty user":  {Id: []*mtproto.InputUser{mtproto.MakeTLInputUserEmpty(nil).To_InputUser()}},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := core.UsersGetUsers(input)
			if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) && !errors.Is(err, mtproto.ErrUserIdInvalid) {
				t.Fatalf("malformed input = (%v, %v), want input/user error", result, err)
			}
		})
	}
}
