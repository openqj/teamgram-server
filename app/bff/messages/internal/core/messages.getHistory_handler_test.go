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
}

func (s *getHistoryMessageClientStub) MessageGetHistoryMessages(context.Context, *messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
	return s.response, nil
}

func (*getHistoryMessageClientStub) MessageGetHistoryMessagesCount(context.Context, *messagepb.TLMessageGetHistoryMessagesCount) (*mtproto.Int32, error) {
	return &mtproto.Int32{V: 1}, nil
}

type getHistoryUserClientStub struct {
	userclient.UserClient
	err error
}

func (s *getHistoryUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return nil, s.err
}

type getHistoryChatRPCStub struct {
	chatpb.UnimplementedRPCChatServer
	listErr error
}

func (*getHistoryChatRPCStub) ChatGetMutableChat(context.Context, *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
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

func TestMessagesGetHistoryPropagatesUserHydrationError(t *testing.T) {
	wantErr := errors.New("user hydration unavailable")
	messages := &getHistoryMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{getHistoryBox(100, mtproto.MakePeerUser(84))},
	}}
	users := &getHistoryUserClientStub{err: wantErr}
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
	users := &getHistoryUserClientStub{}
	chats := newGetHistoryChatClient(t, &getHistoryChatRPCStub{listErr: chatErr})
	core := newGetHistoryTestCore(messages, users, chats)
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer()

	got, err := core.MessagesGetHistory(&mtproto.TLMessagesGetHistory{Peer: peer, Limit: 10})
	if got != nil || err == nil || !strings.Contains(err.Error(), "chat hydration unavailable") {
		t.Fatalf("MessagesGetHistory() = (%v, %v), want nil result and propagated chat hydration error", got, err)
	}
}
