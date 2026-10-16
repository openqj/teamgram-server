package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

const messagesGetRichMessageCallerID int64 = 41

type messagesGetRichMessageClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	err      error
	calls    int
}

func (s *messagesGetRichMessageClientStub) MessageGetUserMessageList(_ context.Context, _ *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	s.calls++
	return s.response, s.err
}

type messagesGetRichMessageUserClientStub struct {
	userclient.UserClient
	response *userpb.Vector_ImmutableUser
	err      error
	calls    int
}

func (s *messagesGetRichMessageUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.calls++
	return s.response, s.err
}

func newMessagesGetRichMessageTestCore(messages messageclient.MessageClient, users userclient.UserClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			UserClient:    users,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: messagesGetRichMessageCallerID},
	}
}

func messagesGetRichMessageUserPeer(userID, accessHash int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId:     userID,
		AccessHash: accessHash,
	}).To_InputPeer()
}

func messagesGetRichMessageChannelPeer(channelID, accessHash int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  channelID,
		AccessHash: accessHash,
	}).To_InputPeer()
}

func messagesGetRichMessageBox(id int32, peer *mtproto.Peer, senderID int64) *mtproto.MessageBox {
	peerUtil := mtproto.FromPeer(peer)
	var fromID *mtproto.Peer
	if senderID > 0 {
		fromID = mtproto.MakePeerUser(senderID)
	}
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      id,
		FromId:  fromID,
		PeerId:  peer,
		Message: "rich message fixture",
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId:    id,
		SenderUserId: senderID,
		PeerType:     peerUtil.PeerType,
		PeerId:       peerUtil.PeerId,
		Message:      message,
	}).To_MessageBox()
}

func messagesGetRichMessageUsers(ids ...int64) *userpb.Vector_ImmutableUser {
	datas := make([]*mtproto.ImmutableUser, 0, len(ids))
	for _, id := range ids {
		datas = append(datas, mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: id, AccessHash: id * 10},
		}).To_ImmutableUser())
	}
	return &userpb.Vector_ImmutableUser{Datas: datas}
}

func messagesGetRichMessageRequest(peer *mtproto.InputPeer, id int32) *mtproto.TLMessagesGetRichMessage {
	return &mtproto.TLMessagesGetRichMessage{Peer: peer, Id: id}
}

func TestMessagesGetRichMessageReturnsHydratedUserMessage(t *testing.T) {
	client := &messagesGetRichMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
		messagesGetRichMessageBox(10, mtproto.MakePeerUser(84), 84),
	}}}
	users := &messagesGetRichMessageUserClientStub{response: messagesGetRichMessageUsers(messagesGetRichMessageCallerID, 84)}
	core := newMessagesGetRichMessageTestCore(client, users)

	got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 840), 10))
	if err != nil {
		t.Fatalf("MessagesGetRichMessage() error = %v", err)
	}
	if got == nil || len(got.GetMessages()) != 1 {
		t.Fatalf("MessagesGetRichMessage() = %+v, want one message", got)
	}
	if message := got.GetMessages()[0]; message == nil || message.GetId() != 10 || message.GetMessage() != "rich message fixture" {
		t.Fatalf("message = %+v, want hydrated message 10", got.GetMessages()[0])
	}
	if client.calls != 1 || users.calls != 1 {
		t.Fatalf("provider calls = message:%d users:%d, want one each", client.calls, users.calls)
	}
}

func TestMessagesGetRichMessageReturnsEmptyForUnknownID(t *testing.T) {
	client := &messagesGetRichMessageClientStub{response: &messagepb.Vector_MessageBox{}}
	core := newMessagesGetRichMessageTestCore(client, nil)

	got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 840), 11))
	if err != nil {
		t.Fatalf("MessagesGetRichMessage() error = %v", err)
	}
	if got == nil || len(got.GetMessages()) != 1 || got.GetMessages()[0] == nil || got.GetMessages()[0].GetPredicateName() != mtproto.Predicate_messageEmpty || got.GetMessages()[0].GetId() != 11 {
		t.Fatalf("messages = %+v, want messageEmpty id 11", got.GetMessages())
	}
	if len(got.GetUsers()) != 0 || len(got.GetChats()) != 0 {
		t.Fatalf("entities = users:%d chats:%d, want empty", len(got.GetUsers()), len(got.GetChats()))
	}
}

func TestMessagesGetRichMessageRejectsZeroUserAccessHashBeforeLookup(t *testing.T) {
	client := &messagesGetRichMessageClientStub{}
	core := newMessagesGetRichMessageTestCore(client, nil)

	got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 0), 10))
	if got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/PEER_ID_INVALID", got, err)
	}
	if client.calls != 0 {
		t.Fatalf("message provider calls = %d, want zero", client.calls)
	}
}

