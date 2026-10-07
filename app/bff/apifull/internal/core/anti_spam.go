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
	"encoding/json"
	"errors"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// persistSetJSON stores v under key prefix "set:" so it does not collide with autosave keys.
func persistSetJSON(userID int64, method string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return persist.Default.Set("set:"+strconv.FormatInt(userID, 10)+":"+method, string(raw))
}

// RPCAntiSpamServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) ChannelsToggleAntiSpam(in *mtproto.TLChannelsToggleAntiSpam) (*mtproto.Updates, error) {
	if in == nil || in.GetChannel() == nil || in.GetEnabled() == nil {
		if _, err := c.requireUserId(); err != nil {
			return nil, err
		}
		return nil, mtproto.ErrChannelInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	enabled := mtproto.FromBool(in.GetEnabled())
	if _, err = c.resolveMemberChannel(uid, in.GetChannel(), in.GetChannel().GetChannelId()); err != nil {
		return nil, err
	}
	if err = domain.UpdateChannelSettings(uid, in.GetChannel().GetChannelId(), domain.ChannelSettings{Antispam: &enabled}); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	channel, ok, err := domain.LoadChannel(in.GetChannel().GetChannelId())
	if err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, mtproto.ErrChannelInvalid
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{channelview.Chat(channel, channel.Creator == uid)}), nil
}

func (c *ApiFullCore) ChannelsReportAntiSpamFalsePositive(in *mtproto.TLChannelsReportAntiSpamFalsePositive) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	channel, err := c.resolveMemberChannel(uid, in.GetChannel(), in.GetChannel().GetChannelId())
	if err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(uid, channel.ID); err != nil {
		return nil, err
	}
	messages, err := domain.ChannelMessagesByID(uid, channel.ID, []int32{in.GetMsgId()})
	if err != nil {
		return nil, err
	}
	if len(messages) != 1 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if err = c.recordReport(uid, "channels.reportAntiSpamFalsePositive", reportTarget{typ: "channel", id: channel.ID}, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}
