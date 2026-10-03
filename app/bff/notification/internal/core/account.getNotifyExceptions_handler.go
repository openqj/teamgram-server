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

func validateNotifyExceptionIDs(kind string, requestedIDs, hydratedIDs []int64) error {
	requested := make(map[int64]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[id] = struct{}{}
	}

	hydrated := make(map[int64]struct{}, len(hydratedIDs))
	for _, id := range hydratedIDs {
		if _, ok := requested[id]; !ok {
			return fmt.Errorf("account.getNotifyExceptions: unexpected %s id %d", kind, id)
		}
		if _, ok := hydrated[id]; ok {
			return fmt.Errorf("account.getNotifyExceptions: duplicate %s id %d", kind, id)
		}
		hydrated[id] = struct{}{}
	}

	for _, id := range requestedIDs {
		if _, ok := hydrated[id]; !ok {
			return fmt.Errorf("account.getNotifyExceptions: missing %s id %d", kind, id)
		}
	}
	return nil
}

// AccountGetNotifyExceptions
// account.getNotifyExceptions#53577479 flags:# compare_sound:flags.1?true peer:flags.0?InputNotifyPeer = Updates;
func (c *NotificationCore) AccountGetNotifyExceptions(in *mtproto.TLAccountGetNotifyExceptions) (*mtproto.Updates, error) {
	// compare_sound

	settings, err := c.svcCtx.Dao.UserClient.UserGetAllNotifySettings(c.ctx, &userpb.TLUserGetAllNotifySettings{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
		return nil, err
	}
	if settings == nil {
		return nil, fmt.Errorf("account.getNotifyExceptions: user.getAllNotifySettings returned no response")
	}

	idHelper := mtproto.NewIDListHelper()

	for _, setting := range settings.GetDatas() {
		if setting == nil {
			return nil, fmt.Errorf("account.getNotifyExceptions: user.getAllNotifySettings returned an invalid peer")
		}
		switch setting.GetPeerType() {
		case mtproto.PEER_USERS, mtproto.PEER_CHATS, mtproto.PEER_BROADCASTS:
			// Global notification settings are not peer exceptions.
			continue
		case mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
			if setting.GetPeerId() <= 0 {
				return nil, fmt.Errorf("account.getNotifyExceptions: user.getAllNotifySettings returned an invalid peer")
			}
		default:
			return nil, fmt.Errorf("account.getNotifyExceptions: user.getAllNotifySettings returned unsupported peer type %d", setting.GetPeerType())
		}
		peer := mtproto.MakePeerUtil(setting.PeerType, setting.PeerId)
		idHelper.PickByPeerUtil(peer.PeerType, peer.PeerId)
	}

	if len(idHelper.ChannelIdList) > 0 && c.svcCtx.Plugin == nil {
		err := fmt.Errorf("account.getNotifyExceptions: channel exception hydration is unavailable")
		c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
		return nil, err
	}

	users := make([]*mtproto.User, 0)
	if len(idHelper.UserIdList) > 0 {
		requestedUsers := mtproto.NewIDListHelper(idHelper.UserIdList...)
		requestedUsers.AppendUsers(c.MD.UserId)
		requestedUserIDs := requestedUsers.UserIdList
		mutableUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
			&userpb.TLUserGetMutableUsers{
				Id: requestedUserIDs,
			})
		if err != nil {
			c.Logger.Errorf("account.getNotifyExceptions - load users error: %v", err)
			return nil, fmt.Errorf("account.getNotifyExceptions: load users: %w", err)
		}
		var hydratedUserIDs []int64
		for _, user := range mutableUsers.GetDatas() {
			if user == nil || user.GetUser() == nil {
				err = fmt.Errorf("account.getNotifyExceptions: load users: malformed user entity")
				c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
				return nil, err
			}
			hydratedUserIDs = append(hydratedUserIDs, user.GetUser().GetId())
		}
		if err = validateNotifyExceptionIDs("user", requestedUserIDs, hydratedUserIDs); err != nil {
			c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
			return nil, err
		}
		users = mutableUsers.GetUserListByIdList(c.MD.UserId, idHelper.UserIdList...)
	}

	var chats []*mtproto.Chat
	if len(idHelper.ChatIdList) > 0 {
		mutableChats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx,
			&chatpb.TLChatGetChatListByIdList{
				IdList: idHelper.ChatIdList,
			})
		if err != nil {
			c.Logger.Errorf("account.getNotifyExceptions - load chats error: %v", err)
			return nil, fmt.Errorf("account.getNotifyExceptions: load chats: %w", err)
		}
		var hydratedChatIDs []int64
		for _, chat := range mutableChats.GetDatas() {
			if chat == nil || chat.GetChat() == nil {
				err = fmt.Errorf("account.getNotifyExceptions: load chats: malformed chat entity")
				c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
				return nil, err
			}
			for _, participant := range chat.GetChatParticipants() {
				if participant == nil {
					err = fmt.Errorf("account.getNotifyExceptions: load chats: malformed chat participant")
					c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
					return nil, err
				}
			}
			hydratedChatIDs = append(hydratedChatIDs, chat.GetChat().GetId())
		}
		if err = validateNotifyExceptionIDs("chat", idHelper.ChatIdList, hydratedChatIDs); err != nil {
			c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
			return nil, err
		}
		chats = mutableChats.GetChatListByIdList(c.MD.UserId, idHelper.ChatIdList...)
	}

	var channels []*mtproto.Chat
	if len(idHelper.ChannelIdList) > 0 {
		channels = c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, idHelper.ChannelIdList...)
		var hydratedChannelIDs []int64
		for _, channel := range channels {
			if channel == nil || (channel.GetPredicateName() != mtproto.Predicate_channel && channel.GetPredicateName() != mtproto.Predicate_channelForbidden) || channel.GetId() <= 0 {
				err := fmt.Errorf("account.getNotifyExceptions: load channels: malformed channel entity")
				c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
				return nil, err
			}
			hydratedChannelIDs = append(hydratedChannelIDs, channel.GetId())
		}
		if err := validateNotifyExceptionIDs("channel", idHelper.ChannelIdList, hydratedChannelIDs); err != nil {
			c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
			return nil, err
		}
	}

	// Build the response only after every required object has been hydrated.
	rUpdates := mtproto.MakeEmptyUpdates()
	rUpdates.Users = users
	rUpdates.PushChat(chats...)
	rUpdates.PushChat(channels...)
	for _, setting := range settings.GetDatas() {
		if setting.GetPeerType() == mtproto.PEER_USERS || setting.GetPeerType() == mtproto.PEER_CHATS || setting.GetPeerType() == mtproto.PEER_BROADCASTS {
			continue
		}
		peer := mtproto.MakePeerUtil(setting.PeerType, setting.PeerId)
		rUpdates.PushBackUpdate(mtproto.MakeTLUpdateNotifySettings(&mtproto.Update{
			Peer_NOTIFYPEER: peer.ToNotifyPeer(),
			NotifySettings:  setting.Settings,
		}).To_Update())
	}

	return rUpdates, nil
}
