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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// MessagesMigrateChat
// messages.migrateChat#a2875319 chat_id:long = Updates;
func (c *ChatsCore) MessagesMigrateChat(in *mtproto.TLMessagesMigrateChat) (*mtproto.Updates, error) {
	if in == nil || in.ChatId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	chat, err := c.loadMutableChat(in.ChatId)
	if err != nil {
		return nil, err
	}

	migrated := chat.MigratedTo()
	me, err := c.requireCreatorOrAdmin(chat, migrated != nil)
	if err != nil {
		c.Logger.Errorf("messages.migrateChat - error: %v", err)
		return nil, err
	}

	var channelId, accessHash int64
	var replyUpdates *mtproto.Updates
	if migrated != nil {
		channelId = migrated.GetChannelId()
		accessHash = migrated.GetAccessHash()
	} else {
		channelId = c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
		accessHash = c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
		if channelId == 0 || accessHash == 0 {
			c.Logger.Errorf("messages.migrateChat - idgen returned empty id")
			return nil, mtproto.ErrInternalServerError
		}
		replyUpdates, err = c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
			UserId:    c.MD.UserId,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  mtproto.PEER_CHAT,
			PeerId:    in.ChatId,
			Message: []*msgpb.OutboxMessage{
				msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
					NoWebpage:  true,
					Background: false,
					RandomId:   rand.Int63(),
					Message:    chat.MakeMessageService(c.MD.UserId, mtproto.MakeMessageActionChatMigrateTo(channelId)),
				}).To_OutboxMessage(),
			},
		})
		if err != nil {
			c.Logger.Errorf("messages.migrateChat - error: %v", err)
			return nil, err
		}

		if _, err = c.svcCtx.Dao.ChatClient.Client().ChatMigratedToChannel(c.ctx, &chatpb.TLChatMigratedToChannel{
			Chat:       chat,
			Id:         channelId,
			AccessHash: accessHash,
		}); err != nil {
			c.Logger.Errorf("messages.migrateChat - error: %v", err)
			return nil, err
		}
		markChatMigrated(chat, channelId, accessHash)
	}
	if err = channelview.ImportMigratedChat(chat, channelId, accessHash); err != nil {
		c.Logger.Errorf("messages.migrateChat - project native channel: %v", err)
		return nil, err
	}

	basic := chat.ToUnsafeChat(c.MD.UserId)
	channel := megagroupChannel(channelId, accessHash, chat.Title(), me.IsChatMemberCreator(), false)
	notice := mtproto.MakeUpdatesByUpdatesChats(
		[]*mtproto.Chat{basic, channel},
		mtproto.MakeTLUpdateChat(&mtproto.Update{ChatId_INT64: in.ChatId}).To_Update(),
	)
	if migrated == nil {
		c.pushChatUpdates(chat, notice)
	}
	if replyUpdates == nil {
		return notice, nil
	}
	replyUpdates.Chats = append(replyUpdates.Chats, basic, channel)
	return replyUpdates, nil
}
