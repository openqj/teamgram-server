/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// SyncPushRpcResult
// sync.pushRpcResult server_id:long auth_key_id:long req_msg_id:long result:bytes = PushUpdates;
func (c *SyncCore) SyncPushRpcResult(in *sync.TLSyncPushRpcResult) (*mtproto.Void, error) {
	if in == nil || in.GetPermAuthKeyId() == 0 || in.GetServerId() == "" || in.GetSessionId() == 0 || in.GetClientReqMsgId() == 0 || len(in.GetRpcResult()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err := c.svcCtx.Dao.PushRpcResultToSession(
		c.ctx,
		in.ServerId,
		&session.TLSessionPushRpcResultData{
			PermAuthKeyId:  in.PermAuthKeyId,
			AuthKeyId:      in.PermAuthKeyId,
			SessionId:      in.SessionId,
			ClientReqMsgId: in.ClientReqMsgId,
			RpcResultData:  in.RpcResult,
		}); err != nil {
		return nil, err
	}

	return mtproto.EmptyVoid, nil
}
