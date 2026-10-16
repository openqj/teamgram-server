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
	"github.com/zeromicro/go-zero/core/mr"
)

// MessagesGetHistory
// messages.getHistory#4423e6c5 peer:InputPeer offset_id:int offset_date:int add_offset:int limit:int max_id:int min_id:int hash:long = messages.Messages;
func (c *MessagesCore) MessagesGetHistory(in *mtproto.TLMessagesGetHistory) (*mtproto.Messages_Messages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetLimit() <= 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if in.GetOffsetId() < 0 || in.GetOffsetDate() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if in.GetMaxId() < 0 || in.GetMinId() < 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	// The count provider cannot apply these bounds, so returning a
	// messagesSlice would fabricate exact pagination metadata.
	if in.GetMaxId() != 0 || in.GetMinId() != 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		err   error
		peer  = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
		limit = in.Limit
	)
	if peer == nil || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	if limit > 50 {
		limit = 50
	}
	if in.GetAddOffset() > 0 && int64(in.GetAddOffset())+int64(limit) > int64(1<<31-1) {
		return nil, mtproto.ErrOffsetInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF:
	case mtproto.PEER_USER:
		if err = c.validateMessagesGetHistoryUserPeer(in.GetPeer()); err != nil {
			return nil, err
		}
	case mtproto.PEER_CHAT:
		if err = c.validateMessagesGetHistoryChatPeer(peer); err != nil {
			return nil, err
		}
	case mtproto.PEER_CHANNEL:
		// channelview currently implements only offset_id and limit.
		if in.GetOffsetDate() != 0 || in.GetAddOffset() != 0 || in.GetHash() != 0 {
			return nil, mtproto.ErrMethodNotImpl
		}
		return channelview.HistoryForInputPeer(c.MD.UserId, in.GetPeer(), in.OffsetId, limit)
		//// 400	CHANNEL_INVALID	The provided channel is invalid
		//// 400	CHANNEL_PRIVATE	You haven't joined this channel/supergroup
		//channel, err = c.svcCtx.Dao.ChannelClient.ChannelGetMutableChannel(
		//	c.ctx,
		//	&channelpb.TLChannelGetMutableChannel{
		//		ChannelId: peer.PeerId,
		//		Id:        []int64{c.MD.UserId},
		//	})
		//if err != nil {
		//	err = mtproto.ErrChannelInvalid
		//	c.Logger.Errorf("messages.getHistory - error: %v", err)
		//	return nil, err
		//}
		//
		//me := channel.GetImmutableChannelParticipant(c.MD.UserId)
		//if me != nil {
		//	// TODO
		//	// minId = mathx.MaxInt(int(me.AvailableMinId), int(in.MinId))
		//}
		//
	default:
		err = mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.getHistory - error: %v", err)
		return nil, err
	}

	var (
		boxList *message.Vector_MessageBox
		count   *mtproto.Int32
	)

	err = mr.Finish(
		func() error {
			var err2 error
			boxList, err2 = c.svcCtx.Dao.MessageClient.MessageGetHistoryMessages(c.ctx, &message.TLMessageGetHistoryMessages{
				UserId:     c.MD.UserId,
				PeerType:   peer.PeerType,
				PeerId:     peer.PeerId,
				OffsetId:   in.OffsetId,
				OffsetDate: in.OffsetDate,
				AddOffset:  in.AddOffset,
				Limit:      limit,
				MaxId:      in.MaxId,
				MinId:      in.MinId,
				Hash:       in.Hash,
			})
			if err2 != nil {
				c.Logger.Errorf("messages.getHistory - error: %v", err2)
			}

			return err2
		},
		func() error {
			var err2 error
			count, err2 = c.svcCtx.Dao.MessageClient.MessageGetHistoryMessagesCount(
				c.ctx,
				&message.TLMessageGetHistoryMessagesCount{
					UserId:   c.MD.UserId,
					PeerType: peer.PeerType,
					PeerId:   peer.PeerId,
				})
			if err2 != nil {
				c.Logger.Errorf("messages.getHistory - error: %v", err2)
			}

			return err2
		})
	if err != nil {
		return nil, err
	}
	if boxList == nil || count == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if count.GetV() < 0 {
		return nil, mtproto.ErrInternalServerError
	}
	if err = validateMessagesGetHistoryBoxes(boxList, limit, in.GetAddOffset()); err != nil {
		return nil, err
	}
	if in.GetAddOffset() > 0 {
		// The provider fetches add_offset+limit rows for a backward page. The
		// public method returns only the requested window after that offset.
		skip := int(in.GetAddOffset())
		if skip >= len(boxList.GetDatas()) {
			boxList.Datas = []*mtproto.MessageBox{}
		} else {
			boxList.Datas = boxList.GetDatas()[skip:]
		}
	}
	if count.GetV() < boxList.Length() {
		return nil, mtproto.ErrInternalServerError
	}

	var (
		messages   []*mtproto.Message
		users      []*mtproto.User
		chats      []*mtproto.Chat
		hydrateErr error
	)
	boxList.Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) {
			messages = messageList
		},
		func(userIdList []int64) {
			if c.svcCtx.Dao.UserClient == nil {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: userIdList,
				})
			if err != nil {
				hydrateErr = err
				return
			}
			if mUsers == nil {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			users = append(users, mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)...)
		},
		func(chatIdList []int64) {
			if c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			mChats, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			if err != nil {
				hydrateErr = err
				return
			}
			if mChats == nil {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			chats = append(chats, mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
		},
		func(channelIdList []int64) {
			resolved := channelview.ChatsByID(c.MD.UserId, channelIdList)
			if len(resolved) != len(channelIdList) {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			chats = append(chats, resolved...)
		})
	if hydrateErr != nil {
		return nil, hydrateErr
	}
	if int32(len(messages)) != boxList.Length() {
		return nil, mtproto.ErrInternalServerError
	}

	var (
		rValues *mtproto.Messages_Messages
	)

	//if !isChannel {
	if boxList.Length() == limit {
		rValues = mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
			Inexact:        false, // TODO: ???
			Count:          count.V,
			NextRate:       nil, // TODO: ???
			OffsetIdOffset: nil, // TODO: ???
			Messages:       messages,
			Users:          mtproto.ToSafeUsers(users),
			Chats:          mtproto.ToSafeChats(chats),
		}).To_Messages_Messages()
	} else {
		rValues = mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
			Messages: messages,
			Users:    mtproto.ToSafeUsers(users),
			Chats:    mtproto.ToSafeChats(chats),
		}).To_Messages_Messages()
	}
	//} else {
	//	rValues = mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
	//		Inexact:        false, // TODO: ???
	//		Pts:            channel.Pts(),
	//		Count:          count.V,
	//		OffsetIdOffset: &wrapperspb.Int32Value{Value: channel.TopMessage() - in.OffsetId}, // TODO: ???
	//		Messages:       messages,
	//		Chats:          mtproto.ToSafeChats(chats),
	//		Users:          mtproto.ToSafeUsers(users),
	//	}).To_Messages_Messages()
	//}

	return rValues, nil
}

