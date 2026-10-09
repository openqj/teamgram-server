// Copyright 2025 Teamgram Authors
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
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetChannelUsername
// user.getChannelUsername channel_id:long = UsernameData;
func (c *UserCore) UserGetChannelUsername(in *user.TLUserGetChannelUsername) (*user.UsernameData, error) {
	list, err := c.svcCtx.Dao.SelectUsernamesByPeer(c.ctx, mtproto.PEER_CHANNEL, in.ChannelId)
	if err != nil {
		c.Logger.Errorf("username.getChannelUsername - error: %v", err)
		return nil, err
	}
	if len(list) == 0 || list[0].Username == "" {
		err = mtproto.ErrUsernameNotOccupied
		c.Logger.Errorf("username.getChannelUsername - error: %v", err)
		return nil, err
	}

	return user.MakeTLUsernameData(&user.UsernameData{
		Username: list[0].Username,
		Peer:     mtproto.MakePeerChannel(in.ChannelId),
		Editable: list[0].Editable,
		Active:   list[0].Active,
	}).To_UsernameData(), nil
}
