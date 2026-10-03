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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetPeerSettings
// messages.getPeerSettings#efd9a6a2 peer:InputPeer = messages.PeerSettings;
func (c *DialogsCore) MessagesGetPeerSettings(in *mtproto.TLMessagesGetPeerSettings) (*mtproto.Messages_PeerSettings, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerId <= 0 && peer.PeerType != mtproto.PEER_SELF {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType != mtproto.PEER_SELF && peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_CHAT && peer.PeerType != mtproto.PEER_CHANNEL {
		return nil, mtproto.ErrPeerIdInvalid
	}

	users := make([]*mtproto.User, 0)
	chats := make([]*mtproto.Chat, 0)
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER:
		peerId := peer.PeerId
		if peer.PeerType == mtproto.PEER_SELF {
			peerId = c.MD.UserId
		}
		userIDs := []int64{peerId}
		if c.MD.UserId != peerId {
			userIDs = append(userIDs, c.MD.UserId)
		}
		mutableUsers, err := c.svcCtx.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: userIDs,
			To: []int64{c.MD.UserId},
		})
		if err != nil {
			c.Logger.Errorf("messages.getPeerSettings - load user error: %v", err)
			return nil, err
		}
		if mutableUsers == nil {
			return nil, fmt.Errorf("messages.getPeerSettings: user.getMutableUsers returned no response")
		}
		requested := make(map[int64]struct{}, len(userIDs))
		for _, id := range userIDs {
			requested[id] = struct{}{}
		}
		seen := make(map[int64]struct{}, len(userIDs))
		for _, entity := range mutableUsers.GetDatas() {
			if entity == nil || entity.GetUser() == nil {
				return nil, fmt.Errorf("messages.getPeerSettings: user.getMutableUsers returned a malformed entity")
			}
			id := entity.GetUser().GetId()
			if _, ok := requested[id]; !ok {
				return nil, fmt.Errorf("messages.getPeerSettings: user.getMutableUsers returned unexpected user %d", id)
			}
			if _, ok := seen[id]; ok {
				return nil, fmt.Errorf("messages.getPeerSettings: user.getMutableUsers returned duplicate user %d", id)
			}
			seen[id] = struct{}{}
		}
		for _, id := range userIDs {
			if _, ok := seen[id]; !ok {
				return nil, mtproto.ErrUserIdInvalid
			}
		}
		user, ok := mutableUsers.GetImmutableUser(peerId)
		if !ok || user == nil || user.GetUser() == nil || user.Deleted() {
			return nil, mtproto.ErrUserIdInvalid
		}
		if peer.PeerType == mtproto.PEER_USER && user.AccessHash() != peer.AccessHash {
			return nil, mtproto.ErrUserIdInvalid
		}
		users = mutableUsers.GetUserListByIdList(c.MD.UserId, peerId)
		if len(users) != 1 {
			return nil, fmt.Errorf("messages.getPeerSettings: user.getMutableUsers did not hydrate peer %d", peerId)
		}
	case mtproto.PEER_CHAT:
		if c.svcCtx.ChatClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		chat, err := c.svcCtx.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getPeerSettings - load chat error: %v", err)
			return nil, err
		}
		if chat == nil || chat.GetChat() == nil || chat.Id() != peer.PeerId {
			return nil, mtproto.ErrChatIdInvalid
		}
		for _, item := range chat.GetChatParticipants() {
			if item == nil {
				return nil, fmt.Errorf("messages.getPeerSettings: chat.getMutableChat returned a malformed participant")
			}
		}
		participant, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
		if !ok || !participant.IsChatMemberStateNormal() {
			return nil, mtproto.ErrUserNotParticipant
		}
		chats = append(chats, chat.ToUnsafeChat(c.MD.UserId))
	case mtproto.PEER_CHANNEL:
		if in.Peer.GetPredicateName() != mtproto.Predicate_inputPeerChannel || c.svcCtx.Plugin == nil {
			return nil, fmt.Errorf("messages.getPeerSettings: channel hydration is unavailable")
		}
		resolved := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, peer.PeerId)
		if len(resolved) != 1 || resolved[0] == nil {
			return nil, fmt.Errorf("messages.getPeerSettings: channel %d was not hydrated", peer.PeerId)
		}
		channel := resolved[0]
		if (channel.GetPredicateName() != mtproto.Predicate_channel && channel.GetPredicateName() != mtproto.Predicate_channelForbidden) || channel.GetId() != peer.PeerId {
			return nil, fmt.Errorf("messages.getPeerSettings: channel resolver returned an invalid entity for %d", peer.PeerId)
		}
		accessHash := channel.GetAccessHash_FLAGINT64()
		if accessHash == nil || accessHash.GetValue() != peer.AccessHash {
			return nil, mtproto.ErrChannelInvalid
		}
		chats = append(chats, channel)
	}
	peerSettings, err := c.svcCtx.UserClient.UserGetPeerSettings(c.ctx, &userpb.TLUserGetPeerSettings{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})
	if err != nil {
		c.Logger.Errorf("messages.getPeerSettings - error: %v", err)
		return nil, err
	}
	if peerSettings == nil {
		return nil, fmt.Errorf("messages.getPeerSettings: user.getPeerSettings returned no response")
	}

	return mtproto.MakeTLMessagesPeerSettings(&mtproto.Messages_PeerSettings{
		Settings: peerSettings,
		Chats:    chats,
		Users:    users,
	}).To_Messages_PeerSettings(), nil
}
