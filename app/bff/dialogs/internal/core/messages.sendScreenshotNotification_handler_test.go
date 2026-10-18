package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
)

type screenshotMessageClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	err      error
}

func (s *screenshotMessageClientStub) MessageGetUserMessageList(context.Context, *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	return s.response, s.err
}

type screenshotMsgClientStub struct {
	msgclient.MsgClient
	response *mtproto.Updates
	err      error
	request  *msgpb.TLMsgSendMessageV2
}

func (s *screenshotMsgClientStub) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	s.request = in
	return s.response, s.err
}

func newScreenshotTestCore(messages messageclient.MessageClient, sender msgclient.MsgClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			MsgClient:     sender,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
	}
}

func screenshotReplyBox() *mtproto.MessageBox {
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:    42,
		MessageId: 77,
		PeerType:  mtproto.PEER_USER,
		PeerId:    42,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id:      77,
			FromId:  mtproto.MakePeerUser(42),
			PeerId:  mtproto.MakePeerUser(42),
			Message: "reply",
		}).To_Message(),
	}).To_MessageBox()
}

func screenshotRequest() *mtproto.TLMessagesSendScreenshotNotification {
	return &mtproto.TLMessagesSendScreenshotNotification{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: 77,
		}).To_InputReplyTo(),
		RandomId: 9001,
	}
}

func TestMessagesSendScreenshotNotificationPersistsServiceMessage(t *testing.T) {
	sender := &screenshotMsgClientStub{response: mtproto.MakeEmptyUpdates()}
	core := newScreenshotTestCore(
		&screenshotMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{screenshotReplyBox()}}},
		sender,
	)

	got, err := core.MessagesSendScreenshotNotification(screenshotRequest())
	if err != nil || got != sender.response {
		t.Fatalf("MessagesSendScreenshotNotification() = (%v, %v), want typed updates", got, err)
	}
	if sender.request == nil || len(sender.request.GetMessage()) != 1 {
		t.Fatalf("MsgSendMessageV2 request = %v, want one service message", sender.request)
	}
	out := sender.request.GetMessage()[0]
	if out.GetRandomId() != 9001 || out.GetMessage().GetAction().GetPredicateName() != mtproto.Predicate_messageActionScreenshotTaken {
		t.Fatalf("outbox = %v, want screenshot action and random id", out)
	}
	if out.GetMessage().GetReplyTo().GetReplyToMsgId() != 77 || out.GetMessage().GetPeerId().GetUserId() != 42 {
		t.Fatalf("service message = %v, want reply 77 in self peer", out.GetMessage())
	}
}

func TestMessagesSendScreenshotNotificationRejectsInvalidReplyAndProviderFailure(t *testing.T) {
	core := newScreenshotTestCore(
		&screenshotMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{screenshotReplyBox()}}},
		&screenshotMsgClientStub{response: mtproto.MakeEmptyUpdates()},
	)
	request := screenshotRequest()
	request.ReplyTo = mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{ReplyToMsgId: 78}).To_InputReplyTo()
	if _, err := core.MessagesSendScreenshotNotification(request); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("invalid reply error = %v, want MESSAGE_ID_INVALID", err)
	}

	wantErr := errors.New("msg service unavailable")
	request = screenshotRequest()
	_, err := newScreenshotTestCore(
		&screenshotMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{screenshotReplyBox()}}},
		&screenshotMsgClientStub{err: wantErr},
	).MessagesSendScreenshotNotification(request)
	if !errors.Is(err, wantErr) {
		t.Fatalf("provider error = %v, want %v", err, wantErr)
	}
}
