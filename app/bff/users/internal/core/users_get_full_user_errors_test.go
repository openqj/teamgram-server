package core

import (
	"context"
	"errors"
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
		{User: &mtproto.UserData{Id: 2}},
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

func (*fullUserErrorUserClient) UserGetPrivacy(context.Context, *userpb.TLUserGetPrivacy) (*userpb.Vector_PrivacyRule, error) {
	return &userpb.Vector_PrivacyRule{}, nil
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
		Id: &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: 2},
	})
	if err != wantErr {
		t.Fatalf("UsersGetFullUser() error = %v, want %v", err, wantErr)
	}
}

func TestCheckFullUserPrivacyMatchesGroupMembership(t *testing.T) {
	ctx := context.Background()
	core := &UsersCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			ChatClient: &fullUserErrorChatClient{result: &chatpb.Vector_UserChatIdList{Datas: []*chatpb.UserChatIdList{{
				UserId:     2,
				ChatIdList: []int64{42},
			}}}},
		}},
	}
	owner := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 1}}
	rules := []*mtproto.PrivacyRule{
		{PredicateName: mtproto.Predicate_privacyValueAllowContacts},
		{PredicateName: mtproto.Predicate_privacyValueAllowChatParticipants, Chats: []int64{42}},
	}

	allowed, err := core.checkFullUserPrivacy(owner, 1, 2, rules)
	if err != nil {
		t.Fatalf("checkFullUserPrivacy() error = %v", err)
	}
	if !allowed {
		t.Fatal("expected matching group member to be allowed")
	}
}

func TestCheckFullUserPrivacyDisallowsMatchingGroupMember(t *testing.T) {
	ctx := context.Background()
	core := &UsersCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			ChatClient: &fullUserErrorChatClient{result: &chatpb.Vector_UserChatIdList{Datas: []*chatpb.UserChatIdList{{
				UserId:     2,
				ChatIdList: []int64{42},
			}}}},
		}},
	}
	owner := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 1}}
	rules := []*mtproto.PrivacyRule{
		{PredicateName: mtproto.Predicate_privacyValueAllowAll},
		{PredicateName: mtproto.Predicate_privacyValueDisallowChatParticipants, Chats: []int64{42}},
	}

	allowed, err := core.checkFullUserPrivacy(owner, 1, 2, rules)
	if err != nil {
		t.Fatalf("checkFullUserPrivacy() error = %v", err)
	}
	if allowed {
		t.Fatal("expected matching group member to be disallowed")
	}
}

func TestCheckFullUserPrivacyFailsClosedForChannelRules(t *testing.T) {
	core := &UsersCore{svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ChatClient: &fullUserErrorChatClient{}}}}
	owner := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 1}}
	rules := []*mtproto.PrivacyRule{
		{PredicateName: mtproto.Predicate_privacyValueAllowContacts},
		{PredicateName: mtproto.Predicate_privacyValueAllowChatParticipants, Chats: []int64{mtproto.MinNebulaChatChannelID}},
	}

	allowed, err := core.checkFullUserPrivacy(owner, 1, 2, rules)
	if allowed || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("checkFullUserPrivacy() = (%v, %v), want (false, ErrMethodNotImpl)", allowed, err)
	}
}

func TestCheckFullUserPrivacyPropagatesMembershipQueryError(t *testing.T) {
	wantErr := errors.New("membership query failed")
	core := &UsersCore{svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ChatClient: &fullUserErrorChatClient{err: wantErr}}}}
	owner := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 1}}
	rules := []*mtproto.PrivacyRule{
		{PredicateName: mtproto.Predicate_privacyValueAllowContacts},
		{PredicateName: mtproto.Predicate_privacyValueAllowChatParticipants, Chats: []int64{42}},
	}

	allowed, err := core.checkFullUserPrivacy(owner, 1, 2, rules)
	if allowed || err != wantErr {
		t.Fatalf("checkFullUserPrivacy() = (%v, %v), want (false, %v)", allowed, err, wantErr)
	}
}
