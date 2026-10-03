// Copyright 2026 Teamgram Authors
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
	"sort"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/schedstore"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"google.golang.org/protobuf/proto"
)

// RPCScheduledMessagesServer stores pending messages through schedstore, which
// is also used by messages.sendMessage's schedule_date path.

func scheduledBox(msgs []*mtproto.Message) *mtproto.Messages_Messages {
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()
}

func scheduledUpdates(ups []*mtproto.Update) *mtproto.Updates {
	if ups == nil {
		ups = []*mtproto.Update{}
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: ups,
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
		Date:    int32(time.Now().Unix()),
	}).To_Updates()
}

func scheduledPeer(c *ApiFullCore, userID int64, input *mtproto.InputPeer) (*mtproto.Peer, error) {
	if input == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(userID, input)
	if !peer.CanDoSendMessage() || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		if err := c.authorizeReactionPeer(userID, input); err != nil {
			return nil, err
		}
	}
	result := peer.ToPeer()
	if result == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return result, nil
}

func scheduledPeerAddress(peer *mtproto.Peer) (int32, int64) {
	if peer == nil {
		return 0, 0
	}
	switch {
	case peer.GetUserId() != 0:
		return mtproto.PEER_USER, peer.GetUserId()
	case peer.GetChatId() != 0:
		return mtproto.PEER_CHAT, peer.GetChatId()
	case peer.GetChannelId() != 0:
		return mtproto.PEER_CHANNEL, peer.GetChannelId()
	default:
		return 0, 0
	}
}

func sameScheduledPeer(left, right *mtproto.Peer) bool {
	leftType, leftID := scheduledPeerAddress(left)
	rightType, rightID := scheduledPeerAddress(right)
	return leftType != 0 && leftType == rightType && leftID == rightID
}

func scheduledForPeer(userID int64, peer *mtproto.Peer) ([]*mtproto.Message, error) {
	all, err := schedstore.List(userID)
	if err != nil {
		return nil, err
	}
	msgs := make([]*mtproto.Message, 0, len(all))
	for _, msg := range all {
		if msg != nil && sameScheduledPeer(msg.GetPeerId(), peer) {
			msgs = append(msgs, msg)
		}
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].GetId() < msgs[j].GetId() })
	return msgs, nil
}

func selectedScheduled(userID int64, peer *mtproto.Peer, ids []int32) ([]*mtproto.Message, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	all, err := schedstore.List(userID)
	if err != nil {
		return nil, err
	}
	byID := make(map[int32]*mtproto.Message, len(all))
	for _, msg := range all {
		if msg != nil {
			byID[msg.GetId()] = msg
		}
	}
	msgs := make([]*mtproto.Message, 0, len(ids))
	seen := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		msg := byID[id]
		if msg == nil || !sameScheduledPeer(msg.GetPeerId(), peer) {
			return nil, mtproto.ErrMessageIdInvalid
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func scheduledRandomID(userID int64, messageID int32) int64 {
	return (int64(messageID) << 32) | (userID & 0xffffffff)
}

func scheduledOutbox(userID int64, peer *mtproto.Peer, pending *mtproto.Message) *msgpb.OutboxMessage {
	message := proto.Clone(pending).(*mtproto.Message)
	message.Id = 0
	message.Out = true
	message.FromScheduled = false
	message.FromId = mtproto.MakePeerUser(userID)
	message.PeerId = peer
	message.Date = int32(time.Now().Unix())
	return msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
		RandomId: scheduledRandomID(userID, pending.GetId()),
		Message:  message,
	}).To_OutboxMessage()
}

func appendScheduledDelete(updates *mtproto.Updates, peer *mtproto.Peer, ids []int32) *mtproto.Updates {
	deleted := mtproto.MakeTLUpdateDeleteScheduledMessages(&mtproto.Update{
		Peer_PEER: peer,
		Messages:  ids,
	}).To_Update()
	if updates == nil {
		return scheduledUpdates([]*mtproto.Update{deleted})
	}
	updates.Updates = append(updates.Updates, deleted)
	return updates
}

func (c *ApiFullCore) MessagesGetScheduledHistory(in *mtproto.TLMessagesGetScheduledHistory) (*mtproto.Messages_Messages, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	peer, err := scheduledPeer(c, userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	msgs, err := scheduledForPeer(userID, peer)
	if err != nil {
		return nil, err
	}
	return scheduledBox(msgs), nil
}

func (c *ApiFullCore) MessagesGetScheduledMessages(in *mtproto.TLMessagesGetScheduledMessages) (*mtproto.Messages_Messages, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	peer, err := scheduledPeer(c, userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	msgs, err := selectedScheduled(userID, peer, in.GetId())
	if err != nil {
		return nil, err
	}
	return scheduledBox(msgs), nil
}

func (c *ApiFullCore) MessagesSendScheduledMessages(in *mtproto.TLMessagesSendScheduledMessages) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	peer, err := scheduledPeer(c, userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	pending, err := selectedScheduled(userID, peer, in.GetId())
	if err != nil {
		return nil, err
	}
	dao := c.apifullDao()
	if dao == nil || dao.ScheduledMessageSender == nil {
		return nil, mtproto.ErrInternalServerError
	}
	peerType, peerID := scheduledPeerAddress(peer)
	outbox := make([]*msgpb.OutboxMessage, 0, len(pending))
	ids := make([]int32, 0, len(pending))
	for _, msg := range pending {
		outbox = append(outbox, scheduledOutbox(userID, peer, msg))
		ids = append(ids, msg.GetId())
	}
	updates, err := dao.ScheduledMessageSender.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    userID,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peerType,
		PeerId:    peerID,
		Message:   outbox,
	})
	if err != nil {
		return nil, err
	}
	deleted, err := schedstore.Delete(userID, ids)
	if err != nil {
		return nil, err
	}
	if len(deleted) != len(ids) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return appendScheduledDelete(updates, peer, deleted), nil
}

func (c *ApiFullCore) MessagesSendMessage(in *mtproto.TLMessagesSendMessage) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetMessage() == "" {
		return nil, mtproto.ErrMessageEmpty
	}
	peer, err := scheduledPeer(c, userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	when := in.GetScheduleDate().GetValue()
	if when == 0 {
		return nil, mtproto.ErrScheduleDateInvalid
	}
	return schedstore.Append(userID, peer, in.GetMessage(), when)
}

func (c *ApiFullCore) MessagesDeleteScheduledMessages(in *mtproto.TLMessagesDeleteScheduledMessages) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	peer, err := scheduledPeer(c, userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	pending, err := selectedScheduled(userID, peer, in.GetId())
	if err != nil {
		return nil, err
	}
	ids := make([]int32, 0, len(pending))
	for _, msg := range pending {
		ids = append(ids, msg.GetId())
	}
	deleted, err := schedstore.Delete(userID, ids)
	if err != nil {
		return nil, err
	}
	return appendScheduledDelete(scheduledUpdates(nil), peer, deleted), nil
}
