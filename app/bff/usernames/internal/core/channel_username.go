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
	"github.com/teamgram/marmota/pkg/strings2"
	"github.com/teamgram/marmota/pkg/utils"
	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func channelID(channel *mtproto.InputChannel) (int64, error) {
	if channel == nil || channel.GetPredicateName() == mtproto.Predicate_inputChannelEmpty || channel.GetChannelId() == 0 {
		return 0, mtproto.ErrChannelInvalid
	}
	return channel.GetChannelId(), nil
}

func usernameFormatInvalid(username string) bool {
	return len(username) < userpb.MinUsernameLen ||
		!strings2.IsAlNumString(username) ||
		utils.IsNumber(username[0])
}

func (c *UsernamesCore) requireChannelAdmin(channelId int64) error {
	found := false
	if c.svcCtx.Plugin != nil {
		for _, ch := range c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, channelId) {
			if ch == nil || ch.GetId() != channelId {
				continue
			}
			found = true
			if channelChatCanChangeInfo(ch) {
				return nil
			}
		}
	}
	if c.channelChatsByID != nil {
		for _, ch := range c.channelChatsByID(c.MD.UserId, []int64{channelId}) {
			if ch == nil || ch.GetId() != channelId {
				continue
			}
			found = true
			if channelChatCanChangeInfo(ch) {
				return nil
			}
		}
	}
	if !found && c.svcCtx.Plugin != nil {
		return mtproto.ErrChannelInvalid
	}
	return mtproto.ErrChatAdminRequired
}

func channelChatCanChangeInfo(ch *mtproto.Chat) bool {
	if ch.GetCreator() {
		return true
	}
	rights := ch.GetAdminRights()
	return rights != nil && (rights.CanChangeInfo() || rights.HasAdminRights())
}
