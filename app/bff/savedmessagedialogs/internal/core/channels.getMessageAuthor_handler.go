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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsGetMessageAuthor
// channels.getMessageAuthor#ece2a0e6 channel:InputChannel id:int = User;
func (c *SavedMessageDialogsCore) ChannelsGetMessageAuthor(in *mtproto.TLChannelsGetMessageAuthor) (*mtproto.User, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	channelID := inputChannelID(in.GetChannel())
	if channelID == 0 {
		c.Logger.Errorf("channels.getMessageAuthor - error: invalid channel")
		return nil, mtproto.ErrChannelInvalid
	}
	if in.GetId() <= 0 {
		return emptyUser(), nil
	}

	// Only this user's own box. A miss, or a hit in some other peer, is userEmpty
	// so the caller cannot tell whether a hidden message exists.
	box, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &message.TLMessageGetUserMessage{
		UserId: c.MD.UserId,
		Id:     in.GetId(),
	})
	if err != nil || box == nil || !boxInChannel(box, channelID) {
		if err != nil {
			c.Logger.Errorf("channels.getMessageAuthor - error: %v", err)
		}
		return emptyUser(), nil
	}

	authorID := messageAuthorUserID(box)
	if authorID <= 0 {
		return emptyUser(), nil
	}

	if c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{authorID},
	})
	if err != nil {
		c.Logger.Errorf("channels.getMessageAuthor - error: %v", err)
		return nil, err
	}
	if users == nil {
		return emptyUser(), nil
	}
	list := users.GetUserListByIdList(c.MD.UserId, authorID)
	if len(list) == 0 || list[0] == nil {
		return emptyUser(), nil
	}
	return list[0], nil
}
