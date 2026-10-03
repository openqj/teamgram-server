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
	"math/rand"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesHideChatJoinRequest
// messages.hideChatJoinRequest#7fe7e815 flags:# approved:flags.0?true peer:InputPeer user_id:InputUser = Updates;
func (c *ChatInvitesCore) MessagesHideChatJoinRequest(in *mtproto.TLMessagesHideChatJoinRequest) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetUserId() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}

	var (
		peer           = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
		userId         = mtproto.FromInputUser(c.MD.UserId, in.GetUserId())
		pushUserIdList = make([]int64, 0)
		pushUsers      *user.Vector_ImmutableUser
	)

	if userId.PeerId <= 0 {
		c.Logger.Errorf("messages.hideChatJoinRequest - error: invalid user")
		return nil, mtproto.ErrUserIdInvalid
	}
	if userId.IsSelf() {
		c.Logger.Errorf("messages.hideChatJoinRequest - error: method MessagesHideChatJoinRequest not impl")
		return nil, mtproto.ErrUserIdInvalid
	}

	if (!peer.IsChat() && !peer.IsChannel()) || peer.PeerId <= 0 {
		c.Logger.Errorf("messages.hideChatJoinRequest - error: method MessagesHideChatJoinRequest not impl")
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		if _, err := channelview.ValidateInputPeerInviteAdmin(c.MD.UserId, in.GetPeer()); err != nil {
			return nil, err
		}
		remaining, err := channelview.ResolveInviteRequest(c.MD.UserId, peer.PeerId, userId.PeerId, "", in.GetApproved())
		if err != nil {
			return nil, err
		}
		recent := make([]int64, 0, len(remaining))
		for _, importer := range remaining {
			recent = append(recent, importer.GetUserId())
		}
		update := mtproto.MakeTLUpdatePendingJoinRequests(&mtproto.Update{
			Peer_PEER:        mtproto.MakePeerChannel(peer.PeerId),
			RequestsPending:  int32(len(recent)),
			RecentRequesters: recent,
		}).To_Update()
		return mtproto.MakeUpdatesByUpdates(update), nil
	}

	mChat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: peer.PeerId,
	})
	if err != nil {
		c.Logger.Errorf("messages.hideChatJoinRequest - error: %v", err)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if mChat == nil {
		c.Logger.Errorf("messages.hideChatJoinRequest - error: chat provider returned nil")
		return nil, mtproto.ErrMethodNotImpl
	}

	me, _ := mChat.GetImmutableChatParticipant(c.MD.UserId)
	if me == nil || !me.CanInviteUsers() {
		c.Logger.Errorf("messages.hideChatJoinRequest - error: %v", err)
		return nil, mtproto.ErrChatAdminRequired
	}
	join, _ := mChat.GetImmutableChatParticipant(userId.PeerId)
	if join != nil && join.IsChatMemberStateNormal() {
		c.Logger.Errorf("messages.hideChatJoinRequest - error: %v", err)
		return nil, mtproto.ErrUserIdInvalid
	}
	pendingBefore, err := c.svcCtx.Dao.ChatClient.ChatGetRecentChatInviteRequesters(c.ctx, &chatpb.TLChatGetRecentChatInviteRequesters{
		SelfId: c.MD.UserId,
		ChatId: peer.PeerId,
	})
	if err != nil {
		return nil, err
	}
	if pendingBefore == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	requested := false
	for _, requesterID := range pendingBefore.GetRecentRequesters() {
		if requesterID == userId.PeerId {
			requested = true
			break
		}
	}
	if !requested {
		return mtproto.MakeEmptyUpdates(), nil
	}

	pendingJoinRequests, err := c.svcCtx.Dao.ChatClient.ChatHideChatJoinRequests(c.ctx, &chatpb.TLChatHideChatJoinRequests{
		SelfId:   c.MD.UserId,
		ChatId:   peer.PeerId,
		Approved: in.GetApproved(),
		Link:     nil,
		UserId:   mtproto.MakeFlagsInt64(userId.PeerId),
	})
	if err != nil {
		return nil, err
	}
	if pendingJoinRequests == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	updatePendingJoinRequests := mtproto.MakeTLUpdatePendingJoinRequests(&mtproto.Update{
		Peer_PEER:        mtproto.MakePeerChat(mChat.Id()),
		RequestsPending:  pendingJoinRequests.RequestsPending,
		RecentRequesters: pendingJoinRequests.RecentRequesters,
	}).To_Update()

	pushUserIdList = append(pushUserIdList, pendingJoinRequests.GetRecentRequesters()...)
	if len(pushUserIdList) > 0 {
		if c.svcCtx.Dao.UserClient == nil {
			return nil, mtproto.ErrMethodNotImpl
		}
		mChat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
			if participant.CanInviteUsers() {
				pushUserIdList = append(pushUserIdList, participant.UserId)
			}
			return nil
		})
		pushUsers, err = c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &user.TLUserGetMutableUsers{
			Id: pushUserIdList,
		})
		if err != nil {
			return nil, err
		}
		if pushUsers == nil {
			return nil, mtproto.ErrMethodNotImpl
		}
	}

	if in.GetApproved() {
		// The basic-chat RPC has no invite-link field, so approval uses the
		// chat-scoped participant provider above.
		if c.svcCtx.Dao.MsgClient == nil {
			return nil, mtproto.ErrMethodNotImpl
		}
		rUpdates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(
			c.ctx,
			&msgpb.TLMsgSendMessageV2{
				UserId:    c.MD.UserId,
				AuthKeyId: c.MD.PermAuthKeyId,
				PeerType:  mtproto.PEER_CHAT,
				PeerId:    mChat.Id(),
				Message: []*msgpb.OutboxMessage{
					msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
						NoWebpage:    true,
						Background:   false,
						RandomId:     rand.Int63(),
						Message:      mChat.MakeMessageService(c.MD.UserId, mtproto.MakeMessageActionChatJoinedByRequest()),
						ScheduleDate: nil,
					}).To_OutboxMessage(),
				},
			})
		if err != nil {
			c.Logger.Errorf("messages.importChatInvite - error: %v", err)
			return nil, err
		}
		if rUpdates == nil {
			return nil, mtproto.ErrMethodNotImpl
		}

		rUpdates.Updates = append(rUpdates.Updates, updatePendingJoinRequests)
		return rUpdates, nil
	} else {
		var pushErr error
		mChat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
			if c.MD.UserId == participant.UserId {
				return nil
			}

			if participant.CanInviteUsers() {
				if c.svcCtx.Dao.SyncClient == nil {
					pushErr = mtproto.ErrMethodNotImpl
					return pushErr
				}
				var pushUpdates *mtproto.Updates
				if len(pushUserIdList) > 0 && pushUsers != nil {
					pushUpdates = mtproto.MakeUpdatesByUpdatesUsers(
						pushUsers.GetUserListByIdList(participant.UserId, pendingJoinRequests.RecentRequesters...),
						updatePendingJoinRequests)
				} else {
					pushUpdates = mtproto.MakeUpdatesByUpdates(updatePendingJoinRequests)
				}
				_, pushErr = c.svcCtx.Dao.SyncClient.SyncPushUpdates(
					c.ctx,
					&sync.TLSyncPushUpdates{
						UserId:  participant.UserId,
						Updates: pushUpdates,
					},
				)
				if pushErr != nil {
					return pushErr
				}
			}
			return nil
		})
		if pushErr != nil {
			return nil, pushErr
		}

		var (
			rUpdates *mtproto.Updates
		)

		if len(pushUserIdList) > 0 && pushUsers != nil {
			rUpdates = mtproto.MakeUpdatesByUpdatesUsers(
				pushUsers.GetUserListByIdList(c.MD.UserId, pendingJoinRequests.RecentRequesters...),
				updatePendingJoinRequests)
		} else {
			rUpdates = mtproto.MakeUpdatesByUpdates(
				updatePendingJoinRequests)
		}

		return rUpdates, nil
	}
}
