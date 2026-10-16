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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
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
	if err := c.validateScreenshotPeer(peer, in.GetPeer()); err != nil {
		return nil, err
	}
	// A screenshot service message is a common PTS event. This BFF has no
	// authoritative service-message persistence or PTS allocator, so emitting
	// updateNewMessage with pts=0 would make the PostgreSQL sync journal reject
	// the request. Keep the authorized boundary explicit until such a producer
	// owns persistence and update state.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *DialogsCore) validateScreenshotPeer(peer *mtproto.PeerUtil, input *mtproto.InputPeer) error {
	if peer == nil || input == nil || peer.PeerId <= 0 {
		return mtproto.ErrPeerIdInvalid
	}

	switch peer.PeerType {
	case mtproto.PEER_SELF:
		return nil
	case mtproto.PEER_USER:
		if input.GetUserId() <= 0 || input.GetAccessHash() == 0 || c.svcCtx.Dao.UserClient == nil {
			return mtproto.ErrUserIdInvalid
		}
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{peer.PeerId},
			To: []int64{c.MD.UserId},
		})
		if err != nil {
			return err
		}
		if users == nil {
			return mtproto.ErrInternalServerError
		}
		for _, user := range users.GetDatas() {
			if user == nil || user.GetUser() == nil {
				continue
			}
			data := user.GetUser()
			if data.GetId() == peer.PeerId && !data.GetDeleted() && data.GetAccessHash() == input.GetAccessHash() {
				return nil
			}
		}
		return mtproto.ErrUserIdInvalid
	case mtproto.PEER_CHAT:
		if input.GetChatId() <= 0 || c.svcCtx.Dao.ChatClient == nil {
			return mtproto.ErrChatIdInvalid
		}
		chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peer.PeerId})
		if err != nil {
			return err
		}
		if chat == nil || chat.GetChat() == nil || chat.GetChat().GetId() != peer.PeerId {
			return mtproto.ErrChatIdInvalid
		}
		actorIsMember := false
		for _, participant := range chat.GetChatParticipants() {
			if participant == nil || participant.GetUserId() <= 0 {
				return mtproto.ErrInternalServerError
			}
			if !participant.IsChatMemberStateNormal() {
				continue
			}
			userID := participant.GetUserId()
			if userID == c.MD.UserId {
				actorIsMember = true
				continue
			}
		}
		if !actorIsMember {
			return mtproto.ErrUserNotParticipant
		}
		return nil
	case mtproto.PEER_CHANNEL:
		if input.GetChannelId() <= 0 || input.GetAccessHash() == 0 {
			return mtproto.ErrChannelInvalid
		}
		return mtproto.ErrMethodNotImpl
	default:
		return mtproto.ErrPeerIdInvalid
	}
}
