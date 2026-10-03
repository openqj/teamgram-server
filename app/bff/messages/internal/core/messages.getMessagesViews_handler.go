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
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetMessagesViews
// messages.getMessagesViews#5784d3e1 peer:InputPeer id:Vector<int> increment:Bool = messages.MessageViews;
func (c *MessagesCore) MessagesGetMessagesViews(in *mtproto.TLMessagesGetMessagesViews) (*mtproto.Messages_MessageViews, error) {
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || len(in.GetId()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	for _, id := range in.GetId() {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
	}

	increment := mtproto.FromBool(in.GetIncrement())
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	var views []*mtproto.MessageViews
	var err error

	switch peer.PeerType {
	case mtproto.PEER_SELF:
		views, err = c.messagesViewsFromStoredMessages(peer, in.GetId())
	case mtproto.PEER_USER:
		if err = c.validateMessagesViewsUserPeer(in.GetPeer()); err == nil {
			views, err = c.messagesViewsFromStoredMessages(peer, in.GetId())
		}
	case mtproto.PEER_CHAT:
		if err = c.validateMessagesViewsChatPeer(peer); err == nil {
			views, err = c.messagesViewsFromStoredMessages(peer, in.GetId())
		}
	case mtproto.PEER_CHANNEL:
		views, err = c.messagesViewsFromChannel(peer, in.GetPeer(), in.GetId(), increment)
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err != nil {
		return nil, err
	}
	if increment && peer.PeerType != mtproto.PEER_CHANNEL {
		return nil, mtproto.ErrMethodNotImpl
	}

	return mtproto.MakeTLMessagesMessageViews(&mtproto.Messages_MessageViews{
		Views: views,
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Messages_MessageViews(), nil
}

func (c *MessagesCore) validateMessagesViewsUserPeer(peer *mtproto.InputPeer) error {
	if peer.GetUserId() <= 0 || peer.GetAccessHash() == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrPeerIdInvalid
	}
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{peer.GetUserId()},
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		return err
	}
	if users == nil {
		return mtproto.ErrInternalServerError
	}
	for _, user := range users.GetDatas() {
		if user != nil && user.GetUser() != nil && !user.GetUser().GetDeleted() &&
			user.GetUser().GetId() == peer.GetUserId() && user.GetUser().GetAccessHash() == peer.GetAccessHash() {
			return nil
		}
	}
	return mtproto.ErrPeerIdInvalid
}

func (c *MessagesCore) validateMessagesViewsChatPeer(peer *mtproto.PeerUtil) error {
	if peer == nil || peer.PeerId <= 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
		return mtproto.ErrPeerIdInvalid
	}
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: peer.PeerId,
	})
	if err != nil {
		return err
	}
	if chat == nil {
		return mtproto.ErrPeerIdInvalid
	}
	member, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	if !ok || member == nil || !member.IsChatMemberStateNormal() {
		return mtproto.ErrUserNotParticipant
	}
	return nil
}

func (c *MessagesCore) messagesViewsFromStoredMessages(peer *mtproto.PeerUtil, ids []int32) ([]*mtproto.MessageViews, error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	boxes, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: ids,
	})
	if err != nil {
		return nil, err
	}
	if boxes == nil {
		return nil, mtproto.ErrInternalServerError
	}

	requested := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}
	byID := make(map[int32]*mtproto.MessageBox, len(boxes.GetDatas()))
	for _, box := range boxes.GetDatas() {
		if box == nil || box.GetMessage() == nil || box.GetMessageId() <= 0 || box.GetMessage().GetId() != box.GetMessageId() || !richPeerMatches(peer, box) {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, ok := requested[box.GetMessageId()]; !ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, exists := byID[box.GetMessageId()]; exists {
			return nil, mtproto.ErrMessageIdInvalid
		}
		byID[box.GetMessageId()] = box
	}

	views := make([]*mtproto.MessageViews, 0, len(ids))
	for _, id := range ids {
		box := byID[id]
		if box == nil || box.GetMessageId() != id || box.GetMessage().GetId() != id || !richPeerMatches(peer, box) {
			return nil, mtproto.ErrMessageIdInvalid
		}
		message := box.GetMessage()
		views = append(views, mtproto.MakeTLMessageViews(&mtproto.MessageViews{
			Views:    message.GetViews(),
			Forwards: message.GetForwards(),
			Replies:  message.GetReplies(),
		}).To_MessageViews())
	}
	return views, nil
}

func (c *MessagesCore) messagesViewsFromChannel(peer *mtproto.PeerUtil, input *mtproto.InputPeer, ids []int32, increment bool) ([]*mtproto.MessageViews, error) {
	if peer == nil || peer.PeerId <= 0 || input == nil || input.GetAccessHash() == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	return channelview.MessageViewsForInputPeer(c.MD.UserId, input, ids, increment)
}
