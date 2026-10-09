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
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (c *ChatsCore) createChat(iUsers []*mtproto.InputUser, chatTitle string, ttlPeriod *wrapperspb.Int32Value) (*mtproto.Messages_InvitedUsers, error) {
	var (
		chatUserIdList  []int64
		userAddList     = make([]int64, 0)
		botAddList      = make([]int64, 0)
		missingInvitees = make([]*mtproto.MissingInvitee, 0)
	)

	// check chat title
	if chatTitle == "" {
		err := mtproto.ErrChatTitleEmpty
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	if len(iUsers) == 0 {
		err := mtproto.ErrUsersTooFew
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	// check user too much
	if len(iUsers) > 200-1 {
		err := mtproto.ErrUsersTooMuch
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	// check len(users)
	chatUserIdList = []int64{c.MD.UserId}
	for _, u := range iUsers {
		if u == nil || u.PredicateName != mtproto.Predicate_inputUser || u.GetUserId() <= 0 || u.GetAccessHash() == 0 {
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("messages.createChat - error: %v", err)
			return nil, err
		} else {
			chatUserIdList = append(chatUserIdList, u.UserId)
		}
	}

	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: chatUserIdList,
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		return nil, err
	}
	if users == nil {
		return nil, mtproto.ErrInternalServerError
	}

	me, _ := users.GetImmutableUser(c.MD.UserId)
	if me == nil || me.GetUser() == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if me.Restricted() {
		err := mtproto.ErrUserRestricted
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	for _, u := range iUsers {
		if addUser, ok := users.GetImmutableUser(u.UserId); !ok {
			err := mtproto.ErrInputUserDeactivated
			c.Logger.Errorf("messages.createChat - error: %v", err)
			return nil, err
		} else {
			if addUser.Deleted() || addUser.GetUser().GetUserType() == userpb.UserTypeDeleted {
				return nil, mtproto.ErrInputUserDeactivated
			}
			if addUser.AccessHash() != u.GetAccessHash() {
				return nil, mtproto.ErrUserIdInvalid
			}
			if addUser.IsBot() {
				if addUser.BotNochats() {
					c.Logger.Errorf("user is bot and nochats, ignore %d", u.UserId)
					continue
				} else {
					botAddList = append(botAddList, addUser.Id())
				}
			} else {
				allowed, err := c.svcCtx.Dao.UserClient.UserCheckPrivacy(c.ctx, &userpb.TLUserCheckPrivacy{
					UserId:  addUser.Id(),
					KeyType: mtproto.CHAT_INVITE,
					PeerId:  c.MD.UserId,
				})
				if err != nil {
					return nil, err
				}
				if allowed == nil {
					return nil, mtproto.ErrInternalServerError
				}
				if !mtproto.FromBool(allowed) {
					missingInvitees = append(missingInvitees, mtproto.MakeTLMissingInvitee(&mtproto.MissingInvitee{
						PremiumWouldAllowInvite: false,
						PremiumRequiredForPm:    false,
						UserId:                  u.UserId,
					}).To_MissingInvitee())
					continue
				}
				userAddList = append(userAddList, addUser.Id())
			}
		}
	}

	if len(userAddList)+len(botAddList) == 0 {
		err := mtproto.ErrUsersTooFew
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatCreateChat2(c.ctx, &chatpb.TLChatCreateChat2{
		CreatorId:  c.MD.UserId,
		UserIdList: userAddList,
		Title:      chatTitle,
		Bots:       botAddList,
		TtlPeriod:  ttlPeriod,
	})
	if err != nil {
		c.Logger.Errorf("createChat duplicate: %v", err)
		return nil, err
	}

	outMessages := []*msgpb.OutboxMessage{
		msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage:  true,
			Background: false,
			RandomId:   rand.Int63(),
			Message: mtproto.MakeTLMessageService(&mtproto.Message{
				Out:                  true,
				Mentioned:            false,
				MediaUnread:          false,
				ReactionsArePossible: false,
				Silent:               false,
				Post:                 false,
				Legacy:               false,
				Id:                   0,
				FromId:               mtproto.MakePeerUser(c.MD.UserId),
				PeerId:               mtproto.MakePeerChat(chat.Id()),
				SavedPeerId:          nil,
				ReplyTo:              nil,
				Date:                 int32(time.Now().Unix()),
				Action: mtproto.MakeTLMessageActionChatCreate(&mtproto.MessageAction{
					Title:            chatTitle,
					Title_FLAGSTRING: mtproto.MakeFlagsString(chatTitle),
					Title_STRING:     chatTitle,
					Users:            append(userAddList, botAddList...),
				}).To_MessageAction(),
				Reactions: nil,
				TtlPeriod: ttlPeriod,
			}).To_Message(),
			ScheduleDate: nil,
		}).To_OutboxMessage(),
	}
	if ttlPeriod != nil {
		outMessages = append(outMessages, msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage:  true,
			Background: false,
			RandomId:   rand.Int63(),
			Message: mtproto.MakeTLMessageService(&mtproto.Message{
				Out:                  true,
				Mentioned:            false,
				MediaUnread:          false,
				ReactionsArePossible: true,
				Silent:               false,
				Post:                 false,
				Legacy:               false,
				Id:                   0,
				FromId:               mtproto.MakePeerUser(c.MD.UserId),
				PeerId:               mtproto.MakePeerChat(chat.Id()),
				SavedPeerId:          nil,
				ReplyTo:              nil,
				Date:                 int32(time.Now().Unix()),
				Action: mtproto.MakeTLMessageActionSetMessagesTTL(&mtproto.MessageAction{
					Period:          ttlPeriod.Value,
					AutoSettingFrom: mtproto.MakeFlagsInt64(c.MD.UserId),
				}).To_MessageAction(),
				Reactions: nil,
				TtlPeriod: nil,
			}).To_Message(),
			ScheduleDate: nil,
		}).To_OutboxMessage())
	}
	// TODO: add attach_data (chat and chat_participants)
	rValue, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_CHAT,
		PeerId:    chat.Chat.Id,
		Message:   outMessages,
	})
	if err != nil {
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	return mtproto.MakeTLMessagesInvitedUsers(&mtproto.Messages_InvitedUsers{
		Updates:         rValue,
		MissingInvitees: missingInvitees,
	}).To_Messages_InvitedUsers(), nil
}
