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
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const messageEditDataUserID int64 = 42

type messageEditDataMessageClient struct {
	messageclient.MessageClient
	response *mtproto.MessageBox
	err      error
	calls    int
	request  *messagepb.TLMessageGetUserMessage
}

func (s *messageEditDataMessageClient) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	s.calls++
	s.request = in
	return s.response, s.err
}

type messageEditDataChatRPCStub struct {
	chatpb.UnimplementedRPCChatServer
	chat    *mtproto.MutableChat
	err     error
	calls   int
	request *chatpb.TLChatGetChatBySelfId
}

func (s *messageEditDataChatRPCStub) ChatGetChatBySelfId(_ context.Context, in *chatpb.TLChatGetChatBySelfId) (*mtproto.MutableChat, error) {
	s.calls++
	s.request = in
	return s.chat, s.err
}

type messageEditDataZRPCClient struct {
	conn *grpc.ClientConn
}

func (c messageEditDataZRPCClient) Conn() *grpc.ClientConn {
	return c.conn
}

func newMessageEditDataChatClient(t *testing.T, rpcStub *messageEditDataChatRPCStub) *chatclient.ChatClientHelper {
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
	return chatclient.NewChatClientHelper(messageEditDataZRPCClient{conn: conn})
}

func newMessageEditDataTestCore(messages messageclient.MessageClient, chats *chatclient.ChatClientHelper) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			ChatClient:    chats,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: messageEditDataUserID},
	}
}

func messageEditDataBox(id int32, peerType int32, peerID, senderID int64, media string) *mtproto.MessageBox {
	var peer *mtproto.Peer
	switch peerType {
	case mtproto.PEER_USER:
		peer = mtproto.MakePeerUser(peerID)
	case mtproto.PEER_CHAT:
		peer = mtproto.MakePeerChat(peerID)
	}
	return &mtproto.MessageBox{
		MessageId:    id,
		SenderUserId: senderID,
		PeerType:     peerType,
		PeerId:       peerID,
		Message: &mtproto.Message{
			Id:     id,
			FromId: mtproto.MakePeerUser(senderID),
			PeerId: peer,
			Media:  &mtproto.MessageMedia{PredicateName: media},
		},
	}
}

func messageEditDataRequest(peer *mtproto.InputPeer, id int32) *mtproto.TLMessagesGetMessageEditData {
	return &mtproto.TLMessagesGetMessageEditData{Peer: peer, Id: id}
}

func TestMessagesGetMessageEditDataReturnsMediaCaptionCapability(t *testing.T) {
	tests := []struct {
		name    string
		media   string
		caption bool
		peer    *mtproto.InputPeer
		boxPeer int32
		peerID  int64
	}{
		{name: "text", peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), boxPeer: mtproto.PEER_USER, peerID: messageEditDataUserID},
		{name: "photo", media: mtproto.Predicate_messageMediaPhoto, caption: true, peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), boxPeer: mtproto.PEER_USER, peerID: messageEditDataUserID},
		{name: "document", media: mtproto.Predicate_messageMediaDocument, caption: true, peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(), boxPeer: mtproto.PEER_USER, peerID: 84},
		{name: "location", media: mtproto.Predicate_messageMediaGeo, peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(), boxPeer: mtproto.PEER_USER, peerID: 84},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			messages := &messageEditDataMessageClient{response: messageEditDataBox(9, tc.boxPeer, tc.peerID, messageEditDataUserID, tc.media)}
			core := newMessageEditDataTestCore(messages, nil)

			got, err := core.MessagesGetMessageEditData(messageEditDataRequest(tc.peer, 9))
			if err != nil || got == nil || got.GetCaption() != tc.caption {
				t.Fatalf("MessagesGetMessageEditData() = (%v, %v), want caption=%v", got, err, tc.caption)
			}
			if messages.calls != 1 || messages.request.GetUserId() != messageEditDataUserID || messages.request.GetId() != 9 {
				t.Fatalf("message lookup = calls %d request %v, want caller and message id", messages.calls, messages.request)
			}
		})
	}
}

