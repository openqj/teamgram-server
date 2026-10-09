package dao

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	sessionclient "github.com/teamgram/teamgram-server/app/interface/session/client"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sessionReplyClient struct {
	sessionclient.SessionClient
	reply func(context.Context, any) (*mtproto.Bool, error)
}

func (c *sessionReplyClient) SessionPushUpdatesData(ctx context.Context, in *session.TLSessionPushUpdatesData) (*mtproto.Bool, error) {
	return c.reply(ctx, in)
}

func (c *sessionReplyClient) SessionPushSessionUpdatesData(ctx context.Context, in *session.TLSessionPushSessionUpdatesData) (*mtproto.Bool, error) {
	return c.reply(ctx, in)
}

func (c *sessionReplyClient) SessionPushRpcResultData(ctx context.Context, in *session.TLSessionPushRpcResultData) (*mtproto.Bool, error) {
	return c.reply(ctx, in)
}

func testUnarySession(t *testing.T, reply func(context.Context, any) (*mtproto.Bool, error)) *Session {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		serverId: "test-session", client: &sessionReplyClient{reply: reply}, ctx: ctx, cancel: cancel,
		options: SessionOptions{RoutineSize: 2, RoutineChan: 8}, sessionChan: make([]chan sessionDataCtx, 2),
	}
	for i := range s.sessionChan {
		s.sessionChan[i] = make(chan sessionDataCtx, 8)
		s.workers.Add(1)
		go s.process(s.sessionChan[i])
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func sessionPushes(pusher SessionPusher) map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		"updates": func(ctx context.Context) error { return pusher.PushUpdates(ctx, &session.TLSessionPushUpdatesData{}) },
		"sessionUpdates": func(ctx context.Context) error {
			return pusher.PushSessionUpdates(ctx, &session.TLSessionPushSessionUpdatesData{})
		},
		"rpcResult": func(ctx context.Context) error {
			return pusher.PushRpcResult(ctx, &session.TLSessionPushRpcResultData{})
		},
	}
}

func TestUnarySessionRequiresConfirmedSuccess(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		reply *mtproto.Bool
		err   error
	}{
		{"success", mtproto.BoolTrue, nil}, {"false", mtproto.BoolFalse, nil},
		{"nil", nil, nil}, {"RPCFailure", nil, status.Error(codes.Unavailable, "session unavailable")},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			s := testUnarySession(t, func(context.Context, any) (*mtproto.Bool, error) { return fixture.reply, fixture.err })
			for name, push := range sessionPushes(s) {
				t.Run(name, func(t *testing.T) {
					err := push(context.Background())
					if fixture.name == "success" && err != nil || fixture.name != "success" && err == nil {
						t.Fatalf("confirmed reply=%v RPC error=%v push error=%v", fixture.reply, fixture.err, err)
					}
					if fixture.err != nil && status.Code(err) != codes.Unavailable {
						t.Fatalf("RPC error was lost: %v", err)
					}
				})
			}
		})
	}
}

func TestUnarySessionWaitsForReplyAndRecovers(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	s := testUnarySession(t, func(ctx context.Context, _ any) (*mtproto.Bool, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return nil, status.Error(codes.Unavailable, "temporary failure")
			}
		}
		return mtproto.BoolTrue, nil
	})
	result := make(chan error, 1)
	go func() { result <- s.PushUpdates(context.Background(), &session.TLSessionPushUpdatesData{}) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("RPC was not called")
	}
	select {
	case err := <-result:
		t.Fatalf("push completed before RPC reply: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-result; status.Code(err) != codes.Unavailable {
		t.Fatalf("failure=%v", err)
	}
	if err := s.PushUpdates(context.Background(), &session.TLSessionPushUpdatesData{}); err != nil {
		t.Fatalf("recovered session could not retry: %v", err)
	}
}

func TestUnarySessionCancellationAndClose(t *testing.T) {
	for _, closeSession := range []bool{false, true} {
		t.Run(map[bool]string{false: "requestCancel", true: "sessionClose"}[closeSession], func(t *testing.T) {
			entered := make(chan struct{})
			s := testUnarySession(t, func(ctx context.Context, _ any) (*mtproto.Bool, error) {
				close(entered)
				<-ctx.Done()
				return nil, ctx.Err()
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- s.PushUpdates(ctx, &session.TLSessionPushUpdatesData{}) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("RPC was not called")
			}
			if closeSession {
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled push error=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled push blocked")
			}
		})
	}
}

func TestUnarySessionConcurrentPush(t *testing.T) {
	var calls atomic.Int32
	s := testUnarySession(t, func(context.Context, any) (*mtproto.Bool, error) { calls.Add(1); return mtproto.BoolTrue, nil })
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.PushUpdates(context.Background(), &session.TLSessionPushUpdatesData{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 32 {
		t.Fatalf("confirmed calls=%d", calls.Load())
	}
}

func TestUnarySessionRequiresEndpointAndWorkers(t *testing.T) {
	for _, fixture := range []struct {
		conf    zrpc.RpcClientConf
		options SessionOptions
	}{{}, {conf: zrpc.RpcClientConf{Endpoints: []string{"127.0.0.1:1"}}}} {
		if s, err := NewSession(fixture.conf, fixture.options); err == nil || s != nil {
			t.Fatalf("invalid configuration session=%v error=%v", s, err)
		}
	}
}
