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
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
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
	provider, ok := c.svcCtx.Dao.ReceivedMessages.(dao.ReceivedMessagesProvider)
	if !ok {
		// A monotonic cursor is not enough to acknowledge a PUSH notification.
		return nil, mtproto.ErrMethodNotImpl
	}
	rows, err := provider.Acknowledge(c.ctx, c.MD.UserId, in.GetMaxId())
	if err != nil {
		if errors.Is(err, mtproto.ErrMessageIdInvalid) || errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			return nil, err
		}
		return nil, mtproto.ErrInternalServerError
	}
	result := &mtproto.Vector_ReceivedNotifyMessage{Datas: make([]*mtproto.ReceivedNotifyMessage, 0, len(rows))}
	for _, row := range rows {
		if row.ID <= 0 {
			return nil, mtproto.ErrInternalServerError
		}
		result.Datas = append(result.Datas, mtproto.MakeTLReceivedNotifyMessage(&mtproto.ReceivedNotifyMessage{
			Id:    row.ID,
			Flags: row.Flags,
		}).To_ReceivedNotifyMessage())
	}
	return result, nil
}
