package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/svc"
	notificationplugin "github.com/teamgram/teamgram-server/app/bff/notification/plugin"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type notifyExceptionsUserClientStub struct {
	userclient.UserClient
	settings    *userpb.Vector_PeerPeerNotifySettings
	settingsErr error
	users       *userpb.Vector_ImmutableUser
	usersErr    error
}

func (s *notifyExceptionsUserClientStub) UserGetAllNotifySettings(context.Context, *userpb.TLUserGetAllNotifySettings) (*userpb.Vector_PeerPeerNotifySettings, error) {
	return s.settings, s.settingsErr
}

func (s *notifyExceptionsUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, s.usersErr
}

type notifyExceptionsChatClientStub struct {
	chatclient.ChatClient
	chats    *chatpb.Vector_MutableChat
	chatsErr error
}

func (s *notifyExceptionsChatClientStub) ChatGetChatListByIdList(context.Context, *chatpb.TLChatGetChatListByIdList) (*chatpb.Vector_MutableChat, error) {
	return s.chats, s.chatsErr
}

type notifyExceptionsPluginStub struct {
	channels []*mtproto.Chat
}

func (s *notifyExceptionsPluginStub) GetChannelListByIdList(context.Context, int64, ...int64) []*mtproto.Chat {
	return s.channels
}

func (s *notifyExceptionsPluginStub) GetChannelById(context.Context, int64, int64) (*mtproto.Chat, error) {
	return nil, errors.New("not used")
}

func newNotifyExceptionsCore(userStub *notifyExceptionsUserClientStub, chatStub *notifyExceptionsChatClientStub, plugin *notifyExceptionsPluginStub) *NotificationCore {
	var channelResolver notificationplugin.NotificationPlugin
	if plugin != nil {
		channelResolver = plugin
	}
	svcCtx := &svc.ServiceContext{
		Dao: &dao.Dao{
			UserClient: userStub,
			ChatClient: chatStub,
		},
		Plugin: channelResolver,
	}
	c := New(context.Background(), svcCtx)
	c.MD = &metadata.RpcMetadata{UserId: 1}
	return c
}

func notifyExceptionsSettings(peerType int32, peerID int64) *userpb.Vector_PeerPeerNotifySettings {
	return &userpb.Vector_PeerPeerNotifySettings{
		Datas: []*userpb.PeerPeerNotifySettings{{
			PeerType: peerType,
			PeerId:   peerID,
		}},
	}
}

func notifyExceptionsImmutableUser(id int64) *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id}}
}

func TestAccountGetNotifyExceptionsFailsClosedWithoutChannelResolver(t *testing.T) {
	userStub := &notifyExceptionsUserClientStub{
		settings: notifyExceptionsSettings(mtproto.PEER_CHANNEL, 42),
	}
	c := newNotifyExceptionsCore(userStub, &notifyExceptionsChatClientStub{}, nil)

	updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
	if err == nil {
		t.Fatal("expected channel hydration error when Plugin is nil")
	}
	if updates != nil {
		t.Fatalf("expected no partial Updates, got: %v", updates)
	}
}

func TestAccountGetNotifyExceptionsRejectsNilSettingsResponse(t *testing.T) {
	userStub := &notifyExceptionsUserClientStub{}
	c := newNotifyExceptionsCore(userStub, &notifyExceptionsChatClientStub{}, nil)

	updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
	if err == nil || updates != nil {
		t.Fatalf("AccountGetNotifyExceptions() = (%v, %v), want fail-closed nil response error", updates, err)
	}
}

func TestAccountGetNotifyExceptionsSkipsGlobalSettings(t *testing.T) {
	userStub := &notifyExceptionsUserClientStub{
		settings: &userpb.Vector_PeerPeerNotifySettings{
			Datas: []*userpb.PeerPeerNotifySettings{
				{PeerType: mtproto.PEER_USERS, PeerId: 0},
				{PeerType: mtproto.PEER_CHATS, PeerId: 0},
				{PeerType: mtproto.PEER_BROADCASTS, PeerId: 0},
			},
		},
	}
	c := newNotifyExceptionsCore(userStub, &notifyExceptionsChatClientStub{}, nil)

	updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
	if err != nil {
		t.Fatalf("global notification settings must not be treated as peer exceptions: %v", err)
	}
	if len(updates.GetUpdates()) != 0 || len(updates.GetUsers()) != 0 || len(updates.GetChats()) != 0 {
		t.Fatalf("global settings must not be returned as exceptions, got: %v", updates)
	}
}

func TestAccountGetNotifyExceptionsPropagatesUserHydrationError(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	userStub := &notifyExceptionsUserClientStub{
		settings: notifyExceptionsSettings(mtproto.PEER_USER, 2),
		usersErr: wantErr,
	}
	c := newNotifyExceptionsCore(userStub, &notifyExceptionsChatClientStub{}, nil)

	updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped user hydration error %v, got %v", wantErr, err)
	}
	if updates != nil {
		t.Fatalf("expected no partial Updates, got: %v", updates)
	}
}