func (c *MessagesCore) validateMessagesGetHistoryUserPeer(input *mtproto.InputPeer) error {
	if input == nil || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrInternalServerError
	}
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{input.GetUserId()},
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		return err
	}
	if users == nil {
		return mtproto.ErrInternalServerError
	}
	for _, user := range users.GetDatas() {
		if user == nil || user.GetUser() == nil {
			continue
		}
		data := user.GetUser()
		if data.GetId() == input.GetUserId() && !data.GetDeleted() && data.GetAccessHash() == input.GetAccessHash() {
			return nil
		}
	}
	return mtproto.ErrPeerIdInvalid
}

func (c *MessagesCore) validateMessagesGetHistoryChatPeer(peer *mtproto.PeerUtil) error {
	if peer == nil || peer.PeerId <= 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
		return mtproto.ErrInternalServerError
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
	for _, participant := range chat.GetChatParticipants() {
		if participant == nil {
			return mtproto.ErrInternalServerError
		}
	}
	member, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	if !ok || member == nil || !member.IsChatMemberStateNormal() {
		return mtproto.ErrUserNotParticipant
	}
	return nil
}

func validateMessagesGetHistoryBoxes(boxList *message.Vector_MessageBox, limit, addOffset int32) error {
	if boxList == nil {
		return mtproto.ErrInternalServerError
	}
	maxBoxes := int64(limit)
	if addOffset > 0 {
		maxBoxes += int64(addOffset)
	}
	if int64(len(boxList.GetDatas())) > maxBoxes {
		return mtproto.ErrInternalServerError
	}
	for _, box := range boxList.GetDatas() {
		if box == nil || box.GetMessage() == nil || box.GetMessageId() <= 0 || box.GetMessage().GetId() != box.GetMessageId() || box.GetPeerId() <= 0 {
			return mtproto.ErrInternalServerError
		}
		switch box.GetPeerType() {
		case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		default:
			return mtproto.ErrInternalServerError
		}
	}
	return nil
}
