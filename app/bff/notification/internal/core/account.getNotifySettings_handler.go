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

// AccountGetNotifySettings
// account.getNotifySettings#12b3ad31 peer:InputNotifyPeer = PeerNotifySettings;
func (c *NotificationCore) AccountGetNotifySettings(in *mtproto.TLAccountGetNotifySettings) (*mtproto.PeerNotifySettings, error) {
	if in == nil || in.GetPeer() == nil {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("account.getNotifySettings - error: %v", err)
		return nil, err
	}

	peer := mtproto.FromInputNotifyPeer(c.MD.UserId, in.GetPeer())
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER:
		if peer.PeerId <= 0 {
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{c.MD.UserId, peer.PeerId},
		})
		if err != nil {
			c.Logger.Errorf("account.getNotifySettings - load user error: %v", err)
			return nil, err
		}
		user, ok := users.GetImmutableUser(peer.PeerId)
		if !ok || user == nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
		if user.Deleted() {
			err = mtproto.ErrInputUserDeactivated
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
	case mtproto.PEER_CHAT:
		if peer.PeerId <= 0 {
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
		group, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("account.getNotifySettings - load chat error: %v", err)
			return nil, mtproto.ErrPeerIdInvalid
		}
		if group == nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
		member, ok := group.GetImmutableChatParticipant(c.MD.UserId)
		if !ok || member == nil || !member.IsChatMemberStateNormal() {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
	case mtproto.PEER_CHANNEL:
		if peer.PeerId <= 0 || c.svcCtx.Plugin == nil {
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
		channel, err := c.svcCtx.Plugin.GetChannelById(c.ctx, c.MD.UserId, peer.PeerId)
		if err != nil {
			c.Logger.Errorf("account.getNotifySettings - load channel error: %v", err)
			return nil, err
		}
		if channel == nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
		if channel.GetPredicateName() == mtproto.Predicate_channelForbidden {
			err = mtproto.ErrChannelPrivate
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
		if channel.GetPredicateName() != mtproto.Predicate_channel {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.getNotifySettings - error: %v", err)
			return nil, err
		}
	case mtproto.PEER_USERS:
	case mtproto.PEER_CHATS:
	case mtproto.PEER_BROADCASTS:
	default:
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("account.getNotifySettings - error: %v", err)
		return nil, err
	}

	settings, err := c.svcCtx.Dao.UserClient.UserGetNotifySettings(c.ctx, &userpb.TLUserGetNotifySettings{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})
	if err != nil {
		c.Logger.Errorf("account.getNotifySettings - error: %v", err)
		return nil, err
	}

	return settings, nil
}
