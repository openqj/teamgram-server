// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package dao

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teamgram/teamgram-server/app/interface/gnetway/gateway"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	gatewaySendBufSize             = 8192
	gatewayInitialReconnectBackoff = 100 * time.Millisecond
	gatewayMaxReconnectBackoff     = 10 * time.Second
)

type gatewayNodeStream struct {
	nodeAddr   string
	conn       *grpc.ClientConn
	stream     grpc.BidiStreamingClient[gateway.GatewayStreamRequest, gateway.GatewayStreamResponse]
	sendCh     chan *gateway.GatewayStreamRequest
	closed     atomic.Int32
	ctx        context.Context
	cancel     context.CancelFunc
	reqCounter atomic.Int64
	pendingMu  sync.Mutex
	pending    map[string]chan error
}

func (ns *gatewayNodeStream) close(err error) {
	if ns.closed.CompareAndSwap(0, 1) {
		ns.cancel()
		ns.pendingMu.Lock()
		for requestID, result := range ns.pending {
			result <- err
			delete(ns.pending, requestID)
		}
		ns.pendingMu.Unlock()
		ns.conn.Close()
	}
}

// StreamingGateway manages bidirectional streaming connections to gnetway nodes.
type StreamingGateway struct {
	mu      sync.RWMutex
	streams map[string]*gatewayNodeStream // gatewayId → stream
}

func NewStreamingGateway() *StreamingGateway {
	return &StreamingGateway{
		streams: make(map[string]*gatewayNodeStream),
	}
}

func (sg *StreamingGateway) getOrCreateStream(gatewayId string) (*gatewayNodeStream, error) {
	sg.mu.RLock()
	ns, ok := sg.streams[gatewayId]
	sg.mu.RUnlock()

	if ok && ns.closed.Load() == 0 {
		return ns, nil
	}

	sg.mu.Lock()
	defer sg.mu.Unlock()

	// double-check
	if ns, ok = sg.streams[gatewayId]; ok && ns.closed.Load() == 0 {
		return ns, nil
	}

	ns, err := sg.createGatewayStream(gatewayId)
	if err != nil {
		return nil, err
	}
	sg.streams[gatewayId] = ns
	return ns, nil
}

func (sg *StreamingGateway) createGatewayStream(addr string) (*gatewayNodeStream, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithReadBufferSize(16*1024*1024),
		grpc.WithWriteBufferSize(16*1024*1024),
	)
	if err != nil {
		return nil, fmt.Errorf("dial gateway %s: %w", addr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := gateway.NewRPCGatewayStreamClient(conn)
	stream, err := client.GatewayDataStream(ctx)
	if err != nil {
		cancel()
		conn.Close()
		return nil, fmt.Errorf("open stream to gateway %s: %w", addr, err)
	}

	ns := &gatewayNodeStream{
		nodeAddr: addr,
		conn:     conn,
		stream:   stream,
		sendCh:   make(chan *gateway.GatewayStreamRequest, gatewaySendBufSize),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan error),
	}

	go sg.sendLoop(ns)
	go sg.recvLoop(ns)

	logx.Infof("StreamingGateway: connected stream to gateway node %s", addr)
	return ns, nil
}

func (sg *StreamingGateway) sendLoop(ns *gatewayNodeStream) {
	for {
		select {
		case <-ns.ctx.Done():
			return
		case req := <-ns.sendCh:
			if err := ns.stream.Send(req); err != nil {
				logx.Errorf("StreamingGateway sendLoop(%s) Send error: %v", ns.nodeAddr, err)
				sg.handleStreamError(ns, err)
				return
			}
		}
	}
}

func (sg *StreamingGateway) recvLoop(ns *gatewayNodeStream) {
	for {
		response, err := ns.stream.Recv()
		if err != nil {
			if ns.closed.Load() == 0 {
				logx.Errorf("StreamingGateway recvLoop(%s) Recv error: %v", ns.nodeAddr, err)
				sg.handleStreamError(ns, err)
			}
			return
		}
		if response == nil {
			sg.handleStreamError(ns, fmt.Errorf("gateway(%s) returned an empty stream reply", ns.nodeAddr))
			return
		}
		if !response.GetSuccess() {
			err = fmt.Errorf("gateway(%s) rejected session delivery", ns.nodeAddr)
		}
		ns.pendingMu.Lock()
		result := ns.pending[response.GetRequestId()]
		delete(ns.pending, response.GetRequestId())
		ns.pendingMu.Unlock()
		if result != nil {
			result <- err
		}
	}
}

func (sg *StreamingGateway) handleStreamError(ns *gatewayNodeStream, err error) {
	addr := ns.nodeAddr
	ns.close(err)

	sg.mu.Lock()
	if sg.streams[addr] == ns {
		delete(sg.streams, addr)
	}
	sg.mu.Unlock()

	// reconnect will happen lazily on next SendDataToGateway call
	logx.Infof("StreamingGateway: removed stream for %s, will reconnect on next send", addr)
}

func (sg *StreamingGateway) SendDataToGateway(ctx context.Context, gatewayId string, authKeyId, sessionId int64, payload []byte) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	ns, err := sg.getOrCreateStream(gatewayId)
	if err != nil {
		return false, err
	}

	req := &gateway.GatewayStreamRequest{
		RequestId: strconv.FormatInt(ns.reqCounter.Add(1), 10),
		SendData: &gateway.TLGatewaySendDataToGateway{
			AuthKeyId: authKeyId,
			SessionId: sessionId,
			Payload:   payload,
		},
	}
	result := make(chan error, 1)
	ns.pendingMu.Lock()
	if ns.closed.Load() != 0 {
		ns.pendingMu.Unlock()
		return false, fmt.Errorf("gateway(%s) stream is closed", gatewayId)
	}
	ns.pending[req.GetRequestId()] = result
	ns.pendingMu.Unlock()
	defer func() {
		ns.pendingMu.Lock()
		delete(ns.pending, req.GetRequestId())
		ns.pendingMu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-ns.ctx.Done():
		return false, fmt.Errorf("gateway(%s) stream is closed", gatewayId)
	case ns.sendCh <- req:
	default:
		return false, fmt.Errorf("StreamingGateway: sendCh full for gateway %s", gatewayId)
	}
	select {
	case err := <-result:
		return err == nil, err
	case <-ctx.Done():
		return false, ctx.Err()
	case <-ns.ctx.Done():
		return false, fmt.Errorf("gateway(%s) stream is closed", gatewayId)
	}
}

func (sg *StreamingGateway) RemoveGateway(gatewayId string) {
	sg.mu.Lock()
	if ns, ok := sg.streams[gatewayId]; ok {
		ns.close(fmt.Errorf("gateway(%s) removed", gatewayId))
		delete(sg.streams, gatewayId)
	}
	sg.mu.Unlock()
}

func (sg *StreamingGateway) Close() {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	for _, ns := range sg.streams {
		ns.close(fmt.Errorf("gateway(%s) stream manager closed", ns.nodeAddr))
	}
	sg.streams = make(map[string]*gatewayNodeStream)
}
