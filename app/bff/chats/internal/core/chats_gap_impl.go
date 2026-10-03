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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

func (c *ChatsCore) loadMutableChat(chatId int64) (*mtproto.MutableChat, error) {
	if chatId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: chatId,
	})
	if err != nil || chat == nil {
		c.Logger.Errorf("chats.loadMutableChat - error: %v", err)
		return nil, mtproto.ErrPeerIdInvalid
	}
	return chat, nil
}

// requireCreatorOrAdmin matches chat.editChatTitle: a normal participant who is creator or admin.
// allowMigrated keeps that check for a chat already migrated (participants are no longer normal).
func (c *ChatsCore) requireCreatorOrAdmin(chat *mtproto.MutableChat, allowMigrated bool) (*mtproto.ImmutableChatParticipant, error) {
	if chat == nil || c.MD == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	me, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	if !ok || me == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if me.IsChatMemberStateNormal() {
		// same state gate as the chat service before CanChangeInfo
	} else if allowMigrated && me.IsChatMemberStateMigrated() {
	} else {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if !me.CanChangeInfo() {
		return nil, mtproto.ErrChatAdminRequired
	}
	return me, nil
}

func chatPeerId(selfId int64, peer *mtproto.InputPeer) (int64, error) {
	if peer == nil {
		return 0, mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(selfId, peer)
	if p == nil || !p.IsChatOrChannel() || p.PeerId == 0 {
		return 0, mtproto.ErrPeerIdInvalid
	}
	return p.PeerId, nil
}

func inputChannelId(channel *mtproto.InputChannel) (int64, error) {
	if channel == nil || channel.GetChannelId() == 0 {
		return 0, mtproto.ErrPeerIdInvalid
	}
	return channel.GetChannelId(), nil
}

func (c *ChatsCore) pushChatUpdates(chat *mtproto.MutableChat, updates *mtproto.Updates) {
	if chat == nil || updates == nil {
		return
	}
	chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if participant == nil || !participant.IsChatMemberStateNormal() {
			return nil
		}
		if _, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId:  userId,
			Updates: updates,
		}); err != nil {
			c.Logger.Errorf("chats.pushChatUpdates - error: %v", err)
		}
		return nil
	})
}

func megagroupChannel(id, accessHash int64, title string, creator, gigagroup bool) *mtproto.Chat {
	ch := &mtproto.Chat{
		Id:        id,
		Title:     title,
		Megagroup: true,
		Gigagroup: gigagroup,
		Creator:   creator,
	}
	if accessHash != 0 {
		ch.AccessHash_FLAGINT64 = mtproto.MakeFlagsInt64(accessHash)
	}
	return mtproto.MakeTLChannel(ch).To_Chat()
}

func markChatMigrated(chat *mtproto.MutableChat, channelId, accessHash int64) {
	if chat == nil || chat.GetChat() == nil {
		return
	}
	chat.Chat.Deactivated = true
	chat.Chat.MigratedTo = mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  channelId,
		AccessHash: accessHash,
	}).To_InputChannel()
}

func participantStamp(p *mtproto.ImmutableChatParticipant) int64 {
	if p.Date != 0 {
		return p.Date
	}
	return p.InvitedAt
}

// futureCreatorUserId is the creator who would remain, otherwise the oldest admin, otherwise the oldest member.
// Zero means the chat would be empty after the caller leaves.
func futureCreatorUserId(chat *mtproto.MutableChat, leaving int64) int64 {
	if chat == nil {
		return 0
	}
	var (
		admin     *mtproto.ImmutableChatParticipant
		adminIdx  int
		member    *mtproto.ImmutableChatParticipant
		memberIdx int
		creator   int64
	)
	for i, p := range chat.GetChatParticipants() {
		if p == nil || p.UserId == leaving || !p.IsChatMemberStateNormal() {
			continue
		}
		if p.IsChatMemberCreator() || p.UserId == chat.Creator() {
			if creator == 0 {
				creator = p.UserId
			}
			continue
		}
		if p.IsChatMemberAdmin() {
			if admin == nil || participantStamp(p) < participantStamp(admin) || (participantStamp(p) == participantStamp(admin) && i < adminIdx) {
				admin = p
				adminIdx = i
			}
			continue
		}
		if member == nil || participantStamp(p) < participantStamp(member) || (participantStamp(p) == participantStamp(member) && i < memberIdx) {
			member = p
			memberIdx = i
		}
	}
	if creator != 0 {
		return creator
	}
	if admin != nil {
		return admin.UserId
	}
	if member != nil {
		return member.UserId
	}
	return 0
}

func stickerSetValue(set *mtproto.InputStickerSet) string {
	if set == nil {
		return "0"
	}
	if set.GetPredicateName() == mtproto.Predicate_inputStickerSetID || set.GetId() != 0 {
		return strconv.FormatInt(set.GetId(), 10)
	}
	if set.GetShortName() != "" {
		return set.GetShortName()
	}
	return "0"
}
