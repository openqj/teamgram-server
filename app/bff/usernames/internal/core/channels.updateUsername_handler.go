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
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsUpdateUsername
// channels.updateUsername#3514b3de channel:InputChannel username:string = Bool;
func (c *UsernamesCore) ChannelsUpdateUsername(in *mtproto.TLChannelsUpdateUsername) (*mtproto.Bool, error) {
	channelId, err := channelID(in.GetChannel())
	if err != nil {
		c.Logger.Errorf("channels.updateUsername - error: %v", err)
		return nil, err
	}
	if err = c.requireChannelAdmin(channelId); err != nil {
		c.Logger.Errorf("channels.updateUsername - error: %v", err)
		return nil, err
	}

	username2 := in.GetUsername()
	current := ""
	got, gerr := c.svcCtx.Dao.UserClient.UserGetChannelUsername(c.ctx, &userpb.TLUserGetChannelUsername{
		ChannelId: channelId,
	})
	if gerr == nil && got != nil {
		current = got.GetUsername()
	}
	if username2 != current {
		if err = c.assignChannelUsername(channelId, current, username2); err != nil {
			c.Logger.Errorf("channels.updateUsername - error: %v", err)
			return nil, err
		}
	}
	if err = channelview.UpdateChannelUsername(channelId, username2); err != nil {
		c.Logger.Errorf("channels.updateUsername - persist channel username: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// assignChannelUsername writes the username row the same way account.updateUsername
// does for users, with the peer set to this channel. It does not touch the users table.
func (c *UsernamesCore) assignChannelUsername(channelId int64, from, username2 string) error {
	if username2 != "" {
		if usernameFormatInvalid(username2) {
			err := mtproto.ErrUsernameInvalid
			c.Logger.Errorf("channels.updateUsername - format error: %v", err)
			return err
		}
		ok, err := c.svcCtx.Dao.UserClient.UserUpdateUsernameByUsername(c.ctx, &userpb.TLUserUpdateUsernameByUsername{
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   channelId,
			Username: username2,
		})
		if err != nil {
			c.Logger.Errorf("channels.updateUsername - error: %v", err)
			return err
		}
		if !mtproto.FromBool(ok) {
			err = mtproto.ErrUsernameOccupied
			c.Logger.Errorf("channels.updateUsername - error: %v", err)
			return err
		}
	}

	if from != "" {
		if _, err := c.svcCtx.Dao.UserClient.UserDeleteUsername(c.ctx, &userpb.TLUserDeleteUsername{
			Username: from,
		}); err != nil {
			c.Logger.Errorf("channels.updateUsername - error: %v", err)
			return err
		}
	}
	return nil
}
