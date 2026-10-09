package service

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/gateway"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type gatewayDelegateStub struct {
	gateway.UnimplementedRPCGatewayServer
	reply *mtproto.Bool
	err   error
}

func (s gatewayDelegateStub) GatewaySendDataToGateway(context.Context, *gateway.TLGatewaySendDataToGateway) (*mtproto.Bool, error) {
	return s.reply, s.err
}

func TestGatewayStreamAckRequiresSuccessfulUnaryDelegate(t *testing.T) {
	for _, test := range []struct {
		name  string
		reply *mtproto.Bool
		err   error
		want  bool
	}{
		{"nil", nil, nil, false},
		{"false", mtproto.BoolFalse, nil, false},
		{"error", mtproto.BoolTrue, errors.New("write failed"), false},
		{"true", mtproto.BoolTrue, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := grpc.NewServer()
			listener := bufconn.Listen(1024 * 1024)
			gateway.RegisterRPCGatewayStreamServer(server, New(nil, gatewayDelegateStub{reply: test.reply, err: test.err}))
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(server.Stop)
			conn, err := grpc.DialContext(context.Background(), "bufnet",
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
				grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			stream, err := gateway.NewRPCGatewayStreamClient(conn).GatewayDataStream(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := stream.Send(&gateway.GatewayStreamRequest{RequestId: "delivery-1", SendData: &gateway.TLGatewaySendDataToGateway{AuthKeyId: 1, SessionId: 2, Payload: []byte("payload")}}); err != nil {
				t.Fatal(err)
			}
			reply, err := stream.Recv()
			if err != nil || reply.GetRequestId() != "delivery-1" || reply.GetSuccess() != test.want {
				t.Fatalf("delegate confirmation=%v err=%v", reply, err)
			}
		})
	}
}
