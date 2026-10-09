package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/chats/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/chats/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type invitePrivacyUserClient struct {
	userclient.UserClient
	allowed              *mtproto.Bool
	privacyErr, usersErr error
	request              *userpb.TLUserCheckPrivacy
}

func (c *invitePrivacyUserClient) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 42}}, {User: &mtproto.UserData{Id: 7, AccessHash: 70}},
	}}, c.usersErr
}

func (c *invitePrivacyUserClient) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	c.request = in
	return c.allowed, c.privacyErr
}

func TestChatInvitesUseAuthoritativePrivacyBeforeMutation(t *testing.T) {
	wantErr := errors.New("postgres privacy unavailable")
	for _, mode := range []string{"add", "create"} {
		for _, tc := range []struct {
			name  string
			users *invitePrivacyUserClient
			want  error
		}{
			{name: "privacy denied", users: &invitePrivacyUserClient{allowed: mtproto.BoolFalse}, want: mtproto.ErrUserPrivacyRestricted},
			{name: "privacy query", users: &invitePrivacyUserClient{privacyErr: wantErr}, want: wantErr},
			{name: "users query", users: &invitePrivacyUserClient{usersErr: wantErr}, want: wantErr},
			{name: "nil privacy", users: &invitePrivacyUserClient{}, want: mtproto.ErrInternalServerError},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				ctx := context.Background()
				c := &ChatsCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: tc.users}}, MD: &metadata.RpcMetadata{UserId: 42}, Logger: logx.WithContext(ctx)}
				input := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7, AccessHash: 70}).To_InputUser()
				var err error
				if mode == "create" {
					got, callErr := c.createChat([]*mtproto.InputUser{input}, "Privacy", nil)
					err = callErr
					if got != nil {
						t.Fatalf("create result after denial = %v", got)
					}
					if errors.Is(tc.want, mtproto.ErrUserPrivacyRestricted) {
						tc.want = mtproto.ErrUsersTooFew
					}
				} else {
					got, callErr := c.addChatUser(9, input, 0)
					err = callErr
					if got != nil {
						t.Fatalf("add result after denial = %v", got)
					}
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("invite error = %v, want %v", err, tc.want)
				}
				if in := tc.users.request; in != nil && (in.GetUserId() != 7 || in.GetPeerId() != 42 || in.GetKeyType() != mtproto.CHAT_INVITE) {
					t.Fatalf("chat invite privacy request = %v", in)
				}
			})
		}
	}
}
