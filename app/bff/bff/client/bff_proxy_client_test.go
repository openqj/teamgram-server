package bff_proxy_client

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type noOpRPCClient struct{}

func (noOpRPCClient) Conn() *grpc.ClientConn { return nil }

func TestInvokeContextUnregisteredRequestReturnsMethodNotImpl(t *testing.T) {
	request := &mtproto.TLEphemeralGetWelcomeMessages{}
	if mtproto.FindRPCContextTuple(request) != nil {
		t.Fatal("test request unexpectedly has an RPC registry entry")
	}

	for name, client := range map[string]*BFFProxyClient{
		"no client route":    {BFFClients: map[string]zrpc.Client{}},
		"stale client route": {BFFClients: map[string]zrpc.Client{"TLEphemeralGetWelcomeMessages": noOpRPCClient{}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := client.InvokeContext(context.Background(), &metadata.RpcMetadata{}, request)
			if got != nil {
				t.Fatalf("InvokeContext() result = %T, want nil", got)
			}
			rpcErr, ok := err.(*mtproto.TLRpcError)
			if !ok || rpcErr.Message() != "METHOD_NOT_IMPL" {
				t.Fatalf("InvokeContext() error = %v, want METHOD_NOT_IMPL", err)
			}
		})
	}
}
