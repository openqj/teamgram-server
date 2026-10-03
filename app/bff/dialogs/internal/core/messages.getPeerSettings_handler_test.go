package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type peerSettingsUserClientStub struct {
	userclient.UserClient
	settings    *mtproto.PeerSettings
	settingsErr error
	users       *userpb.Vector_ImmutableUser
	usersErr    error
}

func (s *peerSettingsUserClientStub) UserGetPeerSettings(context.Context, *userpb.TLUserGetPeerSettings) (*mtproto.PeerSettings, error) {
	return s.settings, s.settingsErr
}

func (s *peerSettingsUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, s.usersErr
}

type peerSettingsChatClientStub struct {
	chatclient.ChatClient
	chat    *mtproto.MutableChat
	err     error
	request *chatpb.TLChatGetMutableChat
}

func (s *peerSettingsChatClientStub) ChatGetMutableChat(_ context.Context, in *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	s.request = in
	return s.chat, s.err
}

type peerSettingsPluginStub struct {
	channels []*mtproto.Chat
}

func (s *peerSettingsPluginStub) GetChannelListByIdList(context.Context, int64, ...int64) []*mtproto.Chat {
	return s.channels
}

func (s *peerSettingsPluginStub) GetChannelDialogById(context.Context, int64, int64) (*dialog.DialogExt, error) {
	return nil, errors.New("not used")
}

func (s *peerSettingsPluginStub) GetChannelMessage(context.Context, int64, int64, int32) (*mtproto.MessageBox, error) {
	return nil, errors.New("not used")
}

func (s *peerSettingsPluginStub) GetChannelTypingRecipients(context.Context, int64, *mtproto.InputPeer) ([]int64, error) {
	return nil, errors.New("not used")
}

func newPeerSettingsTestCore(user *peerSettingsUserClientStub, chat *peerSettingsChatClientStub, plugin *peerSettingsPluginStub) *DialogsCore {
	ctx := context.Background()
	var p interface {
		GetChannelListByIdList(context.Context, int64, ...int64) []*mtproto.Chat
		GetChannelDialogById(context.Context, int64, int64) (*dialog.DialogExt, error)
		GetChannelMessage(context.Context, int64, int64, int32) (*mtproto.MessageBox, error)
		GetChannelTypingRecipients(context.Context, int64, *mtproto.InputPeer) ([]int64, error)
	}
	if plugin != nil {
		p = plugin
	}
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao:    &dao.Dao{UserClient: user, ChatClient: chat},
			Plugin: p,
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func peerSettingsImmutableUser(id, accessHash int64) *mtproto.ImmutableUser {
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: id, AccessHash: accessHash, FirstName: "User"},
	}).To_ImmutableUser()
}

func peerSettingsMutableChat(id, memberID int64) *mtproto.MutableChat {
	return &mtproto.MutableChat{
		Chat: &mtproto.ImmutableChat{PredicateName: mtproto.Predicate_chat, Id: id, Title: "Group"},
		ChatParticipants: []*mtproto.ImmutableChatParticipant{{
			UserId: memberID, ParticipantType: mtproto.ChatMemberNormal, State: mtproto.ChatMemberStateNormal,
		}},
	}
}

