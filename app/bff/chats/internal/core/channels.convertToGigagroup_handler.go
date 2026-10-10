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
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/state"
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
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	channelState, err := state.ConvertChannelToGigagroup(c.MD.UserId, channelId, in.GetChannel().GetAccessHash())
	if err != nil {
		c.Logger.Errorf("channels.convertToGigagroup - error: %v", err)
		switch {
		case errors.Is(err, state.ErrInvalidChannelAccessHash), errors.Is(err, state.ErrChannelMissing), errors.Is(err, state.ErrChannelConversionInvalid):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, state.ErrChannelAdminRequired):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	channel := megagroupChannel(channelState.ID, channelState.AccessHash, channelState.Title, channelState.Creator == c.MD.UserId, true)
	updates := mtproto.MakeUpdatesByUpdatesChats(
		[]*mtproto.Chat{channel},
		mtproto.MakeTLUpdateChannel(&mtproto.Update{ChannelId: channelId}).To_Update(),
	)
	return updates, nil
}
