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
	"sort"

	"github.com/teamgram/marmota/pkg/utils"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func commonChatIDs(data *chatpb.Vector_UserChatIdList, selfID, targetID int64) []int64 {
	if data == nil || selfID <= 0 || targetID <= 0 || selfID == targetID {
		return nil
	}
	byUser := make(map[int64][]int64, len(data.GetDatas()))
	for _, item := range data.GetDatas() {
		if item == nil || item.GetUserId() <= 0 {
			continue
		}
		byUser[item.GetUserId()] = append([]int64(nil), item.GetChatIdList()...)
	}
	left, leftOK := byUser[selfID]
	right, rightOK := byUser[targetID]
	if !leftOK || !rightOK {
		return []int64{}
	}
	// Int64Intersect sorts its inputs in place. The copies above keep the
	// service response reusable for logging and future providers.
	return utils.Int64Intersect(utils.Int64Slice(left), utils.Int64Slice(right))
}

func commonChatPage(ids []int64, maxID int64, limit int32) []int64 {
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}
	page := make([]int64, 0, limit)
	for _, id := range ids {
		if maxID > 0 && id <= maxID {
			continue
		}
		page = append(page, id)
		if int32(len(page)) == limit {
			break
		}
	}
	return page
}

func mergeCommonChatIDs(chatIDs, channelIDs []int64) []int64 {
	ids := append(append([]int64(nil), chatIDs...), channelIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	merged := ids[:0]
	for _, id := range ids {
		if len(merged) == 0 || merged[len(merged)-1] != id {
			merged = append(merged, id)
		}
	}
	return merged
}

// MessagesGetCommonChats
// messages.getCommonChats#e40ca104 user_id:InputUser max_id:long limit:int = messages.Chats;
func (c *ChatsCore) MessagesGetCommonChats(in *mtproto.TLMessagesGetCommonChats) (*mtproto.Messages_Chats, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	inputUser := in.GetUserId()
	if inputUser == nil || inputUser.GetPredicateName() != mtproto.Predicate_inputUser || inputUser.GetUserId() <= 0 || inputUser.GetAccessHash() == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	userId := inputUser.GetUserId()
	if userId == c.MD.UserId {
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("messages.getCommonChats - error: %v", err)
		return nil, err
	}

	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{userId},
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		c.Logger.Errorf("messages.getCommonChats - user lookup error: %v", err)
		return nil, err
	}
	if users == nil {
		return nil, mtproto.ErrInternalServerError
	}
	validUser := false
	for _, user := range users.GetDatas() {
		if user != nil && user.GetUser() != nil && user.Id() == userId && user.AccessHash() == inputUser.GetAccessHash() && !user.Deleted() {
			validUser = true
			break
		}
	}
	if !validUser {
		return nil, mtproto.ErrUserIdInvalid
	}

	// chat
	usersChatIdList, err := c.svcCtx.Dao.ChatClient.Client().ChatGetUsersChatIdList(c.ctx, &chatpb.TLChatGetUsersChatIdList{
		Id: []int64{c.MD.UserId, userId},
	})
	if err != nil {
		c.Logger.Errorf("messages.getCommonChats - error: %v", err)
		return nil, err
	}
	if usersChatIdList == nil {
		return nil, mtproto.ErrInternalServerError
	}
	commonChats := commonChatIDs(usersChatIdList, c.MD.UserId, userId)
	commonChannelIDs := channelview.CommonChannelIDs(c.MD.UserId, userId)
	commonChats = mergeCommonChatIDs(commonChats, commonChannelIDs)
	channelIDSet := make(map[int64]struct{}, len(commonChannelIDs))
	for _, id := range commonChannelIDs {
		channelIDSet[id] = struct{}{}
	}
	count := int32(len(commonChats))
	pageIDs := commonChatPage(commonChats, in.GetMaxId(), in.GetLimit())
	chats := make([]*mtproto.Chat, 0, len(pageIDs))
	if len(pageIDs) > 0 {
		basicIDs := make([]int64, 0, len(pageIDs))
		channelIDs := make([]int64, 0, len(pageIDs))
		for _, id := range pageIDs {
			if _, ok := channelIDSet[id]; ok {
				channelIDs = append(channelIDs, id)
			} else {
				basicIDs = append(basicIDs, id)
			}
		}
		byID := make(map[int64]*mtproto.Chat, len(pageIDs))
		if len(basicIDs) > 0 {
			mChats, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
				SelfId: c.MD.UserId,
				IdList: basicIDs,
			})
			if err != nil {
				c.Logger.Errorf("messages.getCommonChats - hydrate error: %v", err)
				return nil, err
			}
			if mChats == nil {
				return nil, mtproto.ErrInternalServerError
			}
			for _, mutable := range mChats.GetDatas() {
				if mutable != nil && mutable.GetChat() != nil {
					byID[mutable.GetChat().GetId()] = mutable.ToUnsafeChat(c.MD.UserId)
				}
			}
		}
		for _, channel := range channelview.ChatsByID(c.MD.UserId, channelIDs) {
			if channel != nil {
				byID[channel.GetId()] = channel
			}
		}
		for _, id := range pageIDs {
			chat, ok := byID[id]
			if !ok || chat == nil {
				return nil, mtproto.ErrInternalServerError
			}
			chats = append(chats, chat)
		}
	}
	if int32(len(pageIDs)) == count && in.GetMaxId() == 0 {
		return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
	}
	return mtproto.MakeTLMessagesChatsSlice(&mtproto.Messages_Chats{Count: count, Chats: chats}).To_Messages_Chats(), nil
}
