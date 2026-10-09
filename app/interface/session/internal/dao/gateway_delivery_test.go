package dao

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/gateway"
	"google.golang.org/grpc"
)

type gatewayReplyStub struct {
	reply *mtproto.Bool
	err   error
}

func (s gatewayReplyStub) GatewaySendDataToGateway(context.Context, *gateway.TLGatewaySendDataToGateway) (*mtproto.Bool, error) {
	return s.reply, s.err
}

func TestGatewayUnaryRequiresPositiveReply(t *testing.T) {
	for _, test := range []struct {
		name  string
		reply *mtproto.Bool
		err   error
		want  bool
	}{
		{"nil", nil, nil, false},
		{"false", mtproto.BoolFalse, nil, false},
		{"error", mtproto.BoolTrue, errors.New("gateway unavailable"), false},
		{"true", mtproto.BoolTrue, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &Gateway{serverId: "test", client: gatewayReplyStub{test.reply, test.err}}
			ok, err := client.SendDataToGate(context.Background(), 1, 2, []byte("payload"))
			if ok != test.want || (err == nil) != test.want {
				t.Fatalf("delivery ok=%v err=%v", ok, err)
			}
		})
	}
}

type gatewayStreamStub struct {
	gateway.UnimplementedRPCGatewayStreamServer
	handle func(grpc.BidiStreamingServer[gateway.GatewayStreamRequest, gateway.GatewayStreamResponse]) error
}

func (s gatewayStreamStub) GatewayDataStream(stream grpc.BidiStreamingServer[gateway.GatewayStreamRequest, gateway.GatewayStreamResponse]) error {
	return s.handle(stream)
}

func deliveryStreamFixture(t *testing.T, handle func(grpc.BidiStreamingServer[gateway.GatewayStreamRequest, gateway.GatewayStreamResponse]) error) (*StreamingGateway, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	gateway.RegisterRPCGatewayStreamServer(server, gatewayStreamStub{handle: handle})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	manager := NewStreamingGateway()
	t.Cleanup(manager.Close)
	return manager, listener.Addr().String()
}

func TestGatewayStreamWaitsForMatchingAck(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "success", false: "rejected"}[success], func(t *testing.T) {
			seen := make(chan struct{})
			release := make(chan struct{})
			manager, address := deliveryStreamFixture(t, func(stream grpc.BidiStreamingServer[gateway.GatewayStreamRequest, gateway.GatewayStreamResponse]) error {
				request, err := stream.Recv()
				if err != nil {
					return err
				}
				if err := stream.Send(&gateway.GatewayStreamResponse{RequestId: "unrelated", Success: true}); err != nil {
					return err
				}
				close(seen)
				select {
				case <-release:
				case <-stream.Context().Done():
					return stream.Context().Err()
				}
				if err := stream.Send(&gateway.GatewayStreamResponse{RequestId: request.GetRequestId(), Success: success}); err != nil {
					return err
				}
				<-stream.Context().Done()
				return stream.Context().Err()
			})
			done := make(chan error, 1)
			go func() {
				_, err := manager.SendDataToGateway(context.Background(), address, 1, 2, []byte("payload"))
				done <- err
			}()
			select {
			case <-seen:
			case <-time.After(2 * time.Second):
				t.Fatal("stream request was not received")
			}
			select {
			case err := <-done:
				t.Fatalf("queue admission/unrelated ACK completed delivery: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			select {
			case err := <-done:
				if (err == nil) != success {
					t.Fatalf("matching ACK success=%v err=%v", success, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("matching ACK did not complete delivery")
			}
		})
	}
}

func TestGatewayStreamDisconnectAndCancellationFailDelivery(t *testing.T) {
	for _, disconnect := range []bool{true, false} {
		t.Run(map[bool]string{true: "disconnect", false: "canceled"}[disconnect], func(t *testing.T) {
			seen := make(chan struct{})
			manager, address := deliveryStreamFixture(t, func(stream grpc.BidiStreamingServer[gateway.GatewayStreamRequest, gateway.GatewayStreamResponse]) error {
				if _, err := stream.Recv(); err != nil {
					return err
				}
				close(seen)
				if disconnect {
					return errors.New("gateway disconnected")
				}
				<-stream.Context().Done()
				return stream.Context().Err()
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				ok, err := manager.SendDataToGateway(ctx, address, 1, 2, []byte("payload"))
				if ok && err == nil {
					err = errors.New("unconfirmed delivery reported successful")
				}
				done <- err
			}()
			select {
			case <-seen:
			case <-time.After(2 * time.Second):
				t.Fatal("request was not received")
			}
			if !disconnect {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("failed stream delivery was confirmed")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("failed stream delivery remained pending")
			}
		})
	}
}
