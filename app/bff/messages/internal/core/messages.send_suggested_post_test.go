package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/zeromicro/go-zero/core/logx"
)

type suggestedPostSendClient struct {
	msgclient.MsgClient
	request  *msgpb.TLMsgSendMessageV2
	response *mtproto.Updates
}

func (s *suggestedPostSendClient) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	s.request = in
	return s.response, nil
}

func newSuggestedPostSendCore(sender msgclient.MsgClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MsgClient: sender}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
	}
}

func TestMessagesSendMessagePreservesSuggestedPost(t *testing.T) {
	sender := &suggestedPostSendClient{response: mtproto.MakeEmptyUpdates()}
	core := newSuggestedPostSendCore(sender)
	want := &mtproto.SuggestedPost{}

	if _, err := core.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:          mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Message:       "hello",
		RandomId:      1,
		SuggestedPost: want,
	}); err != nil {
		t.Fatalf("MessagesSendMessage() error = %v", err)
	}
	if sender.request == nil || len(sender.request.GetMessage()) != 1 || sender.request.GetMessage()[0].GetMessage().GetSuggestedPost() != want {
		t.Fatalf("sent suggested post = %v, want request object", sender.request)
	}
	savedPeer := sender.request.GetMessage()[0].GetMessage().GetSavedPeerId()
	if savedPeer == nil || savedPeer.GetPredicateName() != mtproto.Predicate_peerUser || savedPeer.GetUserId() != 42 {
		t.Fatalf("saved peer = %v, want peerUser(42)", savedPeer)
	}
}

func TestMessagesSendMediaPreservesSuggestedPost(t *testing.T) {
	sender := &suggestedPostSendClient{response: mtproto.MakeEmptyUpdates()}
	core := newSuggestedPostSendCore(sender)
	want := &mtproto.SuggestedPost{}

	if _, err := core.MessagesSendMedia(&mtproto.TLMessagesSendMedia{
		Peer:          mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Media:         mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia(),
		RandomId:      2,
		SuggestedPost: want,
	}); err != nil {
		t.Fatalf("MessagesSendMedia() error = %v", err)
	}
	if sender.request == nil || len(sender.request.GetMessage()) != 1 || sender.request.GetMessage()[0].GetMessage().GetSuggestedPost() != want {
		t.Fatalf("sent media suggested post = %v, want request object", sender.request)
	}
	savedPeer := sender.request.GetMessage()[0].GetMessage().GetSavedPeerId()
	if savedPeer == nil || savedPeer.GetPredicateName() != mtproto.Predicate_peerUser || savedPeer.GetUserId() != 42 {
		t.Fatalf("saved peer = %v, want peerUser(42)", savedPeer)
	}
}

func TestMessagesSendMediaRejectsSuggestedPostForChannelStorage(t *testing.T) {
	core := newSuggestedPostSendCore(&suggestedPostSendClient{response: mtproto.MakeEmptyUpdates()})
	_, err := core.MessagesSendMedia(&mtproto.TLMessagesSendMedia{
		Peer:          mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 7, AccessHash: 7}).To_InputPeer(),
		Media:         mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia(),
		SuggestedPost: &mtproto.SuggestedPost{},
	})
	if err != mtproto.ErrMethodNotImpl {
		t.Fatalf("channel media suggested post = %v, want METHOD_NOT_IMPL", err)
	}
}
