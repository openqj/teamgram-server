package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type profileUserClient struct {
	userclient.UserClient
	me           *mtproto.ImmutableUser
	aboutRequest *userpb.TLUserUpdateAbout
	nameRequest  *userpb.TLUserUpdateFirstAndLastName
	tabRequest   *userpb.TLUserSetMainProfileTab
	tabResult    *mtproto.Bool
	tabErr       error
	aboutErr     error
	nameErr      error
}

func (c *profileUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.me, nil
}

func (c *profileUserClient) UserUpdateAbout(_ context.Context, in *userpb.TLUserUpdateAbout) (*mtproto.Bool, error) {
	c.aboutRequest = in
	return mtproto.BoolTrue, c.aboutErr
}

func (c *profileUserClient) UserUpdateFirstAndLastName(_ context.Context, in *userpb.TLUserUpdateFirstAndLastName) (*mtproto.Bool, error) {
	c.nameRequest = in
	return mtproto.BoolTrue, c.nameErr
}

func (c *profileUserClient) UserSetMainProfileTab(_ context.Context, in *userpb.TLUserSetMainProfileTab) (*mtproto.Bool, error) {
	c.tabRequest = in
	if c.tabResult == nil {
		return mtproto.BoolTrue, c.tabErr
	}
	return c.tabResult, c.tabErr
}

type profileSyncClient struct {
	syncclient.SyncClient
	request *sync.TLSyncUpdatesNotMe
	err     error
}

func (c *profileSyncClient) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	c.request = in
	return mtproto.EmptyVoid, c.err
}

func newProfileCore(users *profileUserClient, syncer *profileSyncClient) *UserChannelProfilesCore {
	return &UserChannelProfilesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
			SyncClient: syncer,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 9001},
	}
}

func profileUser() *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{
		Id:        42,
		FirstName: "Alice",
		LastName:  "Smith",
		Username:  "alice",
		About:     mtproto.MakeFlagsString("old bio"),
	}}
}

func TestAccountUpdateProfilePreservesOmittedNameFields(t *testing.T) {
	users := &profileUserClient{me: profileUser()}
	syncer := &profileSyncClient{}
	core := newProfileCore(users, syncer)

	got, err := core.AccountUpdateProfile(&mtproto.TLAccountUpdateProfile{About: mtproto.MakeFlagsString("new bio")})
	if err != nil {
		t.Fatalf("AccountUpdateProfile() error = %v", err)
	}
	if got == nil || got.GetFirstName().GetValue() != "Alice" || got.GetLastName().GetValue() != "Smith" {
		t.Fatalf("name changed by about-only request: %v", got)
	}
	if users.nameRequest != nil || syncer.request != nil {
		t.Fatalf("about-only request unexpectedly changed name or pushed update: name=%v sync=%v", users.nameRequest, syncer.request)
	}
	if users.aboutRequest == nil || users.aboutRequest.GetAbout() != "new bio" {
		t.Fatalf("about request = %v, want new bio", users.aboutRequest)
	}
}

func TestAccountUpdateProfileMapsOnlyPresentNameFields(t *testing.T) {
	users := &profileUserClient{me: profileUser()}
	syncer := &profileSyncClient{}
	core := newProfileCore(users, syncer)

	got, err := core.AccountUpdateProfile(&mtproto.TLAccountUpdateProfile{FirstName: mtproto.MakeFlagsString("Alicia")})
	if err != nil {
		t.Fatalf("AccountUpdateProfile() error = %v", err)
	}
	if got == nil || got.GetFirstName().GetValue() != "Alicia" || got.GetLastName().GetValue() != "Smith" {
		t.Fatalf("updated user = %v, want Alicia Smith", got)
	}
	if users.nameRequest == nil || users.nameRequest.GetFirstName() != "Alicia" || users.nameRequest.GetLastName() != "Smith" {
		t.Fatalf("name request = %v, want Alicia Smith", users.nameRequest)
	}
	if syncer.request == nil || syncer.request.GetUserId() != 42 || syncer.request.GetPermAuthKeyId() != 9001 {
		t.Fatalf("sync request = %v, want user 42/auth key 9001", syncer.request)
	}
}

func TestAccountUpdateProfileValidatesContextAndDependencies(t *testing.T) {
	users := &profileUserClient{me: profileUser()}
	syncer := &profileSyncClient{}
	for name, core := range map[string]*UserChannelProfilesCore{
		"nil core": nil,
		"missing metadata": {
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users, SyncClient: syncer}},
		},
		"missing user": {
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users, SyncClient: syncer}},
			MD:     &metadata.RpcMetadata{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := core.AccountUpdateProfile(&mtproto.TLAccountUpdateProfile{}); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("result=(%v,%v), want auth error", got, err)
			}
		})
	}

	core := newProfileCore(users, syncer)
	if got, err := core.AccountUpdateProfile(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request result=(%v,%v), want input error", got, err)
	}
	for name, svcCtx := range map[string]*svc.ServiceContext{
		"missing dao":         nil,
		"missing user client": {Dao: &dao.Dao{SyncClient: syncer}},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := newProfileCore(users, syncer)
			candidate.svcCtx = svcCtx
			if got, err := candidate.AccountUpdateProfile(&mtproto.TLAccountUpdateProfile{}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("result=(%v,%v), want internal error", got, err)
			}
		})
	}
}

func TestAccountSetMainProfileTabValidatesAndPropagatesReply(t *testing.T) {
	users := &profileUserClient{me: profileUser(), tabResult: mtproto.BoolFalse}
	core := newProfileCore(users, &profileSyncClient{})
	tab := mtproto.MakeTLProfileTabPosts(nil).To_ProfileTab()
	got, err := core.AccountSetMainProfileTab(&mtproto.TLAccountSetMainProfileTab{Tab: tab})
	if err != nil || got != mtproto.BoolFalse {
		t.Fatalf("AccountSetMainProfileTab() = (%v,%v), want BoolFalse,nil", got, err)
	}
	if users.tabRequest == nil || users.tabRequest.GetUserId() != 42 || users.tabRequest.GetTab() == nil {
		t.Fatalf("backend request = %v, want user 42 and tab", users.tabRequest)
	}

	for name, request := range map[string]*mtproto.TLAccountSetMainProfileTab{
		"nil":         nil,
		"missing tab": {},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := core.AccountSetMainProfileTab(request); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("result=(%v,%v), want input error", got, err)
			}
		})
	}

	users.tabResult = nil
	users.tabErr = errors.New("backend unavailable")
	if got, err := core.AccountSetMainProfileTab(&mtproto.TLAccountSetMainProfileTab{Tab: tab}); got != nil || !errors.Is(err, users.tabErr) {
		t.Fatalf("backend error result=(%v,%v), want backend error", got, err)
	}
}
