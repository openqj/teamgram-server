package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type connectedBotsUserClient struct {
	userclient.UserClient
	profiles  map[int64]*mtproto.ImmutableUser
	requested []int64
}

func (c *connectedBotsUserClient) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	c.requested = append(c.requested[:0], in.GetId()...)
	result := make([]*mtproto.ImmutableUser, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		if profile := c.profiles[id]; profile != nil {
			result = append(result, profile)
		}
	}
	return &userpb.Vector_ImmutableUser{Datas: result}, nil
}

func connectedBotsProfile(id, accessHash int64, bot bool) *mtproto.ImmutableUser {
	data := &mtproto.UserData{Id: id, AccessHash: accessHash}
	if bot {
		data.Bot = &mtproto.BotData{}
	}
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: data}).To_ImmutableUser()
}

func connectedBotsCore(userID int64, client userclient.UserClient) *ApiFullCore {
	return &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func TestHydrateCanonicalUsersForConnectedBots(t *testing.T) {
	client := &connectedBotsUserClient{profiles: map[int64]*mtproto.ImmutableUser{
		200: connectedBotsProfile(200, 2200, true),
		300: connectedBotsProfile(300, 3300, false),
	}}
	c := connectedBotsCore(100, client)
	users, err := c.hydrateCanonicalUsers([]int64{200, 300}, 100, map[int64]struct{}{200: {}})
	if err != nil {
		t.Fatalf("hydrate users: %v", err)
	}
	if got, want := client.requested, []int64{200, 300}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("requested ids = %v, want %v", got, want)
	}
	if len(users) != 2 || users[0].GetId() != 200 || !users[0].GetBot() || users[0].GetAccessHash().GetValue() != 2200 {
		t.Fatalf("unexpected bot user: %+v", users)
	}
	if users[1].GetId() != 300 || users[1].GetBot() || users[1].GetAccessHash().GetValue() != 3300 {
		t.Fatalf("unexpected owner user: %+v", users[1])
	}
}

func TestHydrateCanonicalUsersFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		client  userclient.UserClient
		ids     []int64
		botIDs  map[int64]struct{}
		wantErr error
	}{
		{
			name:    "missing client",
			wantErr: mtproto.ErrMethodNotImpl,
		},
		{
			name: "missing profile",
			client: &connectedBotsUserClient{profiles: map[int64]*mtproto.ImmutableUser{
				200: connectedBotsProfile(200, 2200, true),
			}},
			ids:     []int64{201},
			botIDs:  map[int64]struct{}{201: {}},
			wantErr: mtproto.ErrUserIdInvalid,
		},
		{
			name: "non bot row",
			client: &connectedBotsUserClient{profiles: map[int64]*mtproto.ImmutableUser{
				200: connectedBotsProfile(200, 2200, false),
			}},
			ids:     []int64{200},
			botIDs:  map[int64]struct{}{200: {}},
			wantErr: mtproto.ErrBotInvalid,
		},
		{
			name: "zero access hash",
			client: &connectedBotsUserClient{profiles: map[int64]*mtproto.ImmutableUser{
				200: connectedBotsProfile(200, 0, true),
			}},
			ids:     []int64{200},
			botIDs:  map[int64]struct{}{200: {}},
			wantErr: mtproto.ErrUserIdInvalid,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := connectedBotsCore(100, tc.client)
			users, err := c.hydrateCanonicalUsers(tc.ids, 100, tc.botIDs)
			if users != nil || !errors.Is(err, tc.wantErr) {
				t.Fatalf("hydrate users = (%v, %v), want nil and %v", users, err, tc.wantErr)
			}
		})
	}
}

func TestValidateBotBusinessConnectionUpdates(t *testing.T) {
	connection := mtproto.MakeTLBotBusinessConnection(&mtproto.BotBusinessConnection{
		ConnectionId: "connection-42", UserId: 1001, DcId: 2, Date: 1700000000,
	}).To_BotBusinessConnection()
	update := mtproto.MakeTLUpdateBotBusinessConnect(&mtproto.Update{Connection: connection, Qts: 7}).To_Update()
	valid := mtproto.MakeUpdatesByUpdates(update)
	if ownerID, err := validateBotBusinessConnectionUpdates(valid, "connection-42"); err != nil || ownerID != 1001 {
		t.Fatalf("validate updates = (%d, %v), want owner 1001", ownerID, err)
	}

	cases := []struct {
		name string
		data *mtproto.Updates
		id   string
	}{
		{name: "wrong connection id", data: valid, id: "connection-other"},
		{name: "empty updates", data: mtproto.MakeUpdatesByUpdates(), id: "connection-42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateBotBusinessConnectionUpdates(tc.data, tc.id); !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("validate error = %v, want INTERNAL_SERVER_ERROR", err)
			}
		})
	}
}
