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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/status"
)

var errChannelUnreadMentionsUnsupported = status.Error(mtproto.ErrBadRequest, "CHANNEL_UNREAD_MENTIONS_UNSUPPORTED")

// MessagesGetUnreadMentions
// messages.getUnreadMentions#46578472 peer:InputPeer offset_id:int add_offset:int limit:int max_id:int min_id:int = messages.Messages;
func (c *MessagesCore) MessagesGetUnreadMentions(in *mtproto.TLMessagesGetUnreadMentions) (*mtproto.Messages_Messages, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}

	var (
		err   error
		peer  = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
		limit = in.GetLimit()
	)

	if c.MD.IsBot {
		err = mtproto.ErrBotMethodInvalid
		c.Logger.Errorf("messages.getUnreadMentions - error: %v", err)
		return nil, err
	}
	if limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if limit > 50 {
		limit = 50
	}

	switch peer.PeerType {
	case mtproto.PEER_CHAT:
		if peer.PeerId <= 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.PEER_CHANNEL:
		if peer.PeerId <= 0 || in.GetPeer().GetAccessHash() == 0 {
			return nil, mtproto.ErrChannelInvalid
		}
		c.Logger.Errorf("messages.getUnreadMentions - error: %v", errChannelUnreadMentionsUnsupported)
		return nil, errChannelUnreadMentionsUnsupported
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(
		c.ctx,
		&chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
	if err != nil {
		c.Logger.Errorf("messages.getUnreadMentions - chat lookup error: %v", err)
		return nil, err
	}
	if chat == nil || chat.GetChat() == nil || chat.Id() != peer.PeerId {
		return nil, mtproto.ErrChatIdInvalid
	}
	for _, participant := range chat.GetChatParticipants() {
		if participant == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}
	member, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	if !ok || member == nil || !member.IsChatMemberStateNormal() {
		return nil, mtproto.ErrUserNotParticipant
	}

	rValues := mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()

	boxList, err := c.svcCtx.Dao.MessageClient.MessageGetUnreadMentions(
		c.ctx,
		&messagepb.TLMessageGetUnreadMentions{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			OffsetId:  in.OffsetId,
			AddOffset: in.AddOffset,
			Limit:     limit,
			MinId:     in.GetMinId(),
			MaxInt:    in.MaxId,
		})
	if err != nil {
		c.Logger.Errorf("messages.getUnreadMentions - message lookup error: %v", err)
		return nil, err
	}
	if boxList == nil {
		return nil, mtproto.ErrInternalServerError
	}
	for _, box := range boxList.GetDatas() {
		if box == nil || box.GetMessage() == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}
	var hydrationErr error

	boxList.Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) {
			rValues.Messages = messageList
		},
		func(userIdList []int64) {
			if hydrationErr != nil || len(userIdList) == 0 {
				return
			}
			mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: userIdList,
				})
			if err != nil {
				hydrationErr = err
				return
			}
			if mUsers == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Users = append(rValues.Users, mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)...)
		},
		func(chatIdList []int64) {
			if hydrationErr != nil || len(chatIdList) == 0 {
				return
			}
			mChats, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			if err != nil {
				hydrationErr = err
				return
			}
			if mChats == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Chats = append(rValues.Chats, mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
		},
		func(channelIdList []int64) {
			if hydrationErr == nil && len(channelIdList) > 0 {
				hydrationErr = mtproto.ErrMethodNotImpl
			}
		})
	if hydrationErr != nil {
		c.Logger.Errorf("messages.getUnreadMentions - entity hydration error: %v", hydrationErr)
		return nil, hydrationErr
	}

	return rValues, nil
}