func TestAccountGetNotifyExceptionsPropagatesBasicChatHydrationError(t *testing.T) {
	wantErr := errors.New("chat service unavailable")
	userStub := &notifyExceptionsUserClientStub{
		settings: notifyExceptionsSettings(mtproto.PEER_CHAT, 9),
	}
	chatStub := &notifyExceptionsChatClientStub{chatsErr: wantErr}
	c := newNotifyExceptionsCore(userStub, chatStub, nil)

	updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped chat hydration error %v, got %v", wantErr, err)
	}
	if updates != nil {
		t.Fatalf("expected no partial Updates, got: %v", updates)
	}
}

func TestAccountGetNotifyExceptionsIncludesResolvedChannel(t *testing.T) {
	userStub := &notifyExceptionsUserClientStub{
		settings: notifyExceptionsSettings(mtproto.PEER_CHANNEL, 42),
	}
	plugin := &notifyExceptionsPluginStub{
		channels: []*mtproto.Chat{{PredicateName: mtproto.Predicate_channel, Id: 42}},
	}
	c := newNotifyExceptionsCore(userStub, &notifyExceptionsChatClientStub{}, plugin)

	updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
	if err != nil {
		t.Fatalf("AccountGetNotifyExceptions returned error: %v", err)
	}
	if len(updates.GetChats()) != 1 || updates.GetChats()[0].GetId() != 42 {
		t.Fatalf("expected channel 42 in Updates, got chats: %v", updates.GetChats())
	}
}

func TestAccountGetNotifyExceptionsRejectsIncompleteUserHydration(t *testing.T) {
	tests := []struct {
		name  string
		users *userpb.Vector_ImmutableUser
	}{
		{name: "nil response"},
		{
			name: "missing requested user",
			users: &userpb.Vector_ImmutableUser{
				Datas: []*mtproto.ImmutableUser{notifyExceptionsImmutableUser(1)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userStub := &notifyExceptionsUserClientStub{
				settings: notifyExceptionsSettings(mtproto.PEER_USER, 2),
				users:    tt.users,
			}
			c := newNotifyExceptionsCore(userStub, &notifyExceptionsChatClientStub{}, nil)

			updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
			if err == nil {
				t.Fatal("expected incomplete user hydration error")
			}
			if updates != nil {
				t.Fatalf("expected no partial Updates, got: %v", updates)
			}
		})
	}
}

func TestAccountGetNotifyExceptionsRejectsIncompleteBasicChatHydration(t *testing.T) {
	tests := []struct {
		name  string
		chats *chatpb.Vector_MutableChat
	}{
		{name: "nil response"},
		{name: "missing requested chat", chats: &chatpb.Vector_MutableChat{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userStub := &notifyExceptionsUserClientStub{
				settings: notifyExceptionsSettings(mtproto.PEER_CHAT, 9),
			}
			chatStub := &notifyExceptionsChatClientStub{chats: tt.chats}
			c := newNotifyExceptionsCore(userStub, chatStub, nil)

			updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
			if err == nil {
				t.Fatal("expected incomplete basic chat hydration error")
			}
			if updates != nil {
				t.Fatalf("expected no partial Updates, got: %v", updates)
			}
		})
	}
}

func TestAccountGetNotifyExceptionsRequiresExactChannelSet(t *testing.T) {
	tests := []struct {
		name     string
		channels []*mtproto.Chat
	}{
		{name: "missing requested channel"},
		{
			name: "unexpected channel",
			channels: []*mtproto.Chat{
				{PredicateName: mtproto.Predicate_channel, Id: 42},
				{PredicateName: mtproto.Predicate_channel, Id: 43},
			},
		},
		{
			name: "duplicate requested channel",
			channels: []*mtproto.Chat{
				{PredicateName: mtproto.Predicate_channel, Id: 42},
				{PredicateName: mtproto.Predicate_channel, Id: 42},
			},
		},
		{
			name: "wrong constructor",
			channels: []*mtproto.Chat{
				mtproto.MakeTLChatEmpty(&mtproto.Chat{Id: 42}).To_Chat(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userStub := &notifyExceptionsUserClientStub{
				settings: notifyExceptionsSettings(mtproto.PEER_CHANNEL, 42),
			}
			plugin := &notifyExceptionsPluginStub{channels: tt.channels}
			c := newNotifyExceptionsCore(userStub, &notifyExceptionsChatClientStub{}, plugin)

			updates, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
			if err == nil {
				t.Fatal("expected incomplete or unexpected channel hydration error")
			}
			if updates != nil {
				t.Fatalf("expected no partial Updates, got: %v", updates)
			}
		})
	}
}
