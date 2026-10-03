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
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesSearchGlobal
// messages.searchGlobal#4bc6589a flags:# folder_id:flags.0?int q:string filter:MessagesFilter min_date:int max_date:int offset_rate:int offset_peer:InputPeer offset_id:int limit:int = messages.Messages;
func (c *MessagesCore) MessagesSearchGlobal(in *mtproto.TLMessagesSearchGlobal) (*mtproto.Messages_Messages, error) {
	// 400	BOT_METHOD_INVALID	This method can't be used by a bot
	// 400	SEARCH_QUERY_EMPTY	The search query is empty
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.MD.IsBot {
		err := mtproto.ErrBotMethodInvalid
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		return nil, err
	}

	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}

	if in.GetQ() == "" {
		err := mtproto.ErrSearchQueryEmpty
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		return nil, err
	}
	if err := validateSearchGlobalOptions(in); err != nil {
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		return nil, err
	}

	var (
		offset = int32(math.MaxInt32)
		limit  = in.GetLimit()
	)

	if limit > 50 {
		limit = 50
	}
	if limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	rValues := mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()

	boxList, err := c.svcCtx.Dao.MessageClient.MessageSearchGlobal(
		c.ctx,
		&messagepb.TLMessageSearchGlobal{
			UserId: c.MD.UserId,
			Q:      in.GetQ(),
			Offset: offset,
			Limit:  limit,
		})
	if err != nil {
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		return nil, err
	}
	if boxList == nil {
		err = mtproto.ErrInternalServerError
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		return nil, err
	}

	var hydrationErr error
	boxList.Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) {
			rValues.Messages = messageList
		},
		func(userIdList []int64) {
			if hydrationErr != nil || len(userIdList) == 0 {
				return
			}
			if c.svcCtx.Dao.UserClient == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: userIdList,
				})
			if err != nil {
				hydrationErr = err
				return
			}
			if mUsers == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Users = append(rValues.Users, mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)...)
		},
		func(chatIdList []int64) {
			if hydrationErr != nil || len(chatIdList) == 0 {
				return
			}
			if c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			mChats, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			if err != nil {
				hydrationErr = err
				return
			}
			if mChats == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Chats = append(rValues.Chats, mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
		},
		func(channelIdList []int64) {
			if hydrationErr != nil || len(channelIdList) == 0 {
				return
			}
			chats := channelview.ChatsByID(c.MD.UserId, channelIdList)
			if len(chats) != len(channelIdList) {
				hydrationErr = mtproto.ErrMethodNotImpl
				return
			}
			rValues.Chats = append(rValues.Chats, chats...)
		})
	if hydrationErr != nil {
		c.Logger.Errorf("messages.searchGlobal - hydration error: %v", hydrationErr)
		return nil, hydrationErr
	}

	return rValues, nil
}

func validateSearchGlobalOptions(in *mtproto.TLMessagesSearchGlobal) error {
	filter := in.GetFilter()
	if filter == nil {
		return mtproto.ErrInputFilterInvalid
	}
	filterType := mtproto.FromMessagesFilter(filter)
	if filterType == mtproto.FilterEmpty {
		if filter.GetPredicateName() != mtproto.Predicate_inputMessagesFilterEmpty {
			return mtproto.ErrInputFilterInvalid
		}
	} else {
		return mtproto.ErrMethodNotImpl
	}

	offsetPeer := in.GetOffsetPeer()
	if offsetPeer == nil {
		return mtproto.ErrOffsetPeerIdInvalid
	}
	if offsetPeer.GetPredicateName() != mtproto.Predicate_inputPeerEmpty {
		return mtproto.ErrMethodNotImpl
	}

	if in.GetFolderId() != nil || in.GetCommunity() != nil || in.GetBroadcastsOnly() || in.GetGroupsOnly() || in.GetUsersOnly() ||
		in.GetMinDate() != 0 || in.GetMaxDate() != 0 || in.GetOffsetRate() != 0 || in.GetOffsetId() != 0 {
		return mtproto.ErrMethodNotImpl
	}

	return nil
}