func TestMessagesGetPeerSettingsFailsClosedForMissingOrInvalidEntities(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		core := newPeerSettingsTestCore(&peerSettingsUserClientStub{}, &peerSettingsChatClientStub{}, nil)
		core.MD = &metadata.RpcMetadata{}
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()})
		if !errors.Is(err, mtproto.ErrAuthKeyUnregistered) || got != nil {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
		}
	})

	t.Run("nil request", func(t *testing.T) {
		core := newPeerSettingsTestCore(&peerSettingsUserClientStub{}, &peerSettingsChatClientStub{}, nil)
		got, err := core.MessagesGetPeerSettings(nil)
		if !errors.Is(err, mtproto.ErrPeerIdInvalid) || got != nil {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want PEER_ID_INVALID", got, err)
		}
	})

	t.Run("nil settings response", func(t *testing.T) {
		core := newPeerSettingsTestCore(&peerSettingsUserClientStub{}, &peerSettingsChatClientStub{}, nil)
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()})
		if err == nil || got != nil {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want fail-closed error", got, err)
		}
	})

	t.Run("valid user hydration", func(t *testing.T) {
		user := &peerSettingsUserClientStub{
			settings: &mtproto.PeerSettings{},
			users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
				peerSettingsImmutableUser(42, 420), peerSettingsImmutableUser(84, 100),
			}},
		}
		core := newPeerSettingsTestCore(user, &peerSettingsChatClientStub{}, nil)
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 100}).To_InputPeer()})
		if err != nil || got == nil || len(got.GetUsers()) != 1 || got.GetUsers()[0].GetId() != 84 {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want hydrated user 84", got, err)
		}
	})

	t.Run("nil user hydration", func(t *testing.T) {
		user := &peerSettingsUserClientStub{settings: &mtproto.PeerSettings{}}
		core := newPeerSettingsTestCore(user, &peerSettingsChatClientStub{}, nil)
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 100}).To_InputPeer()})
		if err == nil || got != nil {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want missing-user error", got, err)
		}
	})

	t.Run("wrong user access hash", func(t *testing.T) {
		user := &peerSettingsUserClientStub{
			settings: &mtproto.PeerSettings{},
			users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
				peerSettingsImmutableUser(42, 420), peerSettingsImmutableUser(84, 101),
			}},
		}
		core := newPeerSettingsTestCore(user, &peerSettingsChatClientStub{}, nil)
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 100}).To_InputPeer()})
		if !errors.Is(err, mtproto.ErrUserIdInvalid) || got != nil {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want USER_ID_INVALID", got, err)
		}
	})

	t.Run("chat hydration error", func(t *testing.T) {
		wantErr := errors.New("chat unavailable")
		user := &peerSettingsUserClientStub{settings: &mtproto.PeerSettings{}}
		chat := &peerSettingsChatClientStub{err: wantErr}
		core := newPeerSettingsTestCore(user, chat, nil)
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer()})
		if !errors.Is(err, wantErr) || got != nil {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want propagated chat error", got, err)
		}
	})

	t.Run("nonmember basic chat", func(t *testing.T) {
		user := &peerSettingsUserClientStub{settings: &mtproto.PeerSettings{}}
		chat := &peerSettingsChatClientStub{chat: peerSettingsMutableChat(17, 7)}
		core := newPeerSettingsTestCore(user, chat, nil)
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer()})
		if !errors.Is(err, mtproto.ErrUserNotParticipant) || got != nil {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want USER_NOT_PARTICIPANT", got, err)
		}
	})

	t.Run("valid basic chat member", func(t *testing.T) {
		user := &peerSettingsUserClientStub{settings: &mtproto.PeerSettings{}}
		chat := &peerSettingsChatClientStub{chat: peerSettingsMutableChat(17, 42)}
		core := newPeerSettingsTestCore(user, chat, nil)
		got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer()})
		if err != nil || got == nil || len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != 17 {
			t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want hydrated chat 17", got, err)
		}
	})
}

func TestMessagesGetPeerSettingsRequiresExactChannelHydration(t *testing.T) {
	user := &peerSettingsUserClientStub{settings: &mtproto.PeerSettings{}}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 90, AccessHash: 901}).To_InputPeer()
	core := newPeerSettingsTestCore(user, &peerSettingsChatClientStub{}, nil)
	if got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: peer}); err == nil || got != nil {
		t.Fatalf("without plugin MessagesGetPeerSettings() = (%v, %v), want fail-closed error", got, err)
	}

	wrongChannel := mtproto.MakeTLChannel(&mtproto.Chat{
		Id: 90, AccessHash_FLAGINT64: mtproto.MakeFlagsInt64(902),
	}).To_Chat()
	core = newPeerSettingsTestCore(user, &peerSettingsChatClientStub{}, &peerSettingsPluginStub{channels: []*mtproto.Chat{wrongChannel}})
	if got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: peer}); !errors.Is(err, mtproto.ErrChannelInvalid) || got != nil {
		t.Fatalf("wrong-hash MessagesGetPeerSettings() = (%v, %v), want CHANNEL_INVALID", got, err)
	}

	channel := mtproto.MakeTLChannel(&mtproto.Chat{
		Id: 90, AccessHash_FLAGINT64: mtproto.MakeFlagsInt64(901),
	}).To_Chat()
	core = newPeerSettingsTestCore(user, &peerSettingsChatClientStub{}, &peerSettingsPluginStub{channels: []*mtproto.Chat{channel}})
	got, err := core.MessagesGetPeerSettings(&mtproto.TLMessagesGetPeerSettings{Peer: peer})
	if err != nil || got == nil || len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != 90 {
		t.Fatalf("MessagesGetPeerSettings() = (%v, %v), want channel 90", got, err)
	}
}
