// Copyright 2022 Teamgram Authors
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
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package sess

import (
	"context"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/logx"
)

func (c *session) onSyncData(ctx context.Context, obj mtproto.TLObject) error {
	// for android, obj maybe is nil
	if obj != nil {
		logx.WithContext(ctx).Infof("session]]>> - session: %s, syncData: %s", c, obj)
	} else {
		logx.WithContext(ctx).Infof("session]]>> - session: %s, syncData: nil", c)
	}

	gatewayId := c.getGatewayId()

	if c.isAndroidPush {
		pusMsgId := c.sessList.cb.getNextNotifyId()
		if err := c.sendPushToQueue(ctx, gatewayId, pusMsgId, androidPushTooLong); err != nil {
			return err
		}
	} else {
		pusMsgId := c.sessList.cb.getNextPushId()
		if err := c.sendPushToQueue(ctx, gatewayId, pusMsgId, obj); err != nil {
			return err
		}
	}

	if !c.sessionOnline() {
		return ErrPushNotDelivered
	}
	if !c.canSync && c.sessList.state == mtproto.AuthStateNormal {
		return ErrPushNotDelivered
	}
	return c.sendQueueToGateway(ctx, gatewayId)
}

func (c *session) onSyncRpcResultData(ctx context.Context, reqMsgId int64, data []byte) error {
	// TODO(@benqi):
	logx.WithContext(ctx).Debugf("onSyncRpcResultData]]>> - %s", data)
	c.pendingQueue.Remove(reqMsgId)
	gatewayId := c.getGatewayId()
	c.sendPushRpcResultToQueue(gatewayId, reqMsgId, data)
	if message := c.outQueue.Lookup(reqMsgId); message != nil && message.sent > 0 {
		return nil
	}
	return c.sendQueueToGateway(ctx, gatewayId)
}

func (c *session) onSyncSessionData(ctx context.Context, obj mtproto.TLObject) error {
	// TODO(@benqi):
	gatewayId := c.getGatewayId()
	pusMsgId := c.sessList.cb.getNextPushId()

	if err := c.sendPushToQueue(ctx, gatewayId, pusMsgId, obj); err != nil {
		return err
	}
	if !c.canSync && c.sessList.state == mtproto.AuthStateNormal {
		return ErrPushNotDelivered
	}
	return c.sendQueueToGateway(ctx, gatewayId)
}
