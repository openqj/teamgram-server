package core

import (
	"context"
	"net"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const messagesViewsCallerID int64 = 41

type messagesViewsMessageClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	err      error
	calls    int
	request  *messagepb.TLMessageGetUserMessageList
}

func (s *messagesViewsMessageClientStub) MessageGetUserMessageList(_ context.Context, in *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	s.calls++
	s.request = in
	return s.response, s.err
}

type messagesViewsUserClientStub struct {
	userclient.UserClient
	response *userpb.Vector_ImmutableUser
	err      error
	calls    int
}

func (s *messagesViewsUserClientStub) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.calls++
	return s.response, s.err
}

type messagesViewsChatRPCStub struct {
	chatpb.UnimplementedRPCChatServer
	chat  *mtproto.MutableChat
	err   error
	calls int
}

func (s *messagesViewsChatRPCStub) ChatGetMutableChat(_ context.Context, _ *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	s.calls++
	return s.chat, s.err
}

type messagesViewsZRPCClient struct {
	conn *grpc.ClientConn
}

func (c messagesViewsZRPCClient) Conn() *grpc.ClientConn {
	return c.conn
}

func newMessagesViewsChatClient(t *testing.T, rpcStub *messagesViewsChatRPCStub) *chatclient.ChatClientHelper {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	chatpb.RegisterRPCChatServer(server, rpcStub)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial in-memory chat RPC: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return chatclient.NewChatClientHelper(messagesViewsZRPCClient{conn: conn})
}

func newMessagesViewsTestCore(messages messageclient.MessageClient, users userclient.UserClient, chats *chatclient.ChatClientHelper) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			UserClient:    users,
			ChatClient:    chats,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: messagesViewsCallerID},
	}
}

func messagesViewsUser(id, accessHash int64) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{
			Id:         id,
			AccessHash: accessHash,
		}}).To_ImmutableUser(),
	}}
}

func messagesViewsBox(id int32, peer *mtproto.Peer, views, forwards *wrapperspb.Int32Value, replies *mtproto.MessageReplies) *mtproto.MessageBox {
	peerUtil := mtproto.FromPeer(peer)
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Id:       id,
		FromId:   mtproto.MakePeerUser(messagesViewsCallerID),
		PeerId:   peer,
		Message:  "views fixture",
		Views:    views,
		Forwards: forwards,
		Replies:  replies,
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: id,
		PeerType:  peerUtil.PeerType,
		PeerId:    peerUtil.PeerId,
		Message:   message,
	}).To_MessageBox()
}

func TestMessagesGetMessagesViewsReturnsStoredValuesInRequestOrder(t *testing.T) {
	const (
		peerID     int64 = 84
		accessHash int64 = 840
	)
	peer := mtproto.MakePeerUser(peerID)
	replies := mtproto.MakeTLMessageReplies(&mtproto.MessageReplies{Comments: true, Replies: 3}).To_MessageReplies()
	first := messagesViewsBox(1, peer, nil, wrapperspb.Int32(7), replies)
	second := messagesViewsBox(2, peer, wrapperspb.Int32(20), wrapperspb.Int32(4), nil)
	messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{first, second},
	}}
	users := &messagesViewsUserClientStub{response: messagesViewsUser(peerID, accessHash)}
	core := newMessagesViewsTestCore(messages, users, nil)

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
			UserId:     peerID,
			AccessHash: accessHash,
		}).To_InputPeer(),
		Id:        []int32{2, 1, 2},
		Increment: mtproto.BoolFalse,
	})
	if err != nil {
		t.Fatalf("MessagesGetMessagesViews() error = %v", err)
	}
	if messages.calls != 1 || len(messages.request.GetIdList()) != 3 {
		t.Fatalf("message lookup calls=%d ids=%v, want one lookup for requested IDs", messages.calls, messages.request.GetIdList())
	}
	if len(got.GetViews()) != 3 {
		t.Fatalf("views length = %d, want 3", len(got.GetViews()))
	}
	if got.GetViews()[0].GetViews().GetValue() != 20 || got.GetViews()[2].GetViews().GetValue() != 20 {
		t.Fatalf("repeated message views = (%v, %v), want 20", got.GetViews()[0].GetViews(), got.GetViews()[2].GetViews())
	}
	if got.GetViews()[1].GetViews() != nil {
		t.Fatalf("nil stored views became %v", got.GetViews()[1].GetViews())
	}
	if got.GetViews()[1].GetForwards().GetValue() != 7 || got.GetViews()[1].GetReplies().GetReplies() != 3 {
		t.Fatalf("stored forwards/replies = (%v, %v), want (7, 3)", got.GetViews()[1].GetForwards(), got.GetViews()[1].GetReplies())
	}
}

func TestMessagesGetMessagesViewsRejectsMissingOrMismatchedStoredMessages(t *testing.T) {
	const (
		peerID     int64 = 84
		accessHash int64 = 840
	)
	validPeer := mtproto.MakePeerUser(peerID)
	tests := []struct {
		name  string
		ids   []int32
		boxes []*mtproto.MessageBox
	}{
		{
			name:  "missing",
			ids:   []int32{2},
			boxes: []*mtproto.MessageBox{messagesViewsBox(1, validPeer, wrapperspb.Int32(1), nil, nil)},
		},
		{
			name:  "peer mismatch",
			ids:   []int32{2},
			boxes: []*mtproto.MessageBox{messagesViewsBox(2, mtproto.MakePeerChat(99), wrapperspb.Int32(1), nil, nil)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: test.boxes}}
			users := &messagesViewsUserClientStub{response: messagesViewsUser(peerID, accessHash)}
			core := newMessagesViewsTestCore(messages, users, nil)

			got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
				Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
					UserId:     peerID,
					AccessHash: accessHash,
				}).To_InputPeer(),
				Id:        test.ids,
				Increment: mtproto.BoolFalse,
			})
			if got != nil || err != mtproto.ErrMessageIdInvalid {
				t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, MESSAGE_ID_INVALID)", got, err)
			}
		})
	}
}

