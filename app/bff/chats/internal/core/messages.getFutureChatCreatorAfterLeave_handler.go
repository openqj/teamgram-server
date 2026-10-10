// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
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

package core

import (
	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetFutureChatCreatorAfterLeave
// messages.getFutureChatCreatorAfterLeave#3b7d0ea6 peer:InputPeer = User;
func (c *ChatsCore) MessagesGetFutureChatCreatorAfterLeave(in *mtproto.TLMessagesGetFutureChatCreatorAfterLeave) (*mtproto.User, error) {
	if in == nil || c.MD == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	chatId, err := chatPeerId(c.MD.UserId, in.Peer)
	if err != nil {
		return nil, err
	}
	chat, err := c.loadMutableChat(chatId)
	if err != nil {
		return nil, err
	}
	me, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	if !ok || me == nil || !me.IsChatMemberStateNormal() {
		c.Logger.Errorf("messages.getFutureChatCreatorAfterLeave - not participant: %d", chatId)
		return nil, mtproto.ErrPeerIdInvalid
	}

	userId := futureCreatorUserId(chat, c.MD.UserId)
	if userId == 0 {
		return mtproto.MakeTLUserEmpty(&mtproto.User{Id: 0}).To_User(), nil
	}

	mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{c.MD.UserId, userId},
	})
	if err != nil {
		c.Logger.Errorf("messages.getFutureChatCreatorAfterLeave - error: %v", err)
		return nil, err
	}
	if mUsers == nil {
		return nil, mtproto.ErrInternalServerError
	}
	users := mUsers.GetUserListByIdList(c.MD.UserId, userId)
	if len(users) == 0 || users[0] == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	return users[0], nil
}
