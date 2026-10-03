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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsCheckUsername
// channels.checkUsername#10e6bd2c channel:InputChannel username:string = Bool;
func (c *UsernamesCore) ChannelsCheckUsername(in *mtproto.TLChannelsCheckUsername) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	channelId, err := channelID(in.GetChannel())
	if err != nil {
		c.Logger.Errorf("channels.checkUsername - error: %v", err)
		return nil, err
	}
	if usernameFormatInvalid(in.GetUsername()) {
		err = mtproto.ErrUsernameInvalid
		c.Logger.Errorf("channels.checkUsername - format error: %v", err)
		return nil, err
	}

	existed, err := c.svcCtx.Dao.UserClient.UserCheckChannelUsername(c.ctx, &userpb.TLUserCheckChannelUsername{
		ChannelId: channelId,
		Username:  in.GetUsername(),
	})
	if err != nil {
		c.Logger.Errorf("channels.checkUsername - error: %v", err)
		return nil, err
	}
	if existed == nil {
		c.Logger.Errorf("channels.checkUsername - user service returned an empty response")
		return nil, mtproto.ErrInternalServerError
	}
	switch existed.GetPredicateName() {
	case userpb.Predicate_usernameExistedNotMe:
		return mtproto.BoolFalse, nil
	case userpb.Predicate_usernameNotExisted, userpb.Predicate_usernameExistedIsMe:
		return mtproto.BoolTrue, nil
	default:
		c.Logger.Errorf("channels.checkUsername - user service returned unknown predicate: %q", existed.GetPredicateName())
		return nil, mtproto.ErrInternalServerError
	}
}
