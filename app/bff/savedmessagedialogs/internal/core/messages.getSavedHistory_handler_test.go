package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bffdao "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type savedHistoryMessageClient struct {
	messageclient.MessageClient
	boxes   *mtproto.MessageBoxList
	err     error
	request *messagepb.TLMessageGetSavedHistoryMessages
	calls   int
}

func (c *savedHistoryMessageClient) MessageGetSavedHistoryMessages(_ context.Context, in *messagepb.TLMessageGetSavedHistoryMessages) (*mtproto.MessageBoxList, error) {
	c.calls++
	c.request = in
	return c.boxes, c.err
}

type savedHistoryUserClient struct {
	userclient.UserClient
	users *userpb.Vector_ImmutableUser
	err   error
	calls int
}

func (c *savedHistoryUserClient) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	c.calls++
	return c.users, c.err
}

type savedHistoryChatClient struct {
	chatclient.ChatClient
	chats *chatpb.Vector_MutableChat
	err   error
	calls int
}

func (c *savedHistoryChatClient) ChatGetChatListByIdList(_ context.Context, _ *chatpb.TLChatGetChatListByIdList) (*chatpb.Vector_MutableChat, error) {
	c.calls++
	return c.chats, c.err
}

func newSavedHistoryCore(messages *savedHistoryMessageClient, users *savedHistoryUserClient, chats *savedHistoryChatClient, userID int64) *SavedMessageDialogsCore {
	ctx := context.Background()
	return &SavedMessageDialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &bffdao.Dao{
			MessageClient: messages,
			UserClient:    users,
			ChatClient:    chats,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func savedHistoryChannelBox(channelID int64) *mtproto.MessageBoxList {
	return &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{{
		MessageId: 17,
		PeerType:  mtproto.PEER_CHANNEL,
		PeerId:    channelID,
		Message: &mtproto.Message{
			Id:      17,
			PeerId:  mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: channelID}).To_Peer(),
			Message: "saved channel message",
		},
	}}}
}

func savedHistoryRequest() *mtproto.TLMessagesGetSavedHistory {
	return &mtproto.TLMessagesGetSavedHistory{
		Peer:  mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 9001, AccessHash: 77}).To_InputPeer(),
		Limit: 20,
	}
}

func TestMessagesGetSavedHistoryHydratesChannelChats(t *testing.T) {
	messages := &savedHistoryMessageClient{boxes: savedHistoryChannelBox(9001)}
	core := newSavedHistoryCore(messages, &savedHistoryUserClient{users: &userpb.Vector_ImmutableUser{}}, &savedHistoryChatClient{chats: &chatpb.Vector_MutableChat{}}, 42)
	core.channelChatsByID = func(userID int64, ids []int64) []*mtproto.Chat {
		if userID != 42 || len(ids) != 1 || ids[0] != 9001 {
			t.Fatalf("channel resolver args = user %d ids %v", userID, ids)
		}
		return []*mtproto.Chat{mtproto.MakeTLChannel(&mtproto.Chat{Id: 9001, Title: "Saved channel", Photo: mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto()}).To_Chat()}
	}

	got, err := core.MessagesGetSavedHistory(savedHistoryRequest())
	if err != nil {
		t.Fatalf("MessagesGetSavedHistory() error = %v", err)
	}
	if got == nil || got.GetPredicateName() != mtproto.Predicate_messages_messages || len(got.GetMessages()) != 1 || len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != 9001 {
		t.Fatalf("MessagesGetSavedHistory() = %v, want one message and hydrated channel", got)
	}
	if messages.calls != 1 || messages.request.GetPeerId() != 9001 || messages.request.GetUserId() != 42 {
		t.Fatalf("message provider calls/request = %d/%v", messages.calls, messages.request)
	}
}

func TestMessagesGetSavedHistoryValidatesAndFailsClosed(t *testing.T) {
	request := savedHistoryRequest()
	unauthenticated := newSavedHistoryCore(&savedHistoryMessageClient{}, &savedHistoryUserClient{}, &savedHistoryChatClient{}, 0)
	if got, err := unauthenticated.MessagesGetSavedHistory(request); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}

	core := newSavedHistoryCore(&savedHistoryMessageClient{}, &savedHistoryUserClient{}, &savedHistoryChatClient{}, 42)
	if got, err := core.MessagesGetSavedHistory(nil); got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("nil request = (%v, %v), want PEER_ID_INVALID", got, err)
	}

	core = newSavedHistoryCore(&savedHistoryMessageClient{boxes: nil}, &savedHistoryUserClient{}, &savedHistoryChatClient{}, 42)
	if got, err := core.MessagesGetSavedHistory(request); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil provider response = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}

	want := errors.New("saved history unavailable")
	core = newSavedHistoryCore(&savedHistoryMessageClient{err: want}, &savedHistoryUserClient{}, &savedHistoryChatClient{}, 42)
	if got, err := core.MessagesGetSavedHistory(request); got != nil || !errors.Is(err, want) {
		t.Fatalf("provider error = (%v, %v), want propagated error", got, err)
	}

	core = newSavedHistoryCore(&savedHistoryMessageClient{boxes: savedHistoryChannelBox(9001)}, &savedHistoryUserClient{}, &savedHistoryChatClient{}, 42)
	core.channelChatsByID = nil
	if got, err := core.MessagesGetSavedHistory(request); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing channel provider = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}
}

func TestMessagesGetSavedHistoryRejectsNegativeLimit(t *testing.T) {
	request := savedHistoryRequest()
	request.Limit = -1
	core := newSavedHistoryCore(&savedHistoryMessageClient{}, &savedHistoryUserClient{}, &savedHistoryChatClient{}, 42)
	if got, err := core.MessagesGetSavedHistory(request); got != nil || !errors.Is(err, mtproto.ErrLimitInvalid) {
		t.Fatalf("negative limit = (%v, %v), want LIMIT_INVALID", got, err)
	}
}
