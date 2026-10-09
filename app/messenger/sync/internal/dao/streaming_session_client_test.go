package dao

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sessionTestServer struct {
	session.UnimplementedRPCSessionServer
	session.UnimplementedRPCSessionStreamServer
	stream func(grpc.BidiStreamingServer[session.SessionStreamRequest, session.SessionStreamResponse]) error
	reply  func(context.Context) (*mtproto.Bool, error)
}

func (s *sessionTestServer) SessionDataStream(stream grpc.BidiStreamingServer[session.SessionStreamRequest, session.SessionStreamResponse]) error {
	return s.stream(stream)
}

func (s *sessionTestServer) SessionPushUpdatesData(ctx context.Context, _ *session.TLSessionPushUpdatesData) (*mtproto.Bool, error) {
	return s.reply(ctx)
}

func (s *sessionTestServer) SessionPushSessionUpdatesData(ctx context.Context, _ *session.TLSessionPushSessionUpdatesData) (*mtproto.Bool, error) {
	return s.reply(ctx)
}

func (s *sessionTestServer) SessionPushRpcResultData(ctx context.Context, _ *session.TLSessionPushRpcResultData) (*mtproto.Bool, error) {
	return s.reply(ctx)
}

func testStreamingSession(t *testing.T, service *sessionTestServer) *StreamingSession {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	session.RegisterRPCSessionServer(server, service)
	session.RegisterRPCSessionStreamServer(server, service)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	client, err := NewStreamingSession(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func ackResponse(requestID string, success bool) *session.SessionStreamResponse {
	return &session.SessionStreamResponse{RequestId: requestID, Payload: &session.SessionStreamResponse_Ack{Ack: &session.SessionStreamAck{Success: success}}}
}

func TestStreamingSessionRequiresMatchingSuccessfulAck(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		response *session.SessionStreamResponse
		code     codes.Code
	}{
		{"success", ackResponse("", true), codes.OK},
		{"falseAck", ackResponse("", false), codes.Unknown},
		{"missingAck", &session.SessionStreamResponse{}, codes.Unknown},
		{"RPCFailure", &session.SessionStreamResponse{Payload: &session.SessionStreamResponse_Error{Error: &session.SessionStreamError{Code: int32(codes.PermissionDenied), Message: "gateway rejected push"}}}, codes.PermissionDenied},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			client := testStreamingSession(t, &sessionTestServer{stream: func(stream grpc.BidiStreamingServer[session.SessionStreamRequest, session.SessionStreamResponse]) error {
				for {
					request, err := stream.Recv()
					if err != nil {
						return err
					}
					once.Do(func() { close(entered) })
					if err := stream.Send(ackResponse("unrelated-request", true)); err != nil {
						return err
					}
					select {
					case <-release:
					case <-stream.Context().Done():
						return stream.Context().Err()
					}
					response := &session.SessionStreamResponse{RequestId: request.GetRequestId(), Payload: fixture.response.GetPayload()}
					if err := stream.Send(response); err != nil {
						return err
					}
				}
			}})
			result := make(chan error, 1)
			go func() { result <- client.PushUpdates(context.Background(), &session.TLSessionPushUpdatesData{}) }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("stream push was not received")
			}
			select {
			case err := <-result:
				t.Fatalf("push completed without matching ACK: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			close(release)
			if err := <-result; status.Code(err) != fixture.code {
				t.Fatalf("ACK error=%v want code=%v", err, fixture.code)
			}
			for name, push := range sessionPushes(client) {
				t.Run(name, func(t *testing.T) {
					if err := push(context.Background()); status.Code(err) != fixture.code {
						t.Fatalf("ACK error=%v want code=%v", err, fixture.code)
					}
				})
			}
		})
	}
}

func TestStreamingSessionBrokenStreamRetriesWithUnaryConfirmation(t *testing.T) {
	var unaryCalls atomic.Int32
	var successful atomic.Bool
	client := testStreamingSession(t, &sessionTestServer{
		stream: func(stream grpc.BidiStreamingServer[session.SessionStreamRequest, session.SessionStreamResponse]) error {
			if _, err := stream.Recv(); err != nil {
				return err
			}
			return status.Error(codes.Unavailable, "stream reset")
		},
		reply: func(context.Context) (*mtproto.Bool, error) {
			unaryCalls.Add(1)
			return mtproto.ToBool(successful.Load()), nil
		},
	})
	if err := client.PushUpdates(context.Background(), &session.TLSessionPushUpdatesData{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("broken stream returned success: %v", err)
	}
	for name, push := range sessionPushes(client) {
		t.Run(name, func(t *testing.T) {
			if err := push(context.Background()); err == nil {
				t.Fatal("unary fallback accepted false reply")
			}
			successful.Store(true)
			if err := push(context.Background()); err != nil {
				t.Fatalf("unary fallback failed after recovery: %v", err)
			}
			successful.Store(false)
		})
	}
	if unaryCalls.Load() != 6 {
		t.Fatalf("unary fallback calls=%d", unaryCalls.Load())
	}
}

func TestStreamingSessionCancellationAndClose(t *testing.T) {
	for _, closeSession := range []bool{false, true} {
		t.Run(map[bool]string{false: "requestCancel", true: "sessionClose"}[closeSession], func(t *testing.T) {
			entered := make(chan struct{})
			client := testStreamingSession(t, &sessionTestServer{stream: func(stream grpc.BidiStreamingServer[session.SessionStreamRequest, session.SessionStreamResponse]) error {
				if _, err := stream.Recv(); err != nil {
					return err
				}
				close(entered)
				<-stream.Context().Done()
				return stream.Context().Err()
			}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- client.PushUpdates(ctx, &session.TLSessionPushUpdatesData{}) }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("stream push was not received")
			}
			if closeSession {
				if err := client.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) && status.Code(err) != codes.Canceled {
					t.Fatalf("cancelled push error=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled push blocked")
			}
			client.pendingMu.Lock()
			pending := len(client.pending)
			client.pendingMu.Unlock()
			if pending != 0 {
				t.Fatalf("cancelled requests retained=%d", pending)
			}
			if closeSession {
				if err := client.PushUpdates(context.Background(), &session.TLSessionPushUpdatesData{}); err == nil {
					t.Fatal("closed session accepted push")
				}
			}
		})
	}
}

func TestStreamingSessionConcurrentPush(t *testing.T) {
	var calls atomic.Int32
	client := testStreamingSession(t, &sessionTestServer{stream: func(stream grpc.BidiStreamingServer[session.SessionStreamRequest, session.SessionStreamResponse]) error {
		for {
			request, err := stream.Recv()
			if err != nil {
				return err
			}
			calls.Add(1)
			if err := stream.Send(ackResponse(request.GetRequestId(), true)); err != nil {
				return err
			}
		}
	}})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.PushUpdates(context.Background(), &session.TLSessionPushUpdatesData{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 32 {
		t.Fatalf("confirmed stream calls=%d", calls.Load())
	}
}
