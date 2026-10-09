package core

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	dialogpb "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetFullUserUsersFailsWhenRequiredUserIsMissing(t *testing.T) {
	tests := []struct {
		name  string
		users *mtproto.MutableUsers
	}{
		{name: "nil response"},
		{name: "missing self", users: &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{{User: &mtproto.UserData{Id: 2}}}}},
		{name: "missing peer", users: &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{{User: &mtproto.UserData{Id: 1}}}}},
		{name: "nil records are ignored", users: &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{nil, {}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := getFullUserUsers(tt.users, 1, 2)
			if err != mtproto.ErrInternalServerError {
				t.Fatalf("expected ErrInternalServerError, got %v", err)
			}
		})
	}
}

func TestGetFullUserUsersReturnsRequestedPair(t *testing.T) {
	self := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 1}}
	peer := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 2}}
	me, gotPeer, err := getFullUserUsers(&mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{self, peer}}, 1, 2)
	if err != nil {
		t.Fatalf("getFullUserUsers() error = %v", err)
	}
	if me != self || gotPeer != peer {
		t.Fatalf("unexpected pair: me=%p peer=%p", me, gotPeer)
	}
}

func TestGetFullUserUsersAllowsSelfPeer(t *testing.T) {
	self := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 1}}
	me, peer, err := getFullUserUsers(&mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{self}}, 1, 1)
	if err != nil {
		t.Fatalf("getFullUserUsers() error = %v", err)
	}
	if me != self || peer != self {
		t.Fatalf("expected self record for both users, got me=%p peer=%p", me, peer)
	}
}

type fullUserErrorUserClient struct {
	userclient.UserClient
	err error
}

func (c *fullUserErrorUserClient) UserGetMutableUsersV2(context.Context, *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 1}},
		{User: &mtproto.UserData{Id: 2, AccessHash: 200}},
	}}, nil
}

func (c *fullUserErrorUserClient) UserGetPeerSettings(context.Context, *userpb.TLUserGetPeerSettings) (*mtproto.PeerSettings, error) {
	return nil, c.err
}

func (*fullUserErrorUserClient) UserGetNotifySettings(context.Context, *userpb.TLUserGetNotifySettings) (*mtproto.PeerNotifySettings, error) {
	return &mtproto.PeerNotifySettings{}, nil
}

func (*fullUserErrorUserClient) UserBlockedByUser(context.Context, *userpb.TLUserBlockedByUser) (*mtproto.Bool, error) {
	return mtproto.BoolFalse, nil
}

func (*fullUserErrorUserClient) UserCheckPrivacy(context.Context, *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (*fullUserErrorUserClient) UserGetGlobalPrivacySettings(context.Context, *userpb.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	return &mtproto.GlobalPrivacySettings{HideReadMarks: true}, nil
}

type fullUserErrorChatClient struct {
	chatclient.ChatClient
	result *chatpb.Vector_UserChatIdList
	err    error
}

func (c *fullUserErrorChatClient) ChatGetUsersChatIdList(context.Context, *chatpb.TLChatGetUsersChatIdList) (*chatpb.Vector_UserChatIdList, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.result != nil {
		return c.result, nil
	}
	return &chatpb.Vector_UserChatIdList{}, nil
}

type fullUserErrorDialogClient struct {
	dialogclient.DialogClient
}

func (*fullUserErrorDialogClient) DialogGetDialogById(context.Context, *dialogpb.TLDialogGetDialogById) (*dialogpb.DialogExt, error) {
	return nil, nil
}

func TestUsersGetFullUserPropagatesPeerSettingsError(t *testing.T) {
	wantErr := errors.New("peer settings unavailable")
	ctx := context.Background()
	core := &UsersCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:   &fullUserErrorUserClient{err: wantErr},
			ChatClient:   &fullUserErrorChatClient{},
			DialogClient: &fullUserErrorDialogClient{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}

	_, err := core.UsersGetFullUser(&mtproto.TLUsersGetFullUser{
		Id: &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: 2, AccessHash: 200},
	})
	if err != wantErr {
		t.Fatalf("UsersGetFullUser() error = %v, want %v", err, wantErr)
	}
}

type fullUserPrivacyClient struct {
	*fullUserErrorUserClient
	allowed *mtproto.Bool
	err     error
	request *userpb.TLUserCheckPrivacy
}

func (c *fullUserPrivacyClient) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	c.request = in
	return c.allowed, c.err
}

func TestCheckFullUserPrivacyUsesAuthoritativeUserService(t *testing.T) {
	wantErr := errors.New("postgres membership unavailable")
	for _, tc := range []struct {
		name         string
		allowed      *mtproto.Bool
		err, wantErr error
		want         bool
	}{
		{name: "allowed", allowed: mtproto.BoolTrue, want: true},
		{name: "denied", allowed: mtproto.BoolFalse},
		{name: "query failure", err: wantErr, wantErr: wantErr},
		{name: "nil response", wantErr: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fullUserPrivacyClient{allowed: tc.allowed, err: tc.err}
			core := &UsersCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: 2}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}}}
			got, err := core.checkFullUserPrivacy(1, mtproto.BIRTHDAY)
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Fatalf("privacy = (%v, %v), want (%v, %v)", got, err, tc.want, tc.wantErr)
			}
			if in := client.request; in.GetUserId() != 1 || in.GetPeerId() != 2 || in.GetKeyType() != mtproto.BIRTHDAY {
				t.Fatalf("privacy request = %v", in)
			}
		})
	}
}

