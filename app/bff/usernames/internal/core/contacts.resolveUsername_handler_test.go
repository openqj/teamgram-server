package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/usernames/plugin"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type resolveUsernameUserClient struct {
	user_client.UserClient
	peer         *mtproto.Peer
	err          error
	users        *userpb.Vector_ImmutableUser
	usersErr     error
	usersCalls   int
	usersRequest *userpb.TLUserGetMutableUsers
}

func (f *resolveUsernameUserClient) UserResolveUsername(context.Context, *userpb.TLUserResolveUsername) (*mtproto.Peer, error) {
	return f.peer, f.err
}

func (f *resolveUsernameUserClient) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	f.usersCalls++
	f.usersRequest = in
	return f.users, f.usersErr
}

type resolveUsernamePlugin struct {
	chats []*mtproto.Chat
}

func (f *resolveUsernamePlugin) GetChannelListByIdList(context.Context, int64, ...int64) []*mtproto.Chat {
	return f.chats
}

func newResolveUsernameTestCore(client *resolveUsernameUserClient, p plugin.UsernamesPlugin) *UsernamesCore {
	c := New(context.Background(), &svc.ServiceContext{
		Dao:    &dao.Dao{UserClient: client},
		Plugin: p,
	})
	c.MD = &metadata.RpcMetadata{UserId: 42}
	return c
}

func resolveUsernameTestUser(id int64, firstName string) *mtproto.ImmutableUser {
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: id, FirstName: firstName},
	}).To_ImmutableUser()
}

func TestContactsResolveUsernameHydratesUser(t *testing.T) {
	client := &resolveUsernameUserClient{
		peer: mtproto.MakePeerUser(77),
		users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
			resolveUsernameTestUser(42, "Requester"),
			resolveUsernameTestUser(77, "Resolved"),
		}},
	}
	c := newResolveUsernameTestCore(client, nil)

	resolved, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "resolveduser"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.GetPeer().GetUserId() != 77 || len(resolved.GetUsers()) != 1 || resolved.GetUsers()[0].GetId() != 77 {
		t.Fatalf("resolved peer did not include the hydrated user: %#v", resolved)
	}
	if client.usersCalls != 1 || client.usersRequest == nil || len(client.usersRequest.GetId()) != 2 || client.usersRequest.GetId()[0] != 42 || client.usersRequest.GetId()[1] != 77 {
		t.Fatalf("mutable user request = %#v, calls = %d", client.usersRequest, client.usersCalls)
	}
}

func TestContactsResolveUsernamePropagatesUserHydrationError(t *testing.T) {
	hydrationErr := errors.New("user service unavailable")
	client := &resolveUsernameUserClient{peer: mtproto.MakePeerUser(77), usersErr: hydrationErr}
	c := newResolveUsernameTestCore(client, nil)

	resolved, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "resolveduser"})
	if resolved != nil || !errors.Is(err, hydrationErr) {
		t.Fatalf("resolved = %#v, error = %v; want nil result and propagated %v", resolved, err, hydrationErr)
	}
}

func TestContactsResolveUsernameFailsClosedOnIncompleteUserHydration(t *testing.T) {
	for _, tt := range []struct {
		name  string
		users *userpb.Vector_ImmutableUser
	}{
		{name: "nil response"},
		{name: "empty response", users: &userpb.Vector_ImmutableUser{}},
		{name: "missing requester", users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{resolveUsernameTestUser(77, "Resolved")}}},
		{name: "missing resolved user", users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{resolveUsernameTestUser(42, "Requester")}}},
		{name: "nil user", users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{nil}}},
		{name: "missing user data", users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{{}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &resolveUsernameUserClient{peer: mtproto.MakePeerUser(77), users: tt.users}
			c := newResolveUsernameTestCore(client, nil)

			resolved, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "resolveduser"})
			if resolved != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("resolved = %#v, error = %v; want nil result and %v", resolved, err, mtproto.ErrInternalServerError)
			}
		})
	}
}

func TestContactsResolveUsernameRejectsMissingChannelWithoutPlugin(t *testing.T) {
	client := &resolveUsernameUserClient{peer: mtproto.MakePeerChannel(77)}
	c := newResolveUsernameTestCore(client, nil)

	resolved, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "publicchannel"})
	if err != mtproto.ErrChannelInvalid {
		t.Fatalf("got error %v, want %v", err, mtproto.ErrChannelInvalid)
	}
	if resolved != nil {
		t.Fatalf("got partial resolved peer %#v with an error", resolved)
	}
}

func TestContactsResolveUsernameUsesChannelLookupWithoutPlugin(t *testing.T) {
	want := mtproto.MakeTLChannel(&mtproto.Chat{Id: 77, Title: "Resolved channel"}).To_Chat()
	client := &resolveUsernameUserClient{peer: mtproto.MakePeerChannel(77)}
	c := newResolveUsernameTestCore(client, nil)
	called := false
	c.channelChatsByID = func(userID int64, ids []int64) []*mtproto.Chat {
		called = true
		if userID != 42 || len(ids) != 1 || ids[0] != 77 {
			t.Fatalf("channel lookup got user ID %d and channel IDs %v", userID, ids)
		}
		return []*mtproto.Chat{want}
	}

	resolved, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "publicchannel"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("channel lookup was not called")
	}
	if len(resolved.GetChats()) != 1 || resolved.GetChats()[0].GetId() != 77 {
		t.Fatalf("resolved chats = %#v, want exactly channel 77", resolved.GetChats())
	}
}

func TestContactsResolveUsernameRejectsMissingResolvedChannel(t *testing.T) {
	client := &resolveUsernameUserClient{peer: mtproto.MakePeerChannel(77)}
	plugin := &resolveUsernamePlugin{chats: []*mtproto.Chat{
		mtproto.MakeTLChannel(&mtproto.Chat{Id: 78}).To_Chat(),
	}}
	c := newResolveUsernameTestCore(client, plugin)

	resolved, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "publicchannel"})
	if err != mtproto.ErrChannelInvalid {
		t.Fatalf("got error %v, want %v", err, mtproto.ErrChannelInvalid)
	}
	if resolved != nil {
		t.Fatalf("got partial resolved peer %#v with an error", resolved)
	}
}

func TestContactsResolveUsernameHydratesMatchingChannel(t *testing.T) {
	want := mtproto.MakeTLChannel(&mtproto.Chat{Id: 77, Title: "Resolved channel"}).To_Chat()
	client := &resolveUsernameUserClient{peer: mtproto.MakePeerChannel(77)}
	plugin := &resolveUsernamePlugin{chats: []*mtproto.Chat{
		mtproto.MakeTLChannel(&mtproto.Chat{Id: 78}).To_Chat(),
		want,
	}}
	c := newResolveUsernameTestCore(client, plugin)

	resolved, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "publicchannel"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.GetPeer().GetChannelId() != 77 {
		t.Fatalf("resolved peer channel id = %d, want 77", resolved.GetPeer().GetChannelId())
	}
	if len(resolved.GetChats()) != 1 || resolved.GetChats()[0].GetId() != 77 {
		t.Fatalf("resolved chats = %#v, want exactly channel 77", resolved.GetChats())
	}
}