func TestMessagesGetMessagesViewsRejectsBadUserAccessHashBeforeLookup(t *testing.T) {
	const (
		peerID     int64 = 84
		accessHash int64 = 840
	)
	messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{messagesViewsBox(1, mtproto.MakePeerUser(peerID), wrapperspb.Int32(1), nil, nil)},
	}}
	users := &messagesViewsUserClientStub{response: messagesViewsUser(peerID, accessHash)}
	core := newMessagesViewsTestCore(messages, users, nil)

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
			UserId:     peerID,
			AccessHash: accessHash + 1,
		}).To_InputPeer(),
		Id: []int32{1},
	})
	if got != nil || err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, PEER_ID_INVALID)", got, err)
	}
	if messages.calls != 0 {
		t.Fatalf("message lookup calls = %d, want 0 after invalid access hash", messages.calls)
	}
}

func TestMessagesGetMessagesViewsRequiresBasicGroupMembership(t *testing.T) {
	messages := &messagesViewsMessageClientStub{}
	chatStub := &messagesViewsChatRPCStub{chat: &mtproto.MutableChat{}}
	core := newMessagesViewsTestCore(messages, nil, newMessagesViewsChatClient(t, chatStub))

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
		Id:   []int32{1},
	})
	if got != nil || err != mtproto.ErrUserNotParticipant {
		t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, USER_NOT_PARTICIPANT)", got, err)
	}
	if messages.calls != 0 {
		t.Fatalf("message lookup calls = %d, want 0 for non-member", messages.calls)
	}
}

func TestMessagesGetMessagesViewsDoesNotFabricateIncrementedCounts(t *testing.T) {
	stored := messagesViewsBox(1, mtproto.MakePeerUser(messagesViewsCallerID), wrapperspb.Int32(7), nil, nil)
	messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{stored},
	}}
	core := newMessagesViewsTestCore(messages, nil, nil)

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer:      mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:        []int32{1},
		Increment: mtproto.BoolTrue,
	})
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
	if messages.calls != 1 || stored.GetMessage().GetViews().GetValue() != 7 {
		t.Fatalf("increment changed persisted fixture or skipped validation: calls=%d views=%v", messages.calls, stored.GetMessage().GetViews())
	}
}

func TestMessagesGetMessagesViewsDoesNotFabricateUserOrChatIncrementedCounts(t *testing.T) {
	const (
		userPeerID     int64 = 84
		userAccessHash int64 = 840
		chatPeerID     int64 = 77
	)

	tests := []struct {
		name       string
		request    *mtproto.InputPeer
		storedPeer *mtproto.Peer
		users      *messagesViewsUserClientStub
		chat       *messagesViewsChatRPCStub
	}{
		{
			name:       "user",
			request:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userPeerID, AccessHash: userAccessHash}).To_InputPeer(),
			storedPeer: mtproto.MakePeerUser(userPeerID),
			users:      &messagesViewsUserClientStub{response: messagesViewsUser(userPeerID, userAccessHash)},
		},
		{
			name:       "basic chat",
			request:    mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatPeerID}).To_InputPeer(),
			storedPeer: mtproto.MakePeerChat(chatPeerID),
			chat: &messagesViewsChatRPCStub{chat: &mtproto.MutableChat{
				Chat: &mtproto.ImmutableChat{Id: chatPeerID},
				ChatParticipants: []*mtproto.ImmutableChatParticipant{{
					UserId:          messagesViewsCallerID,
					ParticipantType: mtproto.ChatMemberNormal,
					State:           mtproto.ChatMemberStateNormal,
				}},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stored := messagesViewsBox(1, test.storedPeer, wrapperspb.Int32(7), nil, nil)
			messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{
				Datas: []*mtproto.MessageBox{stored},
			}}
			var chats *chatclient.ChatClientHelper
			if test.chat != nil {
				chats = newMessagesViewsChatClient(t, test.chat)
			}
			core := newMessagesViewsTestCore(messages, test.users, chats)

			got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
				Peer:      test.request,
				Id:        []int32{1},
				Increment: mtproto.BoolTrue,
			})
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
			if messages.calls != 1 || stored.GetMessage().GetViews().GetValue() != 7 {
				t.Fatalf("increment changed persisted fixture or skipped message validation: calls=%d views=%v", messages.calls, stored.GetMessage().GetViews())
			}
			if test.users != nil && test.users.calls != 1 {
				t.Fatalf("user validation calls = %d, want 1", test.users.calls)
			}
			if test.chat != nil && test.chat.calls != 1 {
				t.Fatalf("chat validation calls = %d, want 1", test.chat.calls)
			}
		})
	}
}

func TestMessagesGetMessagesViewsRejectsEmptyIDs(t *testing.T) {
	core := newMessagesViewsTestCore(nil, nil, nil)
	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{})
	if got != nil || err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("MessagesGetMessagesViews(empty IDs) = (%+v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
	}
}
