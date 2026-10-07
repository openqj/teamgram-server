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
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsResolveUsername
// contacts.resolveUsername#f93ccba3 username:string = contacts.ResolvedPeer;
func (c *UsernamesCore) ContactsResolveUsername(in *mtproto.TLContactsResolveUsername) (*mtproto.Contacts_ResolvedPeer, error) {
	// TODO(@benqi):
	// 401	AUTH_KEY_PERM_EMPTY	The temporary auth key must be binded to the permanent auth key to use these methods.
	// 401	SESSION_PASSWORD_NEEDED	2FA is enabled, use a password to login
	// 400	USERNAME_INVALID	The provided username is not valid
	// 400	USERNAME_NOT_OCCUPIED	The provided username is not occupied
	//
	var (
		peer *mtproto.PeerUtil
	)

	id := userpb.GetBotIdByName(in.GetUsername())
	if id > 0 {
		peer = mtproto.MakeUserPeerUtil(id)
	} else {
		rName, err := c.svcCtx.Dao.UserClient.UserResolveUsername(c.ctx, &userpb.TLUserResolveUsername{
			Username: in.GetUsername(),
		})
		if err != nil {
			c.Logger.Errorf("contacts.resolveUsername - reply: {%v}", err)
			return nil, err
		}

		peer = mtproto.FromPeer(rName)
	}

	resolvedPeer := mtproto.MakeTLContactsResolvedPeer(&mtproto.Contacts_ResolvedPeer{
		Peer:  peer.ToPeer(),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Contacts_ResolvedPeer()

	switch peer.PeerType {
	case mtproto.PEER_USER:
		mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{c.MD.UserId, peer.PeerId},
		})
		if err != nil {
			c.Logger.Errorf("contacts.resolveUsername - user.getMutableUsers error: %v", err)
			return nil, err
		}
		if mUsers == nil {
			c.Logger.Errorf("contacts.resolveUsername - user.getMutableUsers returned no response")
			return nil, mtproto.ErrInternalServerError
		}
		for _, user := range mUsers.GetDatas() {
			if user == nil || user.GetUser() == nil {
				c.Logger.Errorf("contacts.resolveUsername - user.getMutableUsers returned an incomplete user")
				return nil, mtproto.ErrInternalServerError
			}
		}
		if !mUsers.CheckExistUser(c.MD.UserId, peer.PeerId) {
			c.Logger.Errorf("contacts.resolveUsername - user.getMutableUsers omitted requester or resolved user")
			return nil, mtproto.ErrInternalServerError
		}
		resolvedPeer.Users = mUsers.GetUserListByIdList(c.MD.UserId, peer.PeerId)
		if len(resolvedPeer.Users) != 1 || resolvedPeer.Users[0] == nil || resolvedPeer.Users[0].GetId() != peer.PeerId {
			c.Logger.Errorf("contacts.resolveUsername - user.getMutableUsers did not hydrate resolved user %d", peer.PeerId)
			return nil, mtproto.ErrInternalServerError
		}
	case mtproto.PEER_CHAT:
		chat, _ := c.svcCtx.Dao.ChatClient.ChatGetChatBySelfId(c.ctx, &chat.TLChatGetChatBySelfId{
			SelfId: c.MD.UserId,
			ChatId: peer.PeerId,
		})
		if chat != nil {
			resolvedPeer.Chats = []*mtproto.Chat{chat.ToUnsafeChat(c.MD.UserId)}
		}
	case mtproto.PEER_CHANNEL:
		channels := c.channelChatsByID(c.MD.UserId, []int64{peer.PeerId})
		if len(channels) == 0 && c.svcCtx.Plugin != nil {
			channels = c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, peer.PeerId)
		}
		for _, channel := range channels {
			if channel != nil && channel.GetId() == peer.PeerId && channel.GetPredicateName() == mtproto.Predicate_channel {
				resolvedPeer.Chats = []*mtproto.Chat{channel}
				break
			}
		}
		if len(resolvedPeer.Chats) == 0 {
			return nil, mtproto.ErrChannelInvalid
		}
	}

	return resolvedPeer, nil
}
