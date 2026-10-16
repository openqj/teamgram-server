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
	users   *userpb.Vector_ImmutableUser
	err     error
	request *userpb.TLUserGetMutableUsers
}

func (s *getUsersUserClientStub) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.request = in
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
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42, AccessHash: 142, FirstName: "self"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 43, AccessHash: 143, FirstName: "peer"}}).To_ImmutableUser(),
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
	if result.GetDatas()[0].GetAccessHash().GetValue() != 142 || result.GetDatas()[1].GetAccessHash().GetValue() != 143 {
		t.Fatalf("users.getUsers access hashes = %v, want self=142 peer=143", result.GetDatas())
	}
	if users.request == nil || len(users.request.GetId()) != 2 || users.request.GetId()[0] != 42 || users.request.GetId()[1] != 43 || len(users.request.GetTo()) != 1 || users.request.GetTo()[0] != 42 {
		t.Fatalf("provider request = %v, want ids [42 43] and to [42]", users.request)
	}
}

func TestUsersGetUsersPreservesRequestOrderAndDuplicates(t *testing.T) {
	users := &getUsersUserClientStub{users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 42, AccessHash: 142}},
		{User: &mtproto.UserData{Id: 43, AccessHash: 143}},
	}}}
	result, err := newGetUsersTestCore(users).UsersGetUsers(&mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 143}).To_InputUser(),
		mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 143}).To_InputUser(),
	}})
	if err != nil || result == nil || len(result.GetDatas()) != 3 {
		t.Fatalf("users.getUsers result = (%v, %v), want three users", result, err)
	}
	for i, want := range []int64{43, 42, 43} {
		if result.GetDatas()[i].GetId() != want {
			t.Fatalf("users.getUsers result[%d] = %d, want %d", i, result.GetDatas()[i].GetId(), want)
		}
	}
}

func TestUsersGetUsersRejectsAccessHashMismatchAndDeletedUsers(t *testing.T) {
	for name, provider := range map[string]*userpb.Vector_ImmutableUser{
		"hash mismatch": {Datas: []*mtproto.ImmutableUser{
			{User: &mtproto.UserData{Id: 42, AccessHash: 142}},
			{User: &mtproto.UserData{Id: 43, AccessHash: 143}},
		}},
		"deleted user": {Datas: []*mtproto.ImmutableUser{
			{User: &mtproto.UserData{Id: 42, AccessHash: 142}},
			{User: &mtproto.UserData{Id: 43, AccessHash: 143, Deleted: true}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			inputHash := int64(143)
			if name == "hash mismatch" {
				inputHash = 999
			}
			result, err := newGetUsersTestCore(&getUsersUserClientStub{users: provider}).UsersGetUsers(&mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{
				mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
				mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: inputHash}).To_InputUser(),
			}})
			if result != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
				t.Fatalf("users.getUsers = (%v, %v), want nil and USER_ID_INVALID", result, err)
			}
		})
	}
}

func TestUsersGetUsersRejectsMissingCanonicalUser(t *testing.T) {
	result, err := newGetUsersTestCore(&getUsersUserClientStub{users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 42, AccessHash: 142}},
	}}}).UsersGetUsers(&mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 143}).To_InputUser(),
	}})
	if result != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("missing canonical user = (%v, %v), want nil and USER_ID_INVALID", result, err)
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
		"missing access hash": {Id: []*mtproto.InputUser{
			mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43}).To_InputUser(),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := core.UsersGetUsers(input)
			if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) && !errors.Is(err, mtproto.ErrUserIdInvalid) {
				t.Fatalf("malformed input = (%v, %v), want input/user error", result, err)
			}
		})
	}
}

func TestUsersGetUsersRejectsMissingProvider(t *testing.T) {
	core := &UsersCore{MD: &metadata.RpcMetadata{UserId: 42}}
	result, err := core.UsersGetUsers(&mtproto.TLUsersGetUsers{})
	if result != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("missing provider = (%v, %v), want nil and INTERNAL_SERVER_ERROR", result, err)
	}
}
