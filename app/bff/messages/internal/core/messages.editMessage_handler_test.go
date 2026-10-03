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
	"github.com/zeromicro/go-zero/core/logx"
)

type editMessageLookupClient struct {
	messageclient.MessageClient
	err     error
	request *messagepb.TLMessageGetUserMessageList
}

func (c *editMessageLookupClient) MessageGetUserMessageList(_ context.Context, in *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	c.request = in
	return nil, c.err
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
