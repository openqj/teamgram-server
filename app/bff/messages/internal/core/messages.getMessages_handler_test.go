package core

import (
	"context"
	"errors"
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
)

const messagesGetMessagesCallerID int64 = 41

type messagesGetMessagesClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	err      error
	request  *messagepb.TLMessageGetUserMessageList
}

func (s *messagesGetMessagesClientStub) MessageGetUserMessageList(_ context.Context, in *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	s.request = in
	return s.response, s.err
}

type messagesGetMessagesUserClientStub struct {
	userclient.UserClient
	response *userpb.Vector_ImmutableUser
	err      error
}

func (s *messagesGetMessagesUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.response, s.err
}

type messagesGetMessagesChatRPCStub struct {
	chatpb.UnimplementedRPCChatServer
	response *chatpb.Vector_MutableChat
	err      error
}

func (s *messagesGetMessagesChatRPCStub) ChatGetChatListByIdList(context.Context, *chatpb.TLChatGetChatListByIdList) (*chatpb.Vector_MutableChat, error) {
	return s.response, s.err
}

type messagesGetMessagesZRPCClient struct {
	conn *grpc.ClientConn
}

func (c messagesGetMessagesZRPCClient) Conn() *grpc.ClientConn {
	return c.conn
}

func newMessagesGetMessagesChatClient(t *testing.T, rpcStub *messagesGetMessagesChatRPCStub) *chatclient.ChatClientHelper {
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
	return chatclient.NewChatClientHelper(messagesGetMessagesZRPCClient{conn: conn})
}

func newMessagesGetMessagesTestCore(messages messageclient.MessageClient, users userclient.UserClient, chats *chatclient.ChatClientHelper) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			UserClient:    users,
			ChatClient:    chats,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: messagesGetMessagesCallerID},
	}
}

func messagesGetMessagesInputID(id int32) *mtproto.InputMessage {
	return mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: id}).To_InputMessage()
}

func messagesGetMessagesBox(id int32, peer *mtproto.Peer, senderID int64) *mtproto.MessageBox {
	peerUtil := mtproto.FromPeer(peer)
	var fromID *mtproto.Peer
	if senderID > 0 {
		fromID = mtproto.MakePeerUser(senderID)
	}
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      id,
		FromId:  fromID,
		PeerId:  peer,
		Message: "getMessages fixture",
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId:    id,
		SenderUserId: senderID,
		PeerType:     peerUtil.PeerType,
		PeerId:       peerUtil.PeerId,
		Message:      message,
	}).To_MessageBox()
}

func messagesGetMessagesUsers(ids ...int64) *userpb.Vector_ImmutableUser {
	datas := make([]*mtproto.ImmutableUser, 0, len(ids))
	for _, id := range ids {
		datas = append(datas, mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: id, AccessHash: id * 10},
		}).To_ImmutableUser())
	}
	return &userpb.Vector_ImmutableUser{Datas: datas}
}

func TestMessagesGetMessagesReturnsMessageEmptyForUnknownInputMessages(t *testing.T) {
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{}}
	users := &messagesGetMessagesUserClientStub{response: messagesGetMessagesUsers(messagesGetMessagesCallerID)}
	core := newMessagesGetMessagesTestCore(client, users, nil)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			messagesGetMessagesInputID(7),
		},
		Id_VECTORINT32: []int32{8},
	})
	if err != nil {
		t.Fatalf("MessagesGetMessages() error = %v", err)
	}
	if got == nil || len(got.GetMessages()) != 2 {
		t.Fatalf("MessagesGetMessages() = %+v, want two message placeholders", got)
	}
	for i, wantID := range []int32{7, 8} {
		message := got.GetMessages()[i]
		if message == nil || message.GetPredicateName() != mtproto.Predicate_messageEmpty || message.GetId() != wantID {
			t.Fatalf("message[%d] = %+v, want messageEmpty id %d", i, message, wantID)
		}
	}
	if client.request == nil || len(client.request.GetIdList()) != 2 || client.request.GetIdList()[0] != 7 || client.request.GetIdList()[1] != 8 {
		t.Fatalf("message provider request = %+v, want ids [7 8]", client.request)
	}
}

