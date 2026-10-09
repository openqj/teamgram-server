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

package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// MessagesReceivedMessages
// messages.receivedMessages#5a954c0 max_id:int = Vector<ReceivedNotifyMessage>;
func (c *MessagesCore) MessagesReceivedMessages(in *mtproto.TLMessagesReceivedMessages) (*mtproto.Vector_ReceivedNotifyMessage, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetMaxId() < 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ReceivedMessages == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.svcCtx.Dao.ReceivedMessages.Record(ctx, c.MD.UserId, in.GetMaxId()); err != nil {
		return nil, err
	}
	// The notification producer is optional. A durable cursor acknowledgement
	// still has a valid empty result when there are no pending notifications.
	return &mtproto.Vector_ReceivedNotifyMessage{Datas: []*mtproto.ReceivedNotifyMessage{}}, nil
}