func TestMessagesGetRichMessageRejectsInvalidChannelPeerBeforeLookup(t *testing.T) {
	for _, tc := range []struct {
		name string
		peer *mtproto.InputPeer
	}{
		{name: "zero access hash", peer: messagesGetRichMessageChannelPeer(9001, 0)},
		{name: "zero channel id", peer: messagesGetRichMessageChannelPeer(0, 77)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &messagesGetRichMessageClientStub{}
			core := newMessagesGetRichMessageTestCore(client, nil)

			got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(tc.peer, 10))
			if got != nil || !errors.Is(err, mtproto.ErrChannelInvalid) {
				t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/CHANNEL_INVALID", got, err)
			}
			if client.calls != 0 {
				t.Fatalf("message provider calls = %d, want zero", client.calls)
			}
		})
	}
}

func TestMessagesGetRichMessageRequiresMessageProvider(t *testing.T) {
	core := newMessagesGetRichMessageTestCore(nil, nil)

	got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 840), 10))
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetRichMessageRejectsNilProviderList(t *testing.T) {
	client := &messagesGetRichMessageClientStub{}
	core := newMessagesGetRichMessageTestCore(client, nil)

	got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 840), 10))
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesGetRichMessageRejectsMalformedProviderBoxes(t *testing.T) {
	validPeer := mtproto.MakePeerUser(84)
	tests := []struct {
		name string
		box  *mtproto.MessageBox
	}{
		{name: "nil box", box: nil},
		{name: "nil message", box: mtproto.MakeTLMessageBox(&mtproto.MessageBox{MessageId: 10, PeerType: mtproto.PEER_USER, PeerId: 84}).To_MessageBox()},
		{name: "mismatched message id", box: messagesGetRichMessageBox(10, validPeer, 84)},
		{name: "invalid peer type", box: mtproto.MakeTLMessageBox(&mtproto.MessageBox{
			MessageId: 10,
			PeerType:  99,
			PeerId:    84,
			Message:   mtproto.MakeTLMessage(&mtproto.Message{Id: 10, PeerId: validPeer}).To_Message(),
		}).To_MessageBox()},
		{name: "invalid peer id", box: mtproto.MakeTLMessageBox(&mtproto.MessageBox{
			MessageId: 10,
			PeerType:  mtproto.PEER_USER,
			PeerId:    0,
			Message:   mtproto.MakeTLMessage(&mtproto.Message{Id: 10, PeerId: validPeer}).To_Message(),
		}).To_MessageBox()},
	}
	// Replace the otherwise valid fixture's message id so this case exercises
	// the box/message identity check without relying on an invalid constructor.
	tests[2].box.GetMessage().Id = 11

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &messagesGetRichMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{tc.box}}}
			users := &messagesGetRichMessageUserClientStub{response: messagesGetRichMessageUsers(messagesGetRichMessageCallerID, 84)}
			core := newMessagesGetRichMessageTestCore(client, users)

			got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 840), 10))
			if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
			}
		})
	}
}

func TestMessagesGetRichMessageValidBoxDoesNotHideLaterMalformedBox(t *testing.T) {
	client := &messagesGetRichMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
		messagesGetRichMessageBox(10, mtproto.MakePeerUser(84), 84),
		nil,
	}}}
	users := &messagesGetRichMessageUserClientStub{response: messagesGetRichMessageUsers(messagesGetRichMessageCallerID, 84)}
	core := newMessagesGetRichMessageTestCore(client, users)

	got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 840), 10))
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
	}
	if users.calls != 0 {
		t.Fatalf("user provider calls = %d, want zero before malformed response is rejected", users.calls)
	}
}

func TestMessagesGetRichMessagePropagatesProviderError(t *testing.T) {
	lookupErr := errors.New("message lookup unavailable")
	client := &messagesGetRichMessageClientStub{err: lookupErr}
	core := newMessagesGetRichMessageTestCore(client, nil)

	got, err := core.MessagesGetRichMessage(messagesGetRichMessageRequest(messagesGetRichMessageUserPeer(84, 840), 10))
	if got != nil || !errors.Is(err, lookupErr) {
		t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/provider error", got, err)
	}
}

func TestMessagesGetRichMessageNilRequestDoesNotPanic(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: messagesGetRichMessageCallerID}}

	got, err := core.MessagesGetRichMessage(nil)
	if got != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("MessagesGetRichMessage() = (%v, %v), want nil/INPUT_CONSTRUCTOR_INVALID", got, err)
	}
}
