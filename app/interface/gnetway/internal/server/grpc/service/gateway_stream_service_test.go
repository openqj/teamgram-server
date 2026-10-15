package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

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
	hand  func(context.Context, *gateway.TLGatewaySendDataToGateway) (*mtproto.Bool, error)
}

func (s gatewayDelegateStub) GatewaySendDataToGateway(ctx context.Context, in *gateway.TLGatewaySendDataToGateway) (*mtproto.Bool, error) {
	if s.hand != nil {
		return s.hand(ctx, in)
	}
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

func TestGatewayStreamPreservesConnectionOrder(t *testing.T) {
	firstStarted := make(chan struct{})
	otherStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})

	server := grpc.NewServer()
	listener := bufconn.Listen(1024 * 1024)
	delegate := gatewayDelegateStub{
		reply: mtproto.BoolTrue,
		hand: func(_ context.Context, in *gateway.TLGatewaySendDataToGateway) (*mtproto.Bool, error) {
			switch string(in.GetPayload()) {
			case "first":
				close(firstStarted)
				<-releaseFirst
			case "second":
				close(secondStarted)
			case "other":
				close(otherStarted)
			}
			return mtproto.BoolTrue, nil
		},
	}
	gateway.RegisterRPCGatewayStreamServer(server, New(nil, delegate))
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
	requests := []*gateway.GatewayStreamRequest{
		{RequestId: "first", SendData: &gateway.TLGatewaySendDataToGateway{AuthKeyId: 10, SessionId: 20, Payload: []byte("first")}},
		{RequestId: "second", SendData: &gateway.TLGatewaySendDataToGateway{AuthKeyId: 10, SessionId: 20, Payload: []byte("second")}},
		{RequestId: "other", SendData: &gateway.TLGatewaySendDataToGateway{AuthKeyId: 11, SessionId: 21, Payload: []byte("other")}},
	}
	for _, request := range requests {
		if err := stream.Send(request); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first request did not start")
	}
	select {
	case <-otherStarted:
	case <-time.After(time.Second):
		t.Fatal("different connection was blocked")
	}
	select {
	case <-secondStarted:
		t.Fatal("same connection ran concurrently")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)

	var responseIDs []string
	for len(responseIDs) < len(requests) {
		response, recvErr := stream.Recv()
		if recvErr != nil {
			t.Fatal(recvErr)
		}
		responseIDs = append(responseIDs, response.GetRequestId())
	}
	position := func(id string) int {
		for i, value := range responseIDs {
			if value == id {
				return i
			}
		}
		return -1
	}
	if position("first") < 0 || position("second") < 0 || position("other") < 0 {
		t.Fatalf("responses=%v", responseIDs)
	}
	if position("first") > position("second") {
		t.Fatalf("same-connection responses out of order: %v", responseIDs)
	}
}
