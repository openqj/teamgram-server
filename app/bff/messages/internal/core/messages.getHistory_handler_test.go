package core

import (
	"context"
	"errors"
	"net"
	"strings"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type getHistoryMessageClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	count    *mtproto.Int32
}

func (s *getHistoryMessageClientStub) MessageGetHistoryMessages(context.Context, *messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
	return s.response, nil
}

func (s *getHistoryMessageClientStub) MessageGetHistoryMessagesCount(context.Context, *messagepb.TLMessageGetHistoryMessagesCount) (*mtproto.Int32, error) {
	if s.count != nil {
		return s.count, nil
	}
	return &mtproto.Int32{V: 1}, nil
}

type getHistoryUserClientStub struct {
	userclient.UserClient
	validationResponse *userpb.Vector_ImmutableUser
	response           *userpb.Vector_ImmutableUser
	err                error
	calls              int
}

func (s *getHistoryUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.calls++
	if s.calls == 1 && s.validationResponse != nil {
		return s.validationResponse, nil
	}
	if s.response != nil {
		return s.response, nil
	}
	return nil, s.err
}

type getHistoryChatRPCStub struct {
	chatpb.UnimplementedRPCChatServer
	chat    *mtproto.MutableChat
	getErr  error
	listErr error
}

func (s *getHistoryChatRPCStub) ChatGetMutableChat(context.Context, *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.chat != nil {
		return s.chat, nil
	}
	return &mtproto.MutableChat{}, nil
}

func (s *getHistoryChatRPCStub) ChatGetChatListByIdList(context.Context, *chatpb.TLChatGetChatListByIdList) (*chatpb.Vector_MutableChat, error) {
	return nil, s.listErr
}

type getHistoryZRPCClient struct {
	conn *grpc.ClientConn
}

func (c getHistoryZRPCClient) Conn() *grpc.ClientConn {
	return c.conn
}

func newGetHistoryChatClient(t *testing.T, rpcStub *getHistoryChatRPCStub) *chatclient.ChatClientHelper {
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
	return chatclient.NewChatClientHelper(getHistoryZRPCClient{conn: conn})
}

func newGetHistoryTestCore(messages messageclient.MessageClient, users userclient.UserClient, chats *chatclient.ChatClientHelper) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			UserClient:    users,
			ChatClient:    chats,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 41},
	}
}

func getHistoryBox(id int32, peer *mtproto.Peer) *mtproto.MessageBox {
	peerUtil := mtproto.FromPeer(peer)
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      id,
		FromId:  mtproto.MakePeerUser(41),
		PeerId:  peer,
		Message: "history fixture",
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: id,
		PeerType:  peerUtil.PeerType,
		PeerId:    peerUtil.PeerId,
		Message:   msg,
	}).To_MessageBox()
}

func getHistoryUsers(users ...int64) *userpb.Vector_ImmutableUser {
	datas := make([]*mtproto.ImmutableUser, 0, len(users))
	for _, id := range users {
		datas = append(datas, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, AccessHash: id * 10}})
	}
	return &userpb.Vector_ImmutableUser{Datas: datas}
}

func getHistoryValidatedUser(id, accessHash int64) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{{User: &mtproto.UserData{Id: id, AccessHash: accessHash}}}}
}

func TestMessagesGetHistoryPropagatesUserHydrationError(t *testing.T) {
	wantErr := errors.New("user hydration unavailable")
	messages := &getHistoryMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{getHistoryBox(100, mtproto.MakePeerUser(84))},
	}}
	users := &getHistoryUserClientStub{
		validationResponse: getHistoryValidatedUser(84, 840),
		err:                wantErr,
	}
	core := newGetHistoryTestCore(messages, users, nil)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer()

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 10})
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want nil result and propagated user hydration error", got, err)
	}
}

func TestMessagesGetHistoryPropagatesChatHydrationError(t *testing.T) {
	chatErr := status.Error(codes.Unavailable, "chat hydration unavailable")
	messages := &getHistoryMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{getHistoryBox(100, mtproto.MakePeerChat(77))},
	}}
	users := &getHistoryUserClientStub{response: getHistoryUsers(41)}
	chats := newGetHistoryChatClient(t, &getHistoryChatRPCStub{
		chat: &mtproto.MutableChat{ChatParticipants: []*mtproto.ImmutableChatParticipant{{
			UserId: 41, State: mtproto.ChatMemberStateNormal,
		}}},
		listErr: chatErr,
	})
	core := newGetHistoryTestCore(messages, users, chats)
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer()

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 10})
	if got != nil || err == nil || !strings.Contains(err.Error(), "chat hydration unavailable") {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want nil result and propagated chat hydration error", got, err)
	}
}

func TestMessagesGetHistoryRejectsInvalidBounds(t *testing.T) {
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	for _, tc := range []struct {
		name string
		in   *mtproto.TLMessagesGetHistory
		want error
	}{
		{name: "zero limit", in: &mtproto.TLMessagesGetHistory{Peer: peer}, want: mtproto.ErrLimitInvalid},
		{name: "negative offset", in: &mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1, OffsetId: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative message bound", in: &mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1, MaxId: -1}, want: mtproto.ErrMessageIdInvalid},
		{name: "max id unsupported", in: &mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1, MaxId: 10}, want: mtproto.ErrMethodNotImpl},
		{name: "min id unsupported", in: &mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1, MinId: 10}, want: mtproto.ErrMethodNotImpl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := &getHistoryMessageClientStub{}
			core := newGetHistoryTestCore(messages, nil, nil)
			got, err := core.MessagesGetHistory(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("MessagesGetHistory() = (%v, %v), want nil and %v", got, err, tc.want)
			}
		})
	}
}