func TestMessagesGetMessageEditDataRejectsInvalidStoredMessage(t *testing.T) {
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	tests := []struct {
		name     string
		id       int32
		response *mtproto.MessageBox
		want     error
	}{
		{name: "invalid request id", id: 0, want: mtproto.ErrMessageIdInvalid},
		{name: "nil provider response", id: 9, want: mtproto.ErrInternalServerError},
		{name: "nil message", id: 9, response: &mtproto.MessageBox{MessageId: 9}, want: mtproto.ErrInternalServerError},
		{name: "message box id mismatch", id: 9, response: messageEditDataBox(8, mtproto.PEER_USER, messageEditDataUserID, messageEditDataUserID, ""), want: mtproto.ErrMessageIdInvalid},
		{name: "peer mismatch", id: 9, response: messageEditDataBox(9, mtproto.PEER_USER, 84, messageEditDataUserID, ""), want: mtproto.ErrMessageAuthorRequired},
		{name: "incoming message", id: 9, response: messageEditDataBox(9, mtproto.PEER_USER, 84, 84, ""), want: mtproto.ErrMessageAuthorRequired},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			messages := &messageEditDataMessageClient{response: tc.response}
			core := newMessageEditDataTestCore(messages, nil)
			got, err := core.MessagesGetMessageEditData(messageEditDataRequest(peer, tc.id))
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("MessagesGetMessageEditData() = (%v, %v), want nil and %v", got, err, tc.want)
			}
			if tc.id <= 0 && messages.calls != 0 {
				t.Fatalf("message lookup calls = %d, want 0 for invalid id", messages.calls)
			}
		})
	}
}

func TestMessagesGetMessageEditDataFailsClosedWithoutMessageProvider(t *testing.T) {
	core := newMessageEditDataTestCore(nil, nil)
	got, err := core.MessagesGetMessageEditData(messageEditDataRequest(mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), 9))
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetMessageEditData() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetMessageEditDataRejectsUserPeerWithoutAccessHash(t *testing.T) {
	messages := &messageEditDataMessageClient{response: messageEditDataBox(9, mtproto.PEER_USER, 84, messageEditDataUserID, "")}
	core := newMessageEditDataTestCore(messages, nil)
	got, err := core.MessagesGetMessageEditData(messageEditDataRequest(
		mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84}).To_InputPeer(), 9))
	if got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("MessagesGetMessageEditData() = (%v, %v), want nil and PEER_ID_INVALID", got, err)
	}
	if messages.calls != 0 {
		t.Fatalf("message lookup calls = %d, want 0 for missing access hash", messages.calls)
	}
}

func TestMessagesGetMessageEditDataChecksChatAdminRights(t *testing.T) {
	const (
		chatID    int64 = 77
		messageID       = 9
	)
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer()
	message := &messageEditDataMessageClient{response: messageEditDataBox(messageID, mtproto.PEER_CHAT, chatID, 84, "")}

	adminChat := &messageEditDataChatRPCStub{chat: &mtproto.MutableChat{ChatParticipants: []*mtproto.ImmutableChatParticipant{{
		UserId:          messageEditDataUserID,
		State:           mtproto.ChatMemberStateNormal,
		ParticipantType: mtproto.ChatMemberAdmin,
		AdminRights:     &mtproto.ChatAdminRights{EditMessages: true},
	}}}}
	core := newMessageEditDataTestCore(message, newMessageEditDataChatClient(t, adminChat))
	got, err := core.MessagesGetMessageEditData(messageEditDataRequest(peer, messageID))
	if err != nil || got == nil || got.GetCaption() {
		t.Fatalf("admin MessagesGetMessageEditData() = (%v, %v), want editable text result", got, err)
	}
	if adminChat.calls != 1 || adminChat.request.GetSelfId() != messageEditDataUserID || adminChat.request.GetChatId() != chatID {
		t.Fatalf("chat admin request = calls %d request %v", adminChat.calls, adminChat.request)
	}

	memberChat := &messageEditDataChatRPCStub{chat: &mtproto.MutableChat{ChatParticipants: []*mtproto.ImmutableChatParticipant{{
		UserId:          messageEditDataUserID,
		State:           mtproto.ChatMemberStateNormal,
		ParticipantType: mtproto.ChatMemberNormal,
	}}}}
	memberCore := newMessageEditDataTestCore(message, newMessageEditDataChatClient(t, memberChat))
	got, err = memberCore.MessagesGetMessageEditData(messageEditDataRequest(peer, messageID))
	if got != nil || !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("member MessagesGetMessageEditData() = (%v, %v), want CHAT_ADMIN_REQUIRED", got, err)
	}
}

func TestMessagesGetMessageEditDataRejectsChatWithoutProvider(t *testing.T) {
	message := &messageEditDataMessageClient{response: messageEditDataBox(9, mtproto.PEER_CHAT, 77, 84, "")}
	core := newMessageEditDataTestCore(message, nil)
	got, err := core.MessagesGetMessageEditData(messageEditDataRequest(
		mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(), 9))
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetMessageEditData() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
}
