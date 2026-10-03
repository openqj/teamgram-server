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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesEditChatCreator
// messages.editChatCreator#f743b857 peer:InputPeer user_id:InputUser password:InputCheckPasswordSRP = Updates;
func (c *ChatsCore) MessagesEditChatCreator(in *mtproto.TLMessagesEditChatCreator) (*mtproto.Updates, error) {
	if in == nil || c.MD == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	chatId, err := chatPeerId(c.MD.UserId, in.Peer)
	if err != nil {
		return nil, err
	}
	if in.UserId == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	target := mtproto.FromInputUser(c.MD.UserId, in.UserId)
	if target.PeerType == mtproto.PEER_SELF {
		target.PeerType = mtproto.PEER_USER
		target.PeerId = c.MD.UserId
	}
	if target.PeerType != mtproto.PEER_USER || target.PeerId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}

	chat, err := c.loadMutableChat(chatId)
	if err != nil {
		return nil, err
	}
	if _, err = c.requireCreatorOrAdmin(chat, false); err != nil {
		c.Logger.Errorf("messages.editChatCreator - error: %v", err)
		return nil, err
	}

	to, ok := chat.GetImmutableChatParticipant(target.PeerId)
	if !ok || to == nil || !to.IsChatMemberStateNormal() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if to.IsChatMemberCreator() || chat.Creator() == target.PeerId {
		return nil, mtproto.ErrChatNotModified
	}

	chat, err = c.svcCtx.Dao.ChatClient.Client().ChatEditChatAdmin(c.ctx, &chatpb.TLChatEditChatAdmin{
		ChatId:          chatId,
		OperatorId:      c.MD.UserId,
		EditChatAdminId: target.PeerId,
		IsAdmin:         mtproto.BoolTrue,
	})
	if err != nil {
		c.Logger.Errorf("messages.editChatCreator - error: %v", err)
		return nil, err
	}

	var idList []int64
	chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if participant != nil && participant.IsChatMemberStateNormal() {
			idList = append(idList, userId)
		}
		return nil
	})

	var users []*mtproto.User
	mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: idList,
	})
	if err != nil {
		c.Logger.Errorf("messages.editChatCreator - error: %v", err)
	} else if mUsers != nil {
		users = mUsers.GetUserListByIdList(c.MD.UserId, idList...)
	}

	updateChatParticipants := mtproto.MakeTLUpdateChatParticipants(&mtproto.Update{
		Participants_CHATPARTICIPANTS: chat.ToChatParticipants(0),
	}).To_Update()
	updates := mtproto.MakeUpdatesByUpdatesUsersChats(
		users,
		[]*mtproto.Chat{chat.ToUnsafeChat(c.MD.UserId)},
		updateChatParticipants,
	)
	c.pushChatUpdates(chat, updates)
	return updates, nil
}
