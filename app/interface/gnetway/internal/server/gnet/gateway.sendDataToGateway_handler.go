// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gnet

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/gateway"

	"github.com/zeromicro/go-zero/core/logx"
)

// GatewaySendDataToGateway
// gateway.sendDataToGateway auth_key_id:long session_id:long payload:bytes = Bool;
func (s *Server) GatewaySendDataToGateway(ctx context.Context, in *gateway.TLGatewaySendDataToGateway) (reply *mtproto.Bool, err error) {
	logx.WithContext(ctx).Debugf("ReceiveData - request: {kId: %d, sessionId: %d, payloadLen: %d}", in.AuthKeyId, in.SessionId, len(in.Payload))

	authKey, connIdList := s.authSessionMgr.FoundSessionConnId(in.AuthKeyId, in.SessionId)
	if len(connIdList) == 0 {
		logx.WithContext(ctx).Errorf("ReceiveData - not found connId - keyId: %d, sessionId: %d", in.AuthKeyId, in.SessionId)
		return mtproto.BoolFalse, nil
	} else {
		logx.WithContext(ctx).Debugf("found: {k: %v, idList: %v}", in.AuthKeyId, connIdList)
	}

	msgKey, mtpRawData, _ := authKey.AesIgeEncrypt(in.Payload)
	payload := func() []byte {
		x := mtproto.GetEncodeBuf()
		defer mtproto.PutEncodeBuf(x)
		x.Long(authKey.AuthKeyId())
		x.Bytes(msgKey)
		x.Bytes(mtpRawData)
		return append([]byte(nil), x.GetBuf()...)
	}()
	msg := &mtproto.MTPRawMessage{Payload: payload}

	accepted := true
	for _, connId := range connIdList {
		if !s.enqueueGatewayWrite(connId, msg) {
			accepted = false
			metricGatewayWriteRejected.Inc("queue_or_connection")
			logx.WithContext(ctx).Errorf("gateway write queue full or connection closed: connId=%d", connId)
		}
	}
	if !accepted {
		return mtproto.BoolFalse, nil
	}

	return mtproto.BoolTrue, nil
}
