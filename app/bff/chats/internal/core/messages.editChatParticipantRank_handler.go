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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesEditChatParticipantRank
// messages.editChatParticipantRank#a00f32b0 peer:InputPeer participant:InputPeer rank:string = Updates;
func (c *ChatsCore) MessagesEditChatParticipantRank(in *mtproto.TLMessagesEditChatParticipantRank) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil || in.GetParticipant() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var (
		peer        = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
		participant = mtproto.FromInputPeer2(c.MD.UserId, in.Participant)
	)

	if !peer.IsChat() {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.editChatParticipantRank - invalid peer, err: %v", err)
		return nil, err
	}

	if !participant.IsUser() {
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("messages.editChatParticipantRank - invalid participant, err: %v", err)
		return nil, err
	}

	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatEditChatParticipantRank(c.ctx, &chatpb.TLChatEditChatParticipantRank{
		SelfId:      c.MD.UserId,
		ChatId:      peer.PeerId,
		Participant: participant.PeerId,
		Rank:        in.Rank,
	})
	if err != nil {
		c.Logger.Errorf("messages.editChatParticipantRank - error: %v", err)
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
		c.Logger.Errorf("messages.editChatParticipantRank - error: %v", err)
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

	return mtproto.MakeUpdatesByUpdatesUsersChats(
		mUsers.GetUserListByIdList(c.MD.UserId, idList...),
		[]*mtproto.Chat{chat.ToUnsafeChat(c.MD.UserId)},
		updateChatParticipants), nil
}
