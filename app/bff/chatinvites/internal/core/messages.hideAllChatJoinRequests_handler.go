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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesHideAllChatJoinRequests
// messages.hideAllChatJoinRequests#e085f4ea flags:# approved:flags.0?true peer:InputPeer link:flags.1?string = Updates;
func (c *ChatInvitesCore) MessagesHideAllChatJoinRequests(in *mtproto.TLMessagesHideAllChatJoinRequests) (*mtproto.Updates, error) {
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if !peer.IsChat() && !peer.IsChannel() {
		c.Logger.Errorf("messages.hideAllChatJoinRequests - error: peer invalid")
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		if _, err := channelview.ValidateInputPeerInviteAdmin(c.MD.UserId, in.GetPeer()); err != nil {
			return nil, err
		}
		link := ""
		if in.GetLink() != nil {
			link = in.GetLink().GetValue()
		}
		requesters, err := channelview.PendingInviteRequesters(c.MD.UserId, peer.PeerId, link)
		if err != nil {
			return nil, err
		}
		for _, requester := range requesters {
			if _, err = channelview.ResolveInviteRequest(c.MD.UserId, peer.PeerId, requester.GetUserId(), link, in.GetApproved()); err != nil {
				return nil, err
			}
		}
		remaining, err := channelview.PendingInviteRequesters(c.MD.UserId, peer.PeerId, link)
		if err != nil {
			return nil, err
		}
		recent := make([]int64, 0, len(remaining))
		for _, requester := range remaining {
			recent = append(recent, requester.GetUserId())
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
		c.Logger.Errorf("messages.hideAllChatJoinRequests - error: %v", err)
		return nil, mtproto.ErrPeerIdInvalid
	}

	me, _ := mChat.GetImmutableChatParticipant(c.MD.UserId)
	if me == nil || !me.CanInviteUsers() {
		c.Logger.Errorf("messages.hideAllChatJoinRequests - error: admin required")
		return nil, mtproto.ErrChatAdminRequired
	}

	linkScoped := in.GetLink() != nil
	requesterIds := make([]int64, 0)
	if linkScoped {
		requesterIds, err = c.getLinkJoinRequesterIds(peer.PeerId, in.GetLink())
	} else {
		var pending *chatpb.RecentChatInviteRequesters
		pending, err = c.svcCtx.Dao.ChatClient.ChatGetRecentChatInviteRequesters(c.ctx, &chatpb.TLChatGetRecentChatInviteRequesters{
			SelfId: c.MD.UserId,
			ChatId: peer.PeerId,
		})
		if err == nil && pending != nil {
			for _, userId := range pending.GetRecentRequesters() {
				if userId != 0 {
					requesterIds = append(requesterIds, userId)
				}
			}
		}
	}
	if err != nil {
		c.Logger.Errorf("messages.hideAllChatJoinRequests - error: %v", err)
		return nil, hideJoinRequestErr(err)
	}

	var last *chatpb.RecentChatInviteRequesters
	processedRequesterIds := make([]int64, 0, len(requesterIds))
	currentRequesterIds := requesterIds
	for _, userId := range requesterIds {
		last, err = c.svcCtx.Dao.ChatClient.ChatHideChatJoinRequests(c.ctx, &chatpb.TLChatHideChatJoinRequests{
			SelfId:   c.MD.UserId,
			ChatId:   peer.PeerId,
			Approved: in.GetApproved(),
			Link:     in.GetLink(),
			UserId:   wrapperspb.Int64(userId),
		})
		if err != nil {
			c.Logger.Errorf("messages.hideAllChatJoinRequests - error: %v", err)
			return nil, hideJoinRequestErr(err)
		}

		if linkScoped {
			var nextRequesterIds []int64
			if last != nil {
				nextRequesterIds = last.GetRecentRequesters()
			}
			if countJoinRequester(currentRequesterIds, userId) > countJoinRequester(nextRequesterIds, userId) {
				processedRequesterIds = append(processedRequesterIds, userId)
			}
			currentRequesterIds = nextRequesterIds
		} else {
			processedRequesterIds = append(processedRequesterIds, userId)
		}
	}
	if linkScoped {
		last, err = c.svcCtx.Dao.ChatClient.ChatGetRecentChatInviteRequesters(c.ctx, &chatpb.TLChatGetRecentChatInviteRequesters{
			SelfId: c.MD.UserId,
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.hideAllChatJoinRequests - error: %v", err)
			return nil, hideJoinRequestErr(err)
		}
	}

	requestsPending := int32(0)
	var recentRequesters []int64
	if last != nil {
		requestsPending = last.GetRequestsPending()
		recentRequesters = last.GetRecentRequesters()
	}
	updatePendingJoinRequests := mtproto.MakeTLUpdatePendingJoinRequests(&mtproto.Update{
		Peer_PEER:        mtproto.MakePeerChat(mChat.Id()),
		RequestsPending:  requestsPending,
		RecentRequesters: recentRequesters,
	}).To_Update()

	if in.GetApproved() {
		var rUpdates *mtproto.Updates
		for range processedRequesterIds {
			rUpdates, err = c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
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
				c.Logger.Errorf("messages.hideAllChatJoinRequests - error: %v", err)
				return nil, err
			}
		}
		if rUpdates == nil {
			return mtproto.MakeUpdatesByUpdates(updatePendingJoinRequests), nil
		}
		rUpdates.Updates = append(rUpdates.Updates, updatePendingJoinRequests)
		return rUpdates, nil
	}

	mChat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if participant == nil || userId == c.MD.UserId || !participant.CanInviteUsers() {
			return nil
		}
		c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId:  userId,
			Updates: mtproto.MakeUpdatesByUpdates(updatePendingJoinRequests),
		})
		return nil
	})

	return mtproto.MakeUpdatesByUpdates(updatePendingJoinRequests), nil
}

func (c *ChatInvitesCore) getLinkJoinRequesterIds(chatId int64, link *wrapperspb.StringValue) ([]int64, error) {
	const limit = int32(100)
	requesterIds := make([]int64, 0)
	var offsetDate int32
	var offsetUser int64
	for {
		importers, err := c.svcCtx.Dao.ChatClient.ChatGetChatInviteImporters(c.ctx, &chatpb.TLChatGetChatInviteImporters{
			SelfId:     c.MD.UserId,
			ChatId:     chatId,
			Requested:  true,
			Link:       link,
			OffsetDate: offsetDate,
			OffsetUser: offsetUser,
			Limit:      limit,
		})
		if err != nil {
			return nil, err
		}
		datas := importers.GetDatas()
		for _, importer := range datas {
			if importer != nil && importer.GetUserId() != 0 {
				requesterIds = append(requesterIds, importer.GetUserId())
			}
		}
		if int32(len(datas)) < limit {
			return requesterIds, nil
		}
		last := datas[len(datas)-1]
		if last == nil || (offsetDate == last.GetDate() && offsetUser == last.GetUserId()) {
			return requesterIds, nil
		}
		offsetDate = last.GetDate()
		offsetUser = last.GetUserId()
	}
}

func countJoinRequester(requesterIds []int64, userId int64) int {
	count := 0
	for _, requesterId := range requesterIds {
		if requesterId == userId {
			count++
		}
	}
	return count
}

func hideJoinRequestErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "METHOD_NOT_IMPL") || strings.Contains(msg, "ENTERPRISE_IS_BLOCKED") {
		return mtproto.ErrPeerIdInvalid
	}
	return err
}
