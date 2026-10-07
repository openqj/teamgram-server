package userclient

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/zrpc"
)

type BotRegistryClient interface {
	SetBotInfo(context.Context, *user.BotRegistrySetBotInfoRequest) (*mtproto.Bool, error)
}

type botRegistryClient struct {
	client zrpc.Client
}

func NewBotRegistryClient(client zrpc.Client) BotRegistryClient {
	return &botRegistryClient{client: client}
}

func (c *botRegistryClient) SetBotInfo(ctx context.Context, in *user.BotRegistrySetBotInfoRequest) (*mtproto.Bool, error) {
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		ctx, _ = metadata.RpcMetadataToOutgoing(ctx, md)
	}
	return user.NewRPCBotRegistryClient(c.client.Conn()).SetBotInfo(ctx, in)
}
