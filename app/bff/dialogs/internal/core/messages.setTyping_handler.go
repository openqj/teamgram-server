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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesSetTyping
// messages.setTyping#58943ee2 flags:# peer:InputPeer top_msg_id:flags.0?int action:SendMessageAction = Bool;
func (c *DialogsCore) MessagesSetTyping(in *mtproto.TLMessagesSetTyping) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetAction() == nil {
		return nil, mtproto.ErrSendMessageTypeInvalid
	}

	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
		date = int32(time.Now().Unix())
	)
	if peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	pushUpdates := func(userId int64, updates *mtproto.Updates) error {
		response, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId:  userId,
			Updates: updates,
		})
		if err != nil {
			return err
		}
		if response == nil {
			return fmt.Errorf("messages.setTyping: sync.pushUpdates returned no response for user %d", userId)
		}
		return nil
	}

	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER:
		userIDs := []int64{peer.PeerId}
		if c.MD.UserId != peer.PeerId {
			userIDs = append(userIDs, c.MD.UserId)
		}
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: userIDs,
			To: []int64{c.MD.UserId},
		})
		if err != nil {
			return nil, err
		}
		if users == nil {
			return nil, fmt.Errorf("messages.setTyping: user.getMutableUsers returned no response")
		}
		requested := make(map[int64]struct{}, len(userIDs))
		for _, userID := range userIDs {
			requested[userID] = struct{}{}
		}
		resolved := make(map[int64]*mtproto.ImmutableUser, len(userIDs))
		for _, entity := range users.GetDatas() {
			if entity == nil || entity.GetUser() == nil {
				return nil, fmt.Errorf("messages.setTyping: user.getMutableUsers returned a malformed entity")
			}
			userID := entity.GetUser().GetId()
			if _, ok := requested[userID]; !ok {
				return nil, fmt.Errorf("messages.setTyping: user.getMutableUsers returned unexpected user %d", userID)
			}
			if _, ok := resolved[userID]; ok {
				return nil, fmt.Errorf("messages.setTyping: user.getMutableUsers returned duplicate user %d", userID)
			}
			resolved[userID] = entity
		}
		for _, userID := range userIDs {
			if _, ok := resolved[userID]; !ok {
				return nil, mtproto.ErrUserIdInvalid
			}
		}
		user := resolved[peer.PeerId]
		if user.Deleted() {
			return nil, mtproto.ErrUserIdInvalid
		}
		if peer.PeerType == mtproto.PEER_USER && user.AccessHash() != peer.AccessHash {
			return nil, mtproto.ErrUserIdInvalid
		}
		updates := mtproto.MakeTLUpdateShort(&mtproto.Updates{
			Update: mtproto.MakeTLUpdateUserTyping(&mtproto.Update{
				UserId: c.MD.UserId,
				Action: in.Action,
			}).To_Update(),
			Date: date,
		}).To_Updates()
		if err := pushUpdates(peer.PeerId, updates); err != nil {
			return nil, err
		}
	case mtproto.PEER_CHAT:
		chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err != nil {
			return nil, err
		}
		if chat == nil || chat.GetChat() == nil || chat.GetChat().GetId() != peer.PeerId {
			return nil, fmt.Errorf("messages.setTyping: chat.getMutableChat returned no matching chat")
		}
		var actorIsMember bool
		for _, participant := range chat.GetChatParticipants() {
			if participant == nil || participant.GetUserId() <= 0 {
				return nil, fmt.Errorf("messages.setTyping: chat.getMutableChat returned a malformed participant")
			}
			if participant.GetUserId() == c.MD.UserId && participant.IsChatMemberStateNormal() {
				actorIsMember = true
			}
		}
		if !actorIsMember {
			return nil, mtproto.ErrUserNotParticipant
		}

		updates := mtproto.MakeTLUpdateShort(&mtproto.Updates{
			// updateChatUserTyping#86cadb6c chat_id:int from_id:Peer action:SendMessageAction = Update;
			Update: mtproto.MakeTLUpdateChatUserTyping(&mtproto.Update{
				ChatId_INT64: peer.PeerId,
				FromId:       mtproto.MakePeerUser(c.MD.UserId),
				Action:       in.Action,
			}).To_Update(),
			Date: date,
		}).To_Updates()
		recipients := make(map[int64]struct{}, len(chat.GetChatParticipants()))
		for _, participant := range chat.GetChatParticipants() {
			if participant.GetUserId() == c.MD.UserId || !participant.IsChatMemberStateNormal() {
				continue
			}
			recipients[participant.GetUserId()] = struct{}{}
		}
		for userId := range recipients {
			if err := pushUpdates(userId, updates); err != nil {
				return nil, err
			}
		}
	case mtproto.PEER_CHANNEL:
		if c.svcCtx.Plugin == nil {
			return nil, mtproto.ErrMethodNotImpl
		}
		recipients, err := c.svcCtx.Plugin.GetChannelTypingRecipients(c.ctx, c.MD.UserId, in.GetPeer())
		if err != nil {
			return nil, err
		}
		update := &mtproto.Update{
			ChannelId:          peer.PeerId,
			TopMsgId_FLAGINT32: in.GetTopMsgId(),
			FromId:             mtproto.MakePeerUser(c.MD.UserId),
			Action:             in.Action,
		}
		updates := mtproto.MakeTLUpdateShort(&mtproto.Updates{
			Update: mtproto.MakeTLUpdateChannelUserTyping(update).To_Update(),
			Date:   date,
		}).To_Updates()
		for _, userID := range recipients {
			if err := pushUpdates(userID, updates); err != nil {
				return nil, err
			}
		}
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	return mtproto.BoolTrue, nil
}
