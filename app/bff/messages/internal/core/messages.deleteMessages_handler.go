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
	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesDeleteMessages
// messages.deleteMessages#e58e95d2 flags:# revoke:flags.0?true id:Vector<int> = messages.AffectedMessages;
func (c *MessagesCore) MessagesDeleteMessages(in *mtproto.TLMessagesDeleteMessages) (*mtproto.Messages_AffectedMessages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || len(in.GetId()) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	for _, id := range in.GetId() {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
	}
	actingUserID := c.MD.UserId
	if scope, ok := c.businessScope(); ok {
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		actingUserID = scope.OwnerID
		boxes, lookupErr := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
			UserId: actingUserID,
			IdList: in.GetId(),
		})
		if lookupErr != nil {
			return nil, lookupErr
		}
		if boxes == nil || len(boxes.GetDatas()) != len(in.GetId()) {
			return nil, mtproto.ErrMessageIdInvalid
		}
		for _, box := range boxes.GetDatas() {
			if box == nil {
				return nil, mtproto.ErrMessageIdInvalid
			}
			sentByBot := box.SenderUserId == actingUserID || box.SenderUserId == c.MD.UserId
			if sentByBot {
				if err := c.requireBusinessRight(scope.CanDeleteSent); err != nil {
					return nil, err
				}
			} else if err := c.requireBusinessRight(scope.CanDeleteAll); err != nil {
				return nil, err
			}
		}
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	affectedMessages, err := c.svcCtx.Dao.MsgClient.MsgDeleteMessages(c.ctx, &msgpb.TLMsgDeleteMessages{
		UserId:    actingUserID,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_EMPTY,
		PeerId:    0,
		Revoke:    in.Revoke,
		Id:        in.Id,
	})

	if err != nil {
		c.Logger.Errorf("messages.deleteMessages - error: %v", err)
		return nil, err
	}
	if affectedMessages == nil {
		return nil, mtproto.ErrInternalServerError
	}

	return affectedMessages, nil
}
