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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// MessagesSendScreenshotNotification
// messages.sendScreenshotNotification#c97df020 peer:InputPeer reply_to_msg_id:int random_id:long = Updates;
func (c *DialogsCore) MessagesSendScreenshotNotification(in *mtproto.TLMessagesSendScreenshotNotification) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if in == nil || in.GetPeer() == nil {
		c.Logger.Errorf("messages.sendScreenshotNotification - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer == nil || peer.PeerId == 0 {
		c.Logger.Errorf("messages.sendScreenshotNotification - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		c.Logger.Errorf("messages.sendScreenshotNotification - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType == mtproto.PEER_CHANNEL {
		return nil, mtproto.ErrMethodNotImpl
	}

	replyToMsgID := in.GetReplyToMsgId()
	if replyToMsgID == 0 && in.GetReplyTo() != nil {
		replyToMsgID = in.GetReplyTo().GetReplyToMsgId()
	}
	var replyTo *mtproto.MessageReplyHeader
	if replyToMsgID != 0 {
		replyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
			ReplyToMsgId:           replyToMsgID,
			ReplyToMsgId_INT32:     replyToMsgID,
			ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(replyToMsgID),
		}).To_MessageReplyHeader()
	}

	date := int32(time.Now().Unix())
	action := mtproto.MakeMessageActionScreenshotTaken()
	serviceMessage := func(out bool) *mtproto.Message {
		return mtproto.MakeTLMessageService(&mtproto.Message{
			Out:     out,
			FromId:  mtproto.MakePeerUser(c.MD.UserId),
			PeerId:  peer.ToPeer(),
			ReplyTo: replyTo,
			Date:    date,
			Action:  action,
		}).To_Message()
	}
	updatesWith := func(out bool) *mtproto.Updates {
		return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: serviceMessage(out),
			PtsCount:        1,
			RandomId:        in.GetRandomId(),
		}).To_Update())
	}

	rUpdates := updatesWith(true)
	if _, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       rUpdates,
	}); err != nil {
		return nil, err
	}

	switch peer.PeerType {
	case mtproto.PEER_USER:
		if peer.PeerId != c.MD.UserId {
			if _, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
				UserId:  peer.PeerId,
				Updates: updatesWith(false),
			}); err != nil {
				return nil, err
			}
		}
	case mtproto.PEER_CHAT:
		if c.svcCtx.Dao.ChatClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err != nil {
			return nil, err
		}
		if chat == nil || chat.GetChat() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		incoming := updatesWith(false)
		var pushErr error
		chat.Walk(func(userID int64, participant *mtproto.ImmutableChatParticipant) error {
			if pushErr != nil || userID == c.MD.UserId || participant == nil || !participant.IsChatMemberStateNormal() {
				return nil
			}
			_, pushErr = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
				UserId:  userID,
				Updates: incoming,
			})
			return nil
		})
		if pushErr != nil {
			return nil, pushErr
		}
	}

	return rUpdates, nil
}