func TestMessagesGetMessagesHydratesUsersAndBasicGroups(t *testing.T) {
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
		messagesGetMessagesBox(10, mtproto.MakePeerUser(84), 42),
		messagesGetMessagesBox(11, mtproto.MakePeerChat(77), 42),
	}}}
	users := &messagesGetMessagesUserClientStub{response: messagesGetMessagesUsers(messagesGetMessagesCallerID, 42, 84)}
	chats := newMessagesGetMessagesChatClient(t, &messagesGetMessagesChatRPCStub{response: &chatpb.Vector_MutableChat{
		Datas: []*mtproto.MutableChat{{Chat: &mtproto.ImmutableChat{Id: 77, Title: "fixture group"}}},
	}})
	core := newMessagesGetMessagesTestCore(client, users, chats)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{messagesGetMessagesInputID(10), messagesGetMessagesInputID(11)},
	})
	if err != nil {
		t.Fatalf("MessagesGetMessages() error = %v", err)
	}
	if got == nil || len(got.GetMessages()) != 2 || got.GetMessages()[0].GetId() != 10 || got.GetMessages()[1].GetId() != 11 {
		t.Fatalf("messages = %+v, want ids [10 11]", got.GetMessages())
	}
	if len(got.GetUsers()) != 3 || got.GetUsers()[0].GetId() != messagesGetMessagesCallerID || got.GetUsers()[1].GetId() != 42 || got.GetUsers()[2].GetId() != 84 {
		t.Fatalf("users = %+v, want caller, sender, and peer users", got.GetUsers())
	}
	if len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != 77 {
		t.Fatalf("chats = %+v, want basic group 77", got.GetChats())
	}
}

func TestMessagesGetMessagesFailsClosedForChannelEntityWithoutResolver(t *testing.T) {
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
		messagesGetMessagesBox(12, mtproto.MakePeerChannel(9001), 0),
	}}}
	users := &messagesGetMessagesUserClientStub{response: messagesGetMessagesUsers(messagesGetMessagesCallerID)}
	core := newMessagesGetMessagesTestCore(client, users, nil)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{messagesGetMessagesInputID(12)},
	})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("MessagesGetMessages() = (%v, %v), want nil result and METHOD_NOT_IMPL", got, err)
	}
}

func TestMessagesGetMessagesFailsClosedWithoutReturningPartialMixedResults(t *testing.T) {
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
		messagesGetMessagesBox(14, mtproto.MakePeerUser(84), 42),
		messagesGetMessagesBox(15, mtproto.MakePeerChannel(9001), 0),
		messagesGetMessagesBox(16, mtproto.MakePeerChannel(9002), 0),
	}}}
	users := &messagesGetMessagesUserClientStub{response: messagesGetMessagesUsers(messagesGetMessagesCallerID, 42, 84)}
	core := newMessagesGetMessagesTestCore(client, users, nil)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			messagesGetMessagesInputID(14),
			messagesGetMessagesInputID(15),
			messagesGetMessagesInputID(16),
		},
	})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("MessagesGetMessages() = (%v, %v), want nil result and METHOD_NOT_IMPL for mixed user/channel messages", got, err)
	}
}

func TestMessagesGetMessagesRejectsMalformedMessageProviderBox(t *testing.T) {
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{nil}}}
	core := newMessagesGetMessagesTestCore(client, nil, nil)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{messagesGetMessagesInputID(13)},
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetMessages() = (%v, %v), want nil result and INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetMessagesRejectsProviderIDMismatch(t *testing.T) {
	box := messagesGetMessagesBox(13, mtproto.MakePeerUser(84), 42)
	box.Message.Id = 14
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{box}}}
	core := newMessagesGetMessagesTestCore(client, nil, nil)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{messagesGetMessagesInputID(13)},
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetMessages() = (%v, %v), want nil result and INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetMessagesRejectsUnknownProviderPeerType(t *testing.T) {
	box := messagesGetMessagesBox(13, mtproto.MakePeerUser(84), 42)
	box.PeerType = mtproto.PEER_UNKNOWN
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{box}}}
	core := newMessagesGetMessagesTestCore(client, nil, nil)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{messagesGetMessagesInputID(13)},
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetMessages() = (%v, %v), want nil result and INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetMessagesRejectsPinnedInputWithoutResolver(t *testing.T) {
	client := &messagesGetMessagesClientStub{response: &messagepb.Vector_MessageBox{}}
	core := newMessagesGetMessagesTestCore(client, nil, nil)

	got, err := core.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			mtproto.MakeTLInputMessagePinned(nil).To_InputMessage(),
		},
	})
	if got != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("MessagesGetMessages() = (%v, %v), want nil result and INPUT_CONSTRUCTOR_INVALID", got, err)
	}
}