func TestMessagesGetHistoryRejectsUnprovenChannelHash(t *testing.T) {
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 77, AccessHash: 770}).To_InputPeer()
	core := newGetHistoryTestCore(&getHistoryMessageClientStub{}, nil, nil)
	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1, Hash: 1})
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesGetHistory(channel hash) = (%v, %v), want nil and METHOD_NOT_IMPL until canonical hash is available", got, err)
	}
}

func TestMessagesGetHistoryRejectsMismatchedUserAccessHash(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 841}).To_InputPeer()
	users := &getHistoryUserClientStub{response: getHistoryValidatedUser(84, 840)}
	core := newGetHistoryTestCore(&getHistoryMessageClientStub{}, users, nil)

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1})
	if got != nil || err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want nil and PEER_ID_INVALID", got, err)
	}
}

func TestMessagesGetHistoryRequiresChatMembership(t *testing.T) {
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer()
	chats := newGetHistoryChatClient(t, &getHistoryChatRPCStub{chat: &mtproto.MutableChat{
		ChatParticipants: []*mtproto.ImmutableChatParticipant{{UserId: 7, State: mtproto.ChatMemberStateNormal}},
	}})
	messages := &getHistoryMessageClientStub{}
	core := newGetHistoryTestCore(messages, nil, chats)

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1})
	if got != nil || err != mtproto.ErrUserNotParticipant {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want nil and USER_NOT_PARTICIPANT", got, err)
	}
}

func TestMessagesGetHistoryRejectsMalformedProviderBox(t *testing.T) {
	messages := &getHistoryMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{nil},
	}}
	core := newGetHistoryTestCore(messages, nil, nil)
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 1})
	if got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetHistoryUsesSliceWithoutUnsupportedMetadata(t *testing.T) {
	messages := &getHistoryMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{getHistoryBox(100, mtproto.MakePeerUser(41))},
	}}
	users := &getHistoryUserClientStub{response: getHistoryUsers(41)}
	core := newGetHistoryTestCore(messages, users, nil)

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{
		Peer:  mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Limit: 1,
	})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_messages_messagesSlice {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want messages.messagesSlice", got, err)
	}
	if got.GetCount() != 1 || got.GetInexact() || got.GetNextRate() != nil || got.GetOffsetIdOffset() != nil {
		t.Fatalf("slice metadata = count %d inexact %v next_rate %v offset_id_offset %v, want exact count and nil optional metadata", got.GetCount(), got.GetInexact(), got.GetNextRate(), got.GetOffsetIdOffset())
	}
}

func TestMessagesGetHistoryTrimsProviderBackwardAddOffset(t *testing.T) {
	messages := &getHistoryMessageClientStub{
		response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
			getHistoryBox(100, mtproto.MakePeerUser(41)),
			getHistoryBox(99, mtproto.MakePeerUser(41)),
		}},
		count: &mtproto.Int32{V: 2},
	}
	users := &getHistoryUserClientStub{response: getHistoryUsers(41)}
	core := newGetHistoryTestCore(messages, users, nil)

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{
		Peer:      mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		AddOffset: 1,
		Limit:     1,
	})
	if err != nil || got == nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 99 {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want one message after provider add_offset", got, err)
	}
}

func TestMessagesGetHistoryAllowsForwardAddOffset(t *testing.T) {
	messages := &getHistoryMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{getHistoryBox(100, mtproto.MakePeerUser(41))},
	}, count: &mtproto.Int32{V: 1}}
	users := &getHistoryUserClientStub{response: getHistoryUsers(41)}
	core := newGetHistoryTestCore(messages, users, nil)

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{
		Peer:      mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		AddOffset: -1,
		Limit:     1,
	})
	if err != nil || got == nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 100 {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want one message for forward add_offset", got, err)
	}
}

func TestMessagesGetHistoryRejectsProviderOverfetchWithoutSupportedOffset(t *testing.T) {
	messages := &getHistoryMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
		getHistoryBox(100, mtproto.MakePeerUser(41)),
		getHistoryBox(99, mtproto.MakePeerUser(41)),
	}}}
	core := newGetHistoryTestCore(messages, &getHistoryUserClientStub{response: getHistoryUsers(41)}, nil)

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{
		Peer:  mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Limit: 1,
	})
	if got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetHistoryFailsClosedWithoutChatProvider(t *testing.T) {
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer()
	for _, tc := range []struct {
		name  string
		chats *chatclient.ChatClientHelper
	}{
		{name: "missing helper"},
		{name: "missing client", chats: &chatclient.ChatClientHelper{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := newGetHistoryTestCore(&getHistoryMessageClientStub{}, nil, tc.chats)
			got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 10})
			if got != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("MessagesGetHistory() = (%v, %v), want nil result and INTERNAL_SERVER_ERROR", got, err)
			}
		})
	}
}
