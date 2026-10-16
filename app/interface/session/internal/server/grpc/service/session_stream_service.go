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
	"strings"
	"sync"

	"github.com/teamgram/teamgram-server/app/interface/session/internal/core"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/pkg/keyedexecutor"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
)

// A single session stream serves all auth keys assigned to one session node.
// Keep the stream bounded while preserving the order of updates for one
// permanent auth key. Unrelated users continue concurrently. The 128 limit
// keeps a 2,500-user node from overloading the gateway event loops while the
// keyed executor still prevents same-auth reordering.
const sessionStreamHandlerConcurrency = 128

func sessionStreamRequestKey(req *session.SessionStreamRequest) string {
	permAuthKeyID := func(value int64) string {
		if value == 0 {
			return ""
		}
		return "perm:" + strconv.FormatInt(value, 10)
	}
	authKeyID := func(value int64) string {
		if value == 0 {
			return ""
		}
		return "auth:" + strconv.FormatInt(value, 10)
	}

	var key string
	switch payload := req.GetPayload().(type) {
	case *session.SessionStreamRequest_SendData:
		if payload.SendData != nil {
			key = permAuthKeyID(payload.SendData.GetData().GetPermAuthKeyId())
		}
	case *session.SessionStreamRequest_CreateSession:
		if payload.CreateSession != nil {
			key = permAuthKeyID(payload.CreateSession.GetClient().GetPermAuthKeyId())
		}
	case *session.SessionStreamRequest_CloseSession:
		if payload.CloseSession != nil {
			key = permAuthKeyID(payload.CloseSession.GetClient().GetPermAuthKeyId())
		}
	case *session.SessionStreamRequest_QueryAuthKey:
		if payload.QueryAuthKey != nil {
			key = authKeyID(payload.QueryAuthKey.GetAuthKeyId())
		}
	case *session.SessionStreamRequest_SetAuthKey:
		if payload.SetAuthKey != nil {
			if authKey := payload.SetAuthKey.GetAuthKey(); authKey != nil {
				if authKey.GetPermAuthKeyId() != 0 {
					key = permAuthKeyID(authKey.GetPermAuthKeyId())
				} else {
					key = authKeyID(authKey.GetAuthKeyId())
				}
			}
		}
	case *session.SessionStreamRequest_PushUpdates:
		if payload.PushUpdates != nil {
			key = permAuthKeyID(payload.PushUpdates.GetPermAuthKeyId())
		}
	case *session.SessionStreamRequest_PushSessionUpdates:
		if payload.PushSessionUpdates != nil {
			key = permAuthKeyID(payload.PushSessionUpdates.GetPermAuthKeyId())
		}
	case *session.SessionStreamRequest_PushRpcResult:
		if payload.PushRpcResult != nil {
			key = permAuthKeyID(payload.PushRpcResult.GetPermAuthKeyId())
		}
	}
	if key != "" {
		return key
	}
	if requestID := req.GetRequestId(); requestID != "" {
		return "request:" + requestID
	}
	return fmt.Sprintf("request:%p", req)
}

func (s *Service) SessionDataStream(stream grpc.BidiStreamingServer[session.SessionStreamRequest, session.SessionStreamResponse]) error {
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
	executor := keyedexecutor.New(stream.Context(), sessionStreamHandlerConcurrency,
		func(ctx context.Context, req *session.SessionStreamRequest) {
			resp := s.handleStreamRequest(ctx, req)
			sendMu.Lock()
			err := stream.Send(resp)
			sendMu.Unlock()
			recordSendError(err)
		})
	defer executor.Wait()

	for {
		if err := firstSendError(); err != nil {
			logx.Errorf("SessionDataStream Send error: %v", err)
			return err
		}
		req, err := stream.Recv()
		if err != nil {
			executor.Wait()
			if sendErr := firstSendError(); sendErr != nil {
				logx.Errorf("SessionDataStream Send error: %v", sendErr)
				return sendErr
			}
			logx.Errorf("SessionDataStream Recv error: %v", err)
			return err
		}
		if !executor.Submit(sessionStreamRequestKey(req), req) {
			executor.Wait()
			return stream.Context().Err()
		}
	}
}

func (s *Service) handleStreamRequest(ctx context.Context, req *session.SessionStreamRequest) *session.SessionStreamResponse {
	reqId := req.GetRequestId()

	switch p := req.GetPayload().(type) {
	case *session.SessionStreamRequest_SendData:
		return s.handleSendData(ctx, reqId, p.SendData)
	case *session.SessionStreamRequest_CreateSession:
		return s.handleCreateSession(ctx, reqId, p.CreateSession)
	case *session.SessionStreamRequest_CloseSession:
		return s.handleCloseSession(ctx, reqId, p.CloseSession)
	case *session.SessionStreamRequest_QueryAuthKey:
		return s.handleQueryAuthKey(ctx, reqId, p.QueryAuthKey)
	case *session.SessionStreamRequest_SetAuthKey:
		return s.handleSetAuthKey(ctx, reqId, p.SetAuthKey)
	case *session.SessionStreamRequest_PushUpdates:
		return s.handlePushUpdates(ctx, reqId, p.PushUpdates)
	case *session.SessionStreamRequest_PushSessionUpdates:
		return s.handlePushSessionUpdates(ctx, reqId, p.PushSessionUpdates)
	case *session.SessionStreamRequest_PushRpcResult:
		return s.handlePushRpcResult(ctx, reqId, p.PushRpcResult)
	default:
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    400,
					Message: "unknown payload type",
				},
			},
		}
	}
}

