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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesEditChatAdmin
// messages.editChatAdmin#a85bd1c2 chat_id:long user_id:InputUser is_admin:Bool = Bool;
func (c *ChatsCore) MessagesEditChatAdmin(in *mtproto.TLMessagesEditChatAdmin) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetChatId() <= 0 || in.GetIsAdmin() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var (
		adminUser = mtproto.FromInputUser(c.MD.UserId, in.UserId)
	)

	if adminUser.PeerType != mtproto.PEER_USER ||
		adminUser.PeerId == c.MD.UserId {
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("messages.editChatAdmin - invalid user_id, err: %v", err)
		return nil, err
	}

	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatEditChatAdmin(c.ctx, &chatpb.TLChatEditChatAdmin{
		ChatId:          in.ChatId,
		OperatorId:      c.MD.UserId,
		EditChatAdminId: adminUser.PeerId,
		IsAdmin:         in.IsAdmin,
	})
	_ = chat
	if err != nil {
		c.Logger.Errorf("messages.editChatAdmin - error: %v", err)
		return nil, err
	}
	if chat == nil || chat.GetChat() == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var (
		idList []int64
	)

	updateChatParticipants := mtproto.MakeTLUpdateChatParticipants(&mtproto.Update{
		Participants_CHATPARTICIPANTS: chat.ToChatParticipants(0),
	}).To_Update()

	chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if participant == nil {
			return mtproto.ErrInternalServerError
		}
		if participant.IsChatMemberStateNormal() {
			idList = append(idList, userId)
		}
		return nil
	})

	mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: idList,
	})
	if err != nil {
		c.Logger.Errorf("messages.editChatAdmin - load users error: %v", err)
		return nil, err
	}
	if mUsers == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var firstErr error
	chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if firstErr != nil {
			return firstErr
		}
		if participant == nil {
			firstErr = mtproto.ErrInternalServerError
			return firstErr
		}
		if !participant.IsChatMemberStateNormal() {
			return nil
		}

		if reply, pushErr := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx,
			&sync.TLSyncPushUpdates{
				UserId: userId,
				Updates: mtproto.MakeUpdatesByUpdatesUsersChats(
					mUsers.GetUserListByIdList(userId, idList...),
					[]*mtproto.Chat{chat.ToUnsafeChat(userId)},
					updateChatParticipants),
			}); pushErr != nil {
			firstErr = pushErr
			return firstErr
		} else if reply == nil {
			firstErr = mtproto.ErrInternalServerError
			return firstErr
		}

		return nil
	})
	if firstErr != nil {
		return nil, firstErr
	}

	return mtproto.BoolTrue, nil
}