type fullUserProfilePrivacyClient struct {
	*fullUserErrorUserClient
	allowed      map[int32]bool
	failKey      int32
	privacyErr   error
	nilPrivacy   bool
	mu           sync.Mutex
	requests     map[int32]*userpb.TLUserCheckPrivacy
	usersRequest *userpb.TLUserGetMutableUsersV2
}

func (c *fullUserProfilePrivacyClient) UserGetMutableUsersV2(_ context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	c.usersRequest = in
	return &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 1}},
		{User: &mtproto.UserData{Id: 2, AccessHash: 200, About: mtproto.MakeFlagsString("Private bio"),
			ProfilePhoto: &mtproto.Photo{Id: 20}, SavedMusic: &mtproto.Document{Id: 30}, Birthday: "0000-01-01"}},
	}}, nil
}

func (*fullUserProfilePrivacyClient) UserGetPeerSettings(context.Context, *userpb.TLUserGetPeerSettings) (*mtproto.PeerSettings, error) {
	return &mtproto.PeerSettings{}, nil
}

func (c *fullUserProfilePrivacyClient) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	c.mu.Lock()
	c.requests[in.GetKeyType()] = in
	c.mu.Unlock()
	if in.GetKeyType() == c.failKey {
		if c.nilPrivacy {
			return nil, nil
		}
		return nil, c.privacyErr
	}
	return mtproto.ToBool(c.allowed[in.GetKeyType()]), nil
}

func newFullUserProfilePrivacyCore(client *fullUserProfilePrivacyClient) *UsersCore {
	ctx := context.Background()
	client.fullUserErrorUserClient = &fullUserErrorUserClient{}
	client.requests = make(map[int32]*userpb.TLUserCheckPrivacy)
	return &UsersCore{ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: 1}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
		UserClient: client, ChatClient: &fullUserErrorChatClient{}, DialogClient: &fullUserErrorDialogClient{},
	}}}
}

func TestUsersGetFullUserHonorsEachProfilePrivacyKey(t *testing.T) {
	keys := []int32{mtproto.VOICE_MESSAGES, mtproto.ABOUT, mtproto.PROFILE_PHOTO, mtproto.SAVED_MUSIC, mtproto.BIRTHDAY}
	for _, key := range keys {
		client := &fullUserProfilePrivacyClient{allowed: map[int32]bool{key: true}}
		got, err := newFullUserProfilePrivacyCore(client).UsersGetFullUser(&mtproto.TLUsersGetFullUser{Id: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 200}).To_InputUser()})
		if err != nil || got.GetFullUser() == nil || !got.GetFullUser().GetReadDatesPrivate() {
			t.Fatalf("full user for key %d = (%v, %v)", key, got, err)
		}
		full := got.GetFullUser()
		for fieldKey, visible := range map[int32]bool{
			mtproto.VOICE_MESSAGES: !full.GetVoiceMessagesForbidden(), mtproto.ABOUT: full.GetAbout() != nil,
			mtproto.PROFILE_PHOTO: full.GetProfilePhoto() != nil, mtproto.SAVED_MUSIC: full.GetSavedMusic() != nil, mtproto.BIRTHDAY: full.GetBirthday() != nil,
		} {
			if visible != (fieldKey == key) {
				t.Fatalf("full user key %d field %d visible=%v", key, fieldKey, visible)
			}
		}
		client.mu.Lock()
		for _, fieldKey := range keys {
			in := client.requests[fieldKey]
			if in == nil || in.GetUserId() != 2 || in.GetPeerId() != 1 || in.GetKeyType() != fieldKey {
				t.Errorf("full user key %d request = %v", fieldKey, in)
			}
		}
		client.mu.Unlock()
		if in := client.usersRequest; !in.GetPrivacy() || !in.GetHasTo() || len(in.GetTo()) != 2 {
			t.Fatalf("full user snapshot request = %v", in)
		}
	}
}

func TestUsersGetFullUserPropagatesEveryPrivacyQueryFailure(t *testing.T) {
	wantErr := errors.New("postgres privacy query failed")
	for _, key := range []int32{mtproto.VOICE_MESSAGES, mtproto.ABOUT, mtproto.PROFILE_PHOTO, mtproto.SAVED_MUSIC, mtproto.BIRTHDAY} {
		for _, nilResult := range []bool{false, true} {
			client := &fullUserProfilePrivacyClient{failKey: key, nilPrivacy: nilResult, privacyErr: wantErr}
			got, err := newFullUserProfilePrivacyCore(client).UsersGetFullUser(&mtproto.TLUsersGetFullUser{Id: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 200}).To_InputUser()})
			want := wantErr
			if nilResult {
				want = mtproto.ErrInternalServerError
			}
			if got != nil || !errors.Is(err, want) {
				t.Fatalf("full user key %d nil=%v = (%v, %v), want error %v", key, nilResult, got, err, want)
			}
		}
	}
}