func (s *Service) makeShardingErrorResp(reqId string, err error) *session.SessionStreamResponse {
	// check if it's a redirect error (code 700)
	if target, ok := parseRedirectTarget(err); ok {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    700,
					Message: "REDIRECT_TO_" + target,
				},
			},
		}
	}
	return &session.SessionStreamResponse{
		RequestId: reqId,
		Payload: &session.SessionStreamResponse_Error{
			Error: &session.SessionStreamError{
				Code:    500,
				Message: err.Error(),
			},
		},
	}
}

func (s *Service) makeAckResp(reqId string) *session.SessionStreamResponse {
	return &session.SessionStreamResponse{
		RequestId: reqId,
		Payload: &session.SessionStreamResponse_Ack{
			Ack: &session.SessionStreamAck{Success: true},
		},
	}
}

func (s *Service) handleSendData(ctx context.Context, reqId string, in *session.TLSessionSendDataToSession) *session.SessionStreamResponse {
	if err := s.checkShardingV(ctx, in.GetData().GetPermAuthKeyId()); err != nil {
		return s.makeShardingErrorResp(reqId, err)
	}

	c := core.New(ctx, s.svcCtx)
	_, err := c.SessionSendDataToSession(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return s.makeAckResp(reqId)
}

func (s *Service) handleCreateSession(ctx context.Context, reqId string, in *session.TLSessionCreateSession) *session.SessionStreamResponse {
	if err := s.checkShardingV(ctx, in.GetClient().GetPermAuthKeyId()); err != nil {
		return s.makeShardingErrorResp(reqId, err)
	}

	c := core.New(ctx, s.svcCtx)
	_, err := c.SessionCreateSession(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return s.makeAckResp(reqId)
}

func (s *Service) handleCloseSession(ctx context.Context, reqId string, in *session.TLSessionCloseSession) *session.SessionStreamResponse {
	if err := s.checkShardingV(ctx, in.GetClient().GetPermAuthKeyId()); err != nil {
		return s.makeShardingErrorResp(reqId, err)
	}

	c := core.New(ctx, s.svcCtx)
	_, err := c.SessionCloseSession(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return s.makeAckResp(reqId)
}

func (s *Service) handleQueryAuthKey(ctx context.Context, reqId string, in *session.TLSessionQueryAuthKey) *session.SessionStreamResponse {
	c := core.New(ctx, s.svcCtx)
	authKey, err := c.SessionQueryAuthKey(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return &session.SessionStreamResponse{
		RequestId: reqId,
		Payload: &session.SessionStreamResponse_AuthKey{
			AuthKey: authKey,
		},
	}
}

func (s *Service) handleSetAuthKey(ctx context.Context, reqId string, in *session.TLSessionSetAuthKey) *session.SessionStreamResponse {
	c := core.New(ctx, s.svcCtx)
	_, err := c.SessionSetAuthKey(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return s.makeAckResp(reqId)
}

func (s *Service) handlePushUpdates(ctx context.Context, reqId string, in *session.TLSessionPushUpdatesData) *session.SessionStreamResponse {
	if err := s.checkShardingV(ctx, in.GetPermAuthKeyId()); err != nil {
		return s.makeShardingErrorResp(reqId, err)
	}

	c := core.New(ctx, s.svcCtx)
	_, err := c.SessionPushUpdatesData(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return s.makeAckResp(reqId)
}

func (s *Service) handlePushSessionUpdates(ctx context.Context, reqId string, in *session.TLSessionPushSessionUpdatesData) *session.SessionStreamResponse {
	if err := s.checkShardingV(ctx, in.GetPermAuthKeyId()); err != nil {
		return s.makeShardingErrorResp(reqId, err)
	}

	c := core.New(ctx, s.svcCtx)
	_, err := c.SessionPushSessionUpdatesData(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return s.makeAckResp(reqId)
}

func (s *Service) handlePushRpcResult(ctx context.Context, reqId string, in *session.TLSessionPushRpcResultData) *session.SessionStreamResponse {
	if err := s.checkShardingV(ctx, in.GetPermAuthKeyId()); err != nil {
		return s.makeShardingErrorResp(reqId, err)
	}

	c := core.New(ctx, s.svcCtx)
	_, err := c.SessionPushRpcResultData(in)
	if err != nil {
		return &session.SessionStreamResponse{
			RequestId: reqId,
			Payload: &session.SessionStreamResponse_Error{
				Error: &session.SessionStreamError{
					Code:    500,
					Message: err.Error(),
				},
			},
		}
	}
	return s.makeAckResp(reqId)
}

// parseRedirectTarget extracts the target address from a redirect error.
// The error format follows mtproto.NewErrRedirectToX: gRPC code 700, message "REDIRECT_TO_<addr>"
func parseRedirectTarget(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	s := err.Error()
	const prefix = "REDIRECT_TO_"
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return "", false
	}
	target := s[idx+len(prefix):]
	if target == "" {
		return "", false
	}
	return target, true
}
