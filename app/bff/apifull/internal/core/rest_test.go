package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
)

func TestDeepLinkMethodsReturnTypedEmptyResults(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}

	if _, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:      mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 3}).To_InputUser(),
		Peer:     mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(),
		RandomId: 1,
	}); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesStartBot() error = %v, want METHOD_NOT_IMPL", err)
	}
	info, err := c.HelpGetDeepLinkInfo(&mtproto.TLHelpGetDeepLinkInfo{Path: "share/example"})
	if err != nil {
		t.Fatalf("HelpGetDeepLinkInfo() error = %v", err)
	}
	if info.GetPredicateName() != mtproto.Predicate_help_deepLinkInfoEmpty {
		t.Fatalf("HelpGetDeepLinkInfo() predicate = %q", info.GetPredicateName())
	}
	if _, err := c.HelpGetDeepLinkInfo(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("HelpGetDeepLinkInfo(nil) error = %v, want INPUT_REQUEST_INVALID", err)
	}
	urls, err := c.HelpGetRecentMeUrls(&mtproto.TLHelpGetRecentMeUrls{})
	if err != nil {
		t.Fatalf("HelpGetRecentMeUrls() error = %v", err)
	}
	if urls.GetPredicateName() != mtproto.Predicate_help_recentMeUrls || len(urls.GetUrls()) != 0 || len(urls.GetChats()) != 0 || len(urls.GetUsers()) != 0 {
		t.Fatalf("HelpGetRecentMeUrls() = %+v", urls)
	}
}

func TestMessagesStartBotDirectUserDispatchesValidatedStartMessage(t *testing.T) {
	const (
		callerID   int64 = 11001
		botID      int64 = 11002
		accessHash int64 = 771199
		randomID   int64 = 9911002
	)
	sender := &scheduledSenderStub{}
	users := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient:             users,
			ScheduledMessageSender: sender,
		}},
		MD: &metadata.RpcMetadata{UserId: callerID, PermAuthKeyId: 91},
	}
	updates, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:        mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(),
		Peer:       mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: botID, AccessHash: accessHash}).To_InputPeer(),
		RandomId:   randomID,
		StartParam: "launch_1",
	})
	if err != nil || updates == nil {
		t.Fatalf("direct start bot = (%+v, %v), want typed updates", updates, err)
	}
	if sender.request == nil || sender.request.GetUserId() != callerID || sender.request.GetAuthKeyId() != 91 || sender.request.GetPeerType() != mtproto.PEER_USER || sender.request.GetPeerId() != botID {
		t.Fatalf("dispatch request = %+v", sender.request)
	}
	if len(sender.request.GetMessage()) != 1 {
		t.Fatalf("dispatch message count = %d, want 1", len(sender.request.GetMessage()))
	}
	outbox := sender.request.GetMessage()[0]
	if outbox.GetRandomId() != randomID || outbox.GetMessage().GetMessage() != "/start launch_1" || !outbox.GetNoWebpage() {
		t.Fatalf("dispatch outbox = %+v", outbox)
	}
}

func TestMessagesStartBotGroupDispatchesOnlyForActiveMembers(t *testing.T) {
	const (
		callerID   int64 = 11101
		botID      int64 = 11102
		chatID     int64 = 11103
		accessHash int64 = 771200
		randomID   int64 = 9911003
	)
	sender := &scheduledSenderStub{}
	users := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	chats := &startBotChatClient{chat: &mtproto.MutableChat{
		Chat: mtproto.MakeTLImmutableChat(&mtproto.ImmutableChat{Id: chatID}).To_ImmutableChat(),
		ChatParticipants: []*mtproto.ImmutableChatParticipant{
			{UserId: callerID, State: mtproto.ChatMemberStateNormal},
			{UserId: botID, State: mtproto.ChatMemberStateNormal, IsBot: true},
		},
	}}
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient:             users,
			ChatClient:             chats,
			ScheduledMessageSender: sender,
		}},
		MD: &metadata.RpcMetadata{UserId: callerID, PermAuthKeyId: 92},
	}
	updates, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:        mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(),
		Peer:       mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer(),
		RandomId:   randomID,
		StartParam: "launch_2",
	})
	if err != nil || updates == nil {
		t.Fatalf("group start bot = (%+v, %v), want typed updates", updates, err)
	}
	if sender.request == nil || sender.request.GetPeerType() != mtproto.PEER_CHAT || sender.request.GetPeerId() != chatID {
		t.Fatalf("group dispatch request = %+v", sender.request)
	}
	if len(sender.request.GetMessage()) != 1 || sender.request.GetMessage()[0].GetMessage().GetPeerId().GetChatId() != chatID {
		t.Fatalf("group dispatch message = %+v", sender.request.GetMessage())
	}

	chats.chat.ChatParticipants[1].State = mtproto.ChatMemberStateLeft
	if updates, err = c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:      mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(),
		Peer:     mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer(),
		RandomId: randomID + 1,
	}); updates != nil || err != mtproto.ErrBotInvalid {
		t.Fatalf("inactive bot member = (%+v, %v), want BOT_INVALID", updates, err)
	}
}

func TestMessagesStartBotChannelDispatchesOnlyForActiveMembers(t *testing.T) {
	if !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is required for canonical channel storage")
	}
	const (
		callerID   int64 = 11201
		botID      int64 = 11202
		accessHash int64 = 771201
		randomID   int64 = 9911004
	)
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: accessHash, Creator: callerID, Title: "start-bot-channel", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(callerID, channelID) })
	if err := domain.JoinChannel(channelID, botID); err != nil {
		t.Fatal(err)
	}
	sender := &scheduledSenderStub{}
	users := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient:             users,
			ScheduledMessageSender: sender,
		}},
		MD: &metadata.RpcMetadata{UserId: callerID, PermAuthKeyId: 93},
	}
	channelPeer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: accessHash}).To_InputPeer()
	updates, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:        mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(),
		Peer:       channelPeer,
		RandomId:   randomID,
		StartParam: "launch_channel",
	})
	if err != nil || updates == nil {
		t.Fatalf("channel start bot = (%+v, %v), want typed updates", updates, err)
	}
	if sender.request == nil || sender.request.GetPeerType() != mtproto.PEER_CHANNEL || sender.request.GetPeerId() != channelID {
		t.Fatalf("channel dispatch request = %+v", sender.request)
	}
	outbox := sender.request.GetMessage()[0]
	if outbox.GetMessage().GetPeerId().GetChannelId() != channelID || outbox.GetMessage().GetMessage() != "/start launch_channel" {
		t.Fatalf("channel dispatch message = %+v", outbox.GetMessage())
	}

	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: accessHash + 1}).To_InputPeer()
	if got, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(), Peer: badHash, RandomId: randomID + 1,
	}); got != nil || err != mtproto.ErrChannelInvalid {
		t.Fatalf("bad channel hash = (%+v, %v), want CHANNEL_INVALID", got, err)
	}

	if err := domain.LeaveChannel(channelID, botID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(), Peer: channelPeer, RandomId: randomID + 2,
	}); got != nil || err != mtproto.ErrBotInvalid {
		t.Fatalf("inactive channel bot = (%+v, %v), want BOT_INVALID", got, err)
	}
}

type startBotChatClient struct {
	chat_client.ChatClient
	chat *mtproto.MutableChat
}

func (c *startBotChatClient) ChatGetMutableChat(context.Context, *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	return c.chat, nil
}
