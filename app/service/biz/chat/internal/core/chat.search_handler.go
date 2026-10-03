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
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatSearch
// chat.search self_id:long q:string offset:long limit:int = Vector<UserChatIdList>;
func (c *ChatCore) ChatSearch(in *chat.TLChatSearch) (*chat.Vector_MutableChat, error) {
	var (
		chatList = &chat.Vector_MutableChat{
			Datas: []*mtproto.MutableChat{},
		}
	)
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetSelfId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetOffset() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}

	// Check query string and limit
	if len(in.GetQ()) < 3 || in.GetLimit() <= 0 {
		return chatList, nil
	}

	limit := in.GetLimit()
	if limit > 50 {
		limit = 50
	}

	ids, err := c.svcCtx.Dao.ChatsDAO.SearchByQueryStringForUserOffset(c.ctx, in.GetSelfId(), "%"+in.GetQ()+"%", in.GetOffset(), limit)
	if err != nil {
		c.Logger.Errorf("chat.search - search error: %v", err)
		return nil, err
	}
	for _, id := range ids {
		mutableChat, err := c.svcCtx.Dao.GetExcludeParticipantsMutableChat(c.ctx, id)
		if err != nil {
			c.Logger.Errorf("chat.search - chat hydration error: %v", err)
			return nil, err
		}
		if mutableChat == nil || mutableChat.GetChat() == nil || mutableChat.GetChat().GetId() != id {
			return nil, mtproto.ErrInternalServerError
		}
		chatList.Datas = append(chatList.Datas, mutableChat)
	}

	return chatList, nil
}
