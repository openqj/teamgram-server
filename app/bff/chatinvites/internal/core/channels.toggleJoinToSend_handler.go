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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// ChannelsToggleJoinToSend
// channels.toggleJoinToSend#e4cb9580 channel:InputChannel enabled:Bool = Updates;
func (c *ChatInvitesCore) ChannelsToggleJoinToSend(in *mtproto.TLChannelsToggleJoinToSend) (*mtproto.Updates, error) {
	channel := in.GetChannel()
	if channel == nil || channel.GetPredicateName() == mtproto.Predicate_inputChannelEmpty || channel.GetChannelId() == 0 {
		c.Logger.Errorf("channels.toggleJoinToSend - error: channel invalid")
		return nil, mtproto.ErrChannelInvalid
	}
	channelId := channel.GetChannelId()

	handled, err := c.requireChannelInviteAdmin(channel)
	if err != nil {
		return nil, err
	}
	if !handled {
		if err = c.requireInvitePermission(channelId, 0); err != nil {
			return nil, err
		}
	}

	enabled := mtproto.FromBool(in.GetEnabled())
	value := "0"
	if enabled {
		value = "1"
	}
	if err = persist.Default.Set(fmt.Sprintf("channel:%d:join_to_send", channelId), value); err != nil {
		c.Logger.Errorf("channels.toggleJoinToSend - error: %v", err)
		return nil, err
	}

	chat := mtproto.MakeTLChannel(&mtproto.Chat{
		Id:         channelId,
		Megagroup:  true,
		JoinToSend: enabled,
		Photo:      mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
	}).To_Chat()
	update := mtproto.MakeTLUpdateChannel(&mtproto.Update{
		ChannelId: channelId,
	}).To_Update()

	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{chat}, update), nil
}
