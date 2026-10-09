// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package dao

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/session"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcStatus "google.golang.org/grpc/status"
)

const (
	syncStreamSendBufSize       = 8192
	syncInitialReconnectBackoff = 100 * time.Millisecond
	syncMaxReconnectBackoff     = 10 * time.Second
)

// StreamingSession implements the same push interface as Session but uses
// bidirectional gRPC streaming instead of unary RPCs.
type StreamingSession struct {
	serverId     string
	conn         *grpc.ClientConn
	stream       grpc.BidiStreamingClient[session.SessionStreamRequest, session.SessionStreamResponse]
	sendCh       chan *session.SessionStreamRequest
	ctx          context.Context
	cancel       context.CancelFunc
	closed       atomic.Int32
	unavailable  atomic.Bool
	reqCounter   atomic.Int64
	streamCancel context.CancelFunc
	pendingMu    sync.Mutex
	pending      map[string]chan error
}

func NewStreamingSession(addr string) (*StreamingSession, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithReadBufferSize(16*1024*1024),
		grpc.WithWriteBufferSize(16*1024*1024),
	)
	if err != nil {
		return nil, fmt.Errorf("dial session %s: %w", addr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	streamCtx, streamCancel := context.WithCancel(ctx)
	client := session.NewRPCSessionStreamClient(conn)
	stream, err := client.SessionDataStream(streamCtx)
	if err != nil {
		streamCancel()
		cancel()
		conn.Close()
		return nil, fmt.Errorf("open stream to session %s: %w", addr, err)
	}

	ss := &StreamingSession{
		serverId:     addr,
		conn:         conn,
		stream:       stream,
		sendCh:       make(chan *session.SessionStreamRequest, syncStreamSendBufSize),
		ctx:          ctx,
		cancel:       cancel,
		streamCancel: streamCancel,
		pending:      make(map[string]chan error),
	}

	go ss.sendLoop()
	go ss.recvLoop()

	logx.Infof("StreamingSession: connected stream to session node %s", addr)
	return ss, nil
}

func (ss *StreamingSession) sendLoop() {
	for {
		select {
		case <-ss.ctx.Done():
			return
		case req := <-ss.sendCh:
			if err := ss.stream.Send(req); err != nil {
				ss.failStream(err)
				return
			}
		}
	}
}

func (ss *StreamingSession) recvLoop() {
	for {
		response, err := ss.stream.Recv()
		if err != nil {
			ss.failStream(err)
			return
		}
		if remote := response.GetError(); remote != nil {
			err = grpcStatus.Error(codes.Code(remote.GetCode()), remote.GetMessage())
		} else if !response.GetAck().GetSuccess() {
			err = fmt.Errorf("streaming session(%s) rejected push", ss.serverId)
		}
		ss.pendingMu.Lock()
		result := ss.pending[response.GetRequestId()]
		delete(ss.pending, response.GetRequestId())
		ss.pendingMu.Unlock()
		if result != nil {
			result <- err
		}
	}
}

func (ss *StreamingSession) failStream(err error) {
	ss.unavailable.Store(true)
	ss.streamCancel()
	ss.pendingMu.Lock()
	for key, result := range ss.pending {
		result <- err
		delete(ss.pending, key)
	}
	ss.pendingMu.Unlock()
}

func (ss *StreamingSession) push(ctx context.Context, req *session.SessionStreamRequest) error {
	if ss.closed.Load() != 0 {
		return fmt.Errorf("streaming session(%s) closed", ss.serverId)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(ss.ctx, cancel)
	defer stop()
	if ss.unavailable.Load() {
		client := session.NewRPCSessionClient(ss.conn)
		var reply *mtproto.Bool
		var err error
		switch in := req.GetPayload().(type) {
		case *session.SessionStreamRequest_PushUpdates:
			reply, err = client.SessionPushUpdatesData(ctx, in.PushUpdates)
		case *session.SessionStreamRequest_PushSessionUpdates:
			reply, err = client.SessionPushSessionUpdatesData(ctx, in.PushSessionUpdates)
		case *session.SessionStreamRequest_PushRpcResult:
			reply, err = client.SessionPushRpcResultData(ctx, in.PushRpcResult)
		}
		if err != nil {
			return err
		}
		if !mtproto.FromBool(reply) {
			return fmt.Errorf("session(%s) rejected push", ss.serverId)
		}
		return nil
	}
	result := make(chan error, 1)
	ss.pendingMu.Lock()
	if ss.unavailable.Load() {
		ss.pendingMu.Unlock()
		return fmt.Errorf("streaming session(%s) unavailable", ss.serverId)
	}
	ss.pending[req.GetRequestId()] = result
	ss.pendingMu.Unlock()
	defer func() {
		ss.pendingMu.Lock()
		delete(ss.pending, req.GetRequestId())
		ss.pendingMu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ss.sendCh <- req:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-result:
		return err
	}
}

func (ss *StreamingSession) nextRequestId() string {
	return fmt.Sprintf("%d", ss.reqCounter.Add(1))
}

func (ss *StreamingSession) PushUpdates(ctx context.Context, msg *session.TLSessionPushUpdatesData) error {

	req := &session.SessionStreamRequest{
		RequestId: ss.nextRequestId(),
		Payload:   &session.SessionStreamRequest_PushUpdates{PushUpdates: msg},
	}

	return ss.push(ctx, req)
}

func (ss *StreamingSession) PushSessionUpdates(ctx context.Context, msg *session.TLSessionPushSessionUpdatesData) error {

	req := &session.SessionStreamRequest{
		RequestId: ss.nextRequestId(),
		Payload:   &session.SessionStreamRequest_PushSessionUpdates{PushSessionUpdates: msg},
	}

	return ss.push(ctx, req)
}

func (ss *StreamingSession) PushRpcResult(ctx context.Context, msg *session.TLSessionPushRpcResultData) error {

	req := &session.SessionStreamRequest{
		RequestId: ss.nextRequestId(),
		Payload:   &session.SessionStreamRequest_PushRpcResult{PushRpcResult: msg},
	}

	return ss.push(ctx, req)
}

func (ss *StreamingSession) Close() error {
	if ss.closed.CompareAndSwap(0, 1) {
		ss.cancel()
		ss.failStream(context.Canceled)
		ss.conn.Close()
	}
	return nil
}
