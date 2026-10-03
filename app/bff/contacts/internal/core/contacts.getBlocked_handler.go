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

// ContactsGetBlocked
// contacts.getBlocked#9a868f80 flags:# my_stories_from:flags.0?true offset:int limit:int = contacts.Blocked;
func (c *ContactsCore) ContactsGetBlocked(in *mtproto.TLContactsGetBlocked) (*mtproto.Contacts_Blocked, error) {
	var (
		limit           = in.Limit
		contactsBlocked *mtproto.Contacts_Blocked
	)

	if limit > 50 {
		limit = 50
	}

	blockedList, err := c.svcCtx.Dao.UserClient.UserGetBlockedList(c.ctx, &userpb.TLUserGetBlockedList{
		UserId: c.MD.UserId,
		Offset: in.Offset,
		Limit:  limit,
	})
	if err != nil {
		c.Logger.Errorf("contacts.getBlocked - error: %v", err)
		return nil, err
	}

	if len(blockedList.GetDatas()) > 0 {
		// TODO(@benqi): layer119
		contactsBlocked = mtproto.MakeTLContactsBlockedSlice(&mtproto.Contacts_Blocked{
			Blocked: make([]*mtproto.PeerBlocked, 0, len(blockedList.GetDatas())),
			Chats:   nil,
			Users:   nil,
			Count:   blockedList.GetTotalCount(),
		}).To_Contacts_Blocked()

		var (
			idHelper = mtproto.NewIDListHelper(c.MD.UserId)
			readErr  error
		)

		for _, blocked := range blockedList.GetDatas() {
			peer := blocked.GetPeerId()
			idHelper.PickByPeer(peer)
			// idHelper.AppendUsers(blocked.GetPeerId().GetUserId())
			contactsBlocked.Blocked = append(contactsBlocked.Blocked, mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
				PeerId: blocked.GetPeerId(),
				Date:   blocked.Date,
			}).To_PeerBlocked())
		}

		idHelper.Visit(
			func(userIdList []int64) {
				if readErr != nil {
					return
				}
				users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
					&userpb.TLUserGetMutableUsers{
						Id: userIdList,
					})
				if err != nil {
					readErr = err
					return
				}
				contactsBlocked.Users = users.GetUserListByIdList(c.MD.UserId, userIdList...)
			},
			func(chatIdList []int64) {
				if readErr != nil {
					return
				}
				chats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
					SelfId: c.MD.UserId,
					IdList: chatIdList,
				})
				if err != nil {
					readErr = err
					return
				}
				contactsBlocked.Chats = append(contactsBlocked.Chats, chats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
			},
			func(channelIdList []int64) {
				if readErr != nil {
					return
				}
				if c.svcCtx.Plugin == nil {
					readErr = mtproto.ErrInternalServerError
					return
				}
				contactsBlocked.Chats = append(contactsBlocked.Chats, c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, channelIdList...)...)
			})
		if readErr != nil {
			c.Logger.Errorf("contacts.getBlocked - error: %v", readErr)
			return nil, readErr
		}
	} else {
		contactsBlocked = mtproto.MakeTLContactsBlockedSlice(&mtproto.Contacts_Blocked{
			Blocked: []*mtproto.PeerBlocked{},
			Chats:   []*mtproto.Chat{},
			Users:   []*mtproto.User{},
			Count:   blockedList.GetTotalCount(),
		}).To_Contacts_Blocked()
	}

	return contactsBlocked, nil
}
