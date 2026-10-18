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
	"github.com/zeromicro/go-zero/core/logx"
)

type nilUpdatesSendClient struct {
	msgclient.MsgClient
}

func (*nilUpdatesSendClient) MsgSendMessageV2(context.Context, *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	return nil, nil
}

func newNilUpdatesSendCore() *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MsgClient: &nilUpdatesSendClient{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func TestMessagesSendMessageFailsClosedOnNilProviderResponse(t *testing.T) {
	core := newNilUpdatesSendCore()
	got, err := core.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:     mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Message:  "hello",
		RandomId: 1,
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesSendMessage(nil provider response) = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesSendMediaFailsClosedOnNilProviderResponse(t *testing.T) {
	core := newNilUpdatesSendCore()
	got, err := core.MessagesSendMedia(&mtproto.TLMessagesSendMedia{
		Peer:     mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Media:    mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia(),
		RandomId: 2,
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesSendMedia(nil provider response) = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}
