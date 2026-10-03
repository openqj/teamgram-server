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
)

// ChannelsConvertToGigagroup
// channels.convertToGigagroup#b290c69 channel:InputChannel = Updates;
func (c *ChatsCore) ChannelsConvertToGigagroup(in *mtproto.TLChannelsConvertToGigagroup) (*mtproto.Updates, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	channelId, err := inputChannelId(in.GetChannel())
	if err != nil {
		return nil, err
	}
	chat, err := c.loadMutableChat(channelId)
	if err != nil {
		return nil, err
	}
	me, err := c.requireCreatorOrAdmin(chat, false)
	if err != nil {
		c.Logger.Errorf("channels.convertToGigagroup - error: %v", err)
		return nil, err
	}

	var accessHash int64
	if migrated := chat.MigratedTo(); migrated != nil {
		accessHash = migrated.GetAccessHash()
	}
	channel := megagroupChannel(channelId, accessHash, chat.Title(), me.IsChatMemberCreator(), true)
	updates := mtproto.MakeUpdatesByUpdatesChats(
		[]*mtproto.Chat{channel},
		mtproto.MakeTLUpdateChat(&mtproto.Update{ChatId_INT64: channelId}).To_Update(),
	)
	c.pushChatUpdates(chat, updates)
	return updates, nil
}
