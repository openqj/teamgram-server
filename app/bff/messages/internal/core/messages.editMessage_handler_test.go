package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type editMessageLookupClient struct {
	messageclient.MessageClient
	err         error
	request     *messagepb.TLMessageGetUserMessageList
	peerMessage *mtproto.MessageBox
}

func (c *editMessageLookupClient) MessageGetUserMessageList(_ context.Context, in *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	c.request = in
	return nil, c.err
}

func (c *editMessageLookupClient) MessageGetPeerUserMessage(_ context.Context, _ *messagepb.TLMessageGetPeerUserMessage) (*mtproto.MessageBox, error) {
	return c.peerMessage, nil
}

func TestMessagesEditMessagePropagatesMessageLookupError(t *testing.T) {
	lookupErr := errors.New("message lookup failed")
	client := &editMessageLookupClient{err: lookupErr}
	ctx := context.Background()
	core := &MessagesCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client}},
		MD:     &metadata.RpcMetadata{UserId: 41},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer()

	got, err := core.MessagesEditMessage(&mtproto.TLMessagesEditMessage{Peer: peer, Id: 7})
	if got != nil || err != lookupErr {
		t.Fatalf("MessagesEditMessage() = (%+v, %v), want (nil, original lookup error)", got, err)
	}
	if client.request == nil || client.request.GetUserId() != 41 || len(client.request.GetIdList()) != 1 || client.request.GetIdList()[0] != 7 {
		t.Fatalf("message lookup request = %+v, want user 41 and message id 7", client.request)
	}
}

type editMessageWriter struct {
	msgclient.MsgClient
	request *msgpb.TLMsgEditMessageV2
}

func (c *editMessageWriter) MsgEditMessageV2(_ context.Context, in *msgpb.TLMsgEditMessageV2) (*mtproto.Updates, error) {
	c.request = in
	return &mtproto.Updates{}, nil
}

func TestMessagesEditMessageMapsRecipientViewToBotOutbox(t *testing.T) {
	const botID = int64(900000000001)
	const chatID = int64(7)
	const recipientMessageID = int32(636)
	const senderMessageID = int32(504)
	lookup := &editMessageLookupClient{}
	lookup.MessageClient = lookup
	lookup.peerMessage = &mtproto.MessageBox{
		UserId:          chatID,
		MessageId:       senderMessageID,
		DialogMessageId: 1234,
		SenderUserId:    botID,
		PeerType:        mtproto.PEER_USER,
		PeerId:          chatID,
		Message:         &mtproto.Message{Id: senderMessageID, Message: "old"},
	}
	writer := &editMessageWriter{}
	ctx := context.Background()
	core := &MessagesCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: lookup, MsgClient: writer}},
		MD:     &metadata.RpcMetadata{UserId: botID, IsBot: true},
	}

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: chatID, AccessHash: 99}).To_InputPeer()
	if _, err := core.MessagesEditMessage(&mtproto.TLMessagesEditMessage{Peer: peer, Id: recipientMessageID, Message: wrapperspb.String("new")}); err != nil {
		t.Fatalf("MessagesEditMessage() error = %v", err)
	}
	if writer.request == nil || writer.request.GetDstMessage() == nil || writer.request.GetDstMessage().GetMessageId() != senderMessageID {
		t.Fatalf("edit request = %+v, want sender message id %d", writer.request, senderMessageID)
	}
}
