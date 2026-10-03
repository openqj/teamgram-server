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

type updatePinnedMessageClientStub struct {
	msgclient.MsgClient
	updates *mtproto.Updates
	err     error
}

func (s *updatePinnedMessageClientStub) MsgUpdatePinnedMessage(_ context.Context, _ *msgpb.TLMsgUpdatePinnedMessage) (*mtproto.Updates, error) {
	return s.updates, s.err
}

func newUpdatePinnedMessageTestCore(msg msgclient.MsgClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MsgClient: msg}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 9001},
	}
}

func updatePinnedMessageRequest() *mtproto.TLMessagesUpdatePinnedMessage {
	return &mtproto.TLMessagesUpdatePinnedMessage{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
		Id:   7,
	}
}

func TestMessagesUpdatePinnedMessagePropagatesProviderErrors(t *testing.T) {
	providerErr := errors.New("message provider unavailable")
	got, err := newUpdatePinnedMessageTestCore(&updatePinnedMessageClientStub{err: providerErr}).MessagesUpdatePinnedMessage(updatePinnedMessageRequest())
	if !errors.Is(err, providerErr) || got != nil {
		t.Fatalf("MessagesUpdatePinnedMessage() = (%v, %v), want propagated provider error", got, err)
	}

	got, err = newUpdatePinnedMessageTestCore(&updatePinnedMessageClientStub{}).MessagesUpdatePinnedMessage(updatePinnedMessageRequest())
	if err != mtproto.ErrInternalServerError || got != nil {
		t.Fatalf("MessagesUpdatePinnedMessage() = (%v, %v), want INTERNAL_SERVER_ERROR for nil response", got, err)
	}
}

func TestMessagesUpdatePinnedMessageReturnsTypedProviderResult(t *testing.T) {
	updates := mtproto.MakeEmptyUpdates()
	got, err := newUpdatePinnedMessageTestCore(&updatePinnedMessageClientStub{updates: updates}).MessagesUpdatePinnedMessage(updatePinnedMessageRequest())
	if err != nil || got != updates {
		t.Fatalf("MessagesUpdatePinnedMessage() = (%v, %v), want provider updates", got, err)
	}
}
