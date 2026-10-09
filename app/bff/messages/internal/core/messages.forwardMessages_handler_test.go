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
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type forwardMessageClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	err      error
}

func (s *forwardMessageClientStub) MessageGetUserMessageList(_ context.Context, _ *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	return s.response, s.err
}

type forwardUserClientStub struct {
	userclient.UserClient
	allowed *mtproto.Bool
	request *userpb.TLUserCheckPrivacy
	err     error
}

func (s *forwardUserClientStub) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	s.request = in
	return s.allowed, s.err
}

type forwardMsgClientStub struct {
	msgclient.MsgClient
	response *mtproto.Updates
	err      error
	request  *msgpb.TLMsgSendMessageV2
}

func (s *forwardMsgClientStub) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	s.request = in
	return s.response, s.err
}

func newForwardTestCore(messages messageclient.MessageClient, users userclient.UserClient, sender msgclient.MsgClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			UserClient:    users,
			MsgClient:     sender,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
	}
}

func forwardSelfBox(id int32) *mtproto.MessageBox {
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:       42,
		MessageId:    id,
		SenderUserId: 42,
		PeerType:     mtproto.PEER_USER,
		PeerId:       42,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id:      id,
			FromId:  mtproto.MakePeerUser(42),
			PeerId:  mtproto.MakePeerUser(42),
			Date:    100,
			Message: "forward me",
		}).To_Message(),
	}).To_MessageBox()
}

func forwardSelfRequest() *mtproto.TLMessagesForwardMessages {
	return &mtproto.TLMessagesForwardMessages{
		FromPeer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:       []int32{100},
		RandomId: []int64{9001},
		ToPeer:   mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	}
}

func TestMessagesForwardMessagesSendsStoredMessageAndReturnsUpdates(t *testing.T) {
	sender := &forwardMsgClientStub{response: mtproto.MakeEmptyUpdates()}
	core := newForwardTestCore(
		&forwardMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{forwardSelfBox(100)}}},
		&forwardUserClientStub{allowed: mtproto.BoolTrue},
		sender,
	)

	got, err := core.MessagesForwardMessages(forwardSelfRequest())
	if err != nil || got != sender.response {
		t.Fatalf("MessagesForwardMessages() = (%v, %v), want typed updates", got, err)
	}
	if sender.request == nil || len(sender.request.GetMessage()) != 1 {
		t.Fatalf("MsgSendMessageV2 request = %v, want one forwarded message", sender.request)
	}
	forwarded := sender.request.GetMessage()[0]
	if forwarded.GetRandomId() != 9001 || forwarded.GetMessage().GetMessage() != "forward me" || forwarded.GetMessage().GetPeerId().GetUserId() != 42 {
		t.Fatalf("forwarded outbox = %v, want random_id/text/self peer", forwarded)
	}
}

func TestMessagesForwardMessagesFailsClosedOnNilSendResponse(t *testing.T) {
	sender := &forwardMsgClientStub{}
	core := newForwardTestCore(
		&forwardMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{forwardSelfBox(100)}}},
		&forwardUserClientStub{allowed: mtproto.BoolTrue},
		sender,
	)

	got, err := core.MessagesForwardMessages(forwardSelfRequest())
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesForwardMessages(nil send response) = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesForwardMessagesFailsClosedOnNilMessageList(t *testing.T) {
	core := newForwardTestCore(
		&forwardMessageClientStub{},
		&forwardUserClientStub{allowed: mtproto.BoolTrue},
		&forwardMsgClientStub{response: mtproto.MakeEmptyUpdates()},
	)

	got, err := core.MessagesForwardMessages(forwardSelfRequest())
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesForwardMessages(nil message list) = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}

func TestForwardPrivacyUsesAuthoritativeUserService(t *testing.T) {
	wantErr := errors.New("postgres privacy unavailable")
	for _, tc := range []struct {
		name         string
		allowed      *mtproto.Bool
		err, wantErr error
		want         bool
	}{
		{name: "allowed", allowed: mtproto.BoolTrue, want: true},
		{name: "denied", allowed: mtproto.BoolFalse},
		{name: "query failure", err: wantErr, wantErr: wantErr},
		{name: "nil response", wantErr: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users := &forwardUserClientStub{allowed: tc.allowed, err: tc.err}
			c := newForwardTestCore(nil, users, nil)
			got, err := c.checkForwardPrivacy(context.Background(), 7, 42)
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Fatalf("privacy = (%v, %v), want (%v, %v)", got, err, tc.want, tc.wantErr)
			}
			if in := users.request; in.GetUserId() != 7 || in.GetPeerId() != 42 || in.GetKeyType() != mtproto.FORWARDS {
				t.Fatalf("forward privacy request = %v", in)
			}
		})
	}
}

func TestMessagesForwardPrivacyFailurePreventsSend(t *testing.T) {
	wantErr := errors.New("postgres privacy unavailable")
	sender := &forwardMsgClientStub{}
	c := newForwardTestCore(&forwardMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{forwardSelfBox(100)}}}, &forwardUserClientStub{err: wantErr}, sender)
	if got, err := c.MessagesForwardMessages(forwardSelfRequest()); got != nil || !errors.Is(err, wantErr) || sender.request != nil {
		t.Fatalf("forward = (%v, %v), sent=%v", got, err, sender.request)
	}
}
