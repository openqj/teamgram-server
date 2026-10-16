// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/gateway"
	"github.com/teamgram/teamgram-server/pkg/keyedexecutor"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/metric"
	"google.golang.org/grpc"
)

// Gateway streams are shared by all sessions on one gateway node. Bounded
// parallel handling keeps the stream responsive while preserving ordering for
// one auth key/session connection.
const gatewayStreamHandlerConcurrency = 256

var gatewayStreamHandlerDuration = metric.NewHistogramVec(&metric.HistogramVecOpts{
	Namespace: "gnetway",
	Subsystem: "gateway_stream",
	Name:      "handler_duration_ms",
	Help:      "Time from receiving a gateway stream request through the response ACK.",
	Labels:    []string{"outcome"},
	Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 10000},
})

func gatewayStreamRequestKey(req *gateway.GatewayStreamRequest) string {
	if data := req.GetSendData(); data != nil && data.GetAuthKeyId() != 0 && data.GetSessionId() != 0 {
		return "connection:" + strconv.FormatInt(data.GetAuthKeyId(), 10) + ":" + strconv.FormatInt(data.GetSessionId(), 10)
	}
	if requestID := req.GetRequestId(); requestID != "" {
		return "request:" + requestID
	}
	return fmt.Sprintf("request:%p", req)
}

func (s *Service) GatewayDataStream(stream grpc.BidiStreamingServer[gateway.GatewayStreamRequest, gateway.GatewayStreamResponse]) error {
	var (
		sendMu  sync.Mutex
		sendErr error
		errMu   sync.Mutex
	)
	recordSendError := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if sendErr == nil {
			sendErr = err
		}
		errMu.Unlock()
	}
	firstSendError := func() error {
		errMu.Lock()
		defer errMu.Unlock()
		return sendErr
	}
	executor := keyedexecutor.New(stream.Context(), gatewayStreamHandlerConcurrency,
		func(ctx context.Context, req *gateway.GatewayStreamRequest) {
			started := time.Now()
			outcome := "invalid"
			sendData := req.GetSendData()
			resp := &gateway.GatewayStreamResponse{RequestId: req.GetRequestId()}
			if sendData == nil {
				resp.Success = false
			} else {
				// Delegate to the existing unary handler so auth/session routing
				// and queue admission have one implementation.
				reply, rpcErr := s.RPCGatewayServer.GatewaySendDataToGateway(ctx, sendData)
				resp.Success = rpcErr == nil && reply != nil && mtproto.FromBool(reply)
				switch {
				case rpcErr != nil:
					outcome = "error"
				case resp.Success:
					outcome = "ok"
				default:
					outcome = "rejected"
				}
			}
			sendMu.Lock()
			err := stream.Send(resp)
			sendMu.Unlock()
			if err != nil {
				outcome = "send_error"
			}
			gatewayStreamHandlerDuration.ObserveFloat(time.Since(started).Seconds()*1000, outcome)
			recordSendError(err)
		})
	defer executor.Wait()

	for {
		if err := firstSendError(); err != nil {
			logx.Errorf("GatewayDataStream Send error: %v", err)
			return err
		}
		req, err := stream.Recv()
		if err != nil {
			executor.Wait()
			if sendErr := firstSendError(); sendErr != nil {
				logx.Errorf("GatewayDataStream Send error: %v", sendErr)
				return sendErr
			}
			logx.Errorf("GatewayDataStream Recv error: %v", err)
			return err
		}
		if !executor.Submit(gatewayStreamRequestKey(req), req) {
			executor.Wait()
			return stream.Context().Err()
		}
	}
}
