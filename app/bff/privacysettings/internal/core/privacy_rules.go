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

package core

import (
	"fmt"

	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func validatePrivacyHydratedIDs(kind string, requestedIDs, hydratedIDs []int64) error {
	requested := make(map[int64]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[id] = struct{}{}
	}

	hydrated := make(map[int64]struct{}, len(hydratedIDs))
	for _, id := range hydratedIDs {
		if _, ok := requested[id]; !ok {
			return fmt.Errorf("privacy rules: unexpected %s id %d", kind, id)
		}
		if _, ok := hydrated[id]; ok {
			return fmt.Errorf("privacy rules: duplicate %s id %d", kind, id)
		}
		hydrated[id] = struct{}{}
	}

	for _, id := range requestedIDs {
		if _, ok := hydrated[id]; !ok {
			return fmt.Errorf("privacy rules: missing %s id %d", kind, id)
		}
	}
	return nil
}

func (c *PrivacySettingsCore) hydratePrivacyRuleObjects(rules []*mtproto.PrivacyRule) ([]*mtproto.User, []*mtproto.Chat, error) {
	for _, rule := range rules {
		if rule == nil {
			return nil, nil, fmt.Errorf("privacy rules: malformed rule")
		}
	}
	ruleIDs := mtproto.NewIDListHelper()
	ruleIDs.PickByRules(rules)
	if len(ruleIDs.ChannelIdList) > 0 {
		return nil, nil, fmt.Errorf("privacy rules: channel hydration is unavailable")
	}

	users := make([]*mtproto.User, 0, len(ruleIDs.UserIdList))
	if len(ruleIDs.UserIdList) > 0 {
		requestedUserIDs := mtproto.NewIDListHelper(c.MD.UserId)
		requestedUserIDs.AppendUsers(ruleIDs.UserIdList...)
		mutableUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: requestedUserIDs.UserIdList,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("privacy rules: load users: %w", err)
		}
		if mutableUsers == nil {
			return nil, nil, fmt.Errorf("privacy rules: load users: nil response")
		}
		var hydratedIDs []int64
		for _, user := range mutableUsers.GetDatas() {
			if user == nil || user.GetUser() == nil {
				return nil, nil, fmt.Errorf("privacy rules: load users: malformed user entity")
			}
			hydratedIDs = append(hydratedIDs, user.Id())
		}
		if err := validatePrivacyHydratedIDs("user", requestedUserIDs.UserIdList, hydratedIDs); err != nil {
			return nil, nil, err
		}
		users = mutableUsers.GetUserListByIdList(c.MD.UserId, ruleIDs.UserIdList...)
	}

	chats := make([]*mtproto.Chat, 0, len(ruleIDs.ChatIdList))
	if len(ruleIDs.ChatIdList) > 0 {
		mutableChats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
			IdList: ruleIDs.ChatIdList,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("privacy rules: load chats: %w", err)
		}
		if mutableChats == nil {
			return nil, nil, fmt.Errorf("privacy rules: load chats: nil response")
		}
		var hydratedIDs []int64
		for _, chat := range mutableChats.GetDatas() {
			if chat == nil || chat.GetChat() == nil {
				return nil, nil, fmt.Errorf("privacy rules: load chats: malformed chat entity")
			}
			for _, participant := range chat.GetChatParticipants() {
				if participant == nil {
					return nil, nil, fmt.Errorf("privacy rules: load chats: malformed chat participant")
				}
			}
			hydratedIDs = append(hydratedIDs, chat.GetChat().GetId())
		}
		if err := validatePrivacyHydratedIDs("chat", ruleIDs.ChatIdList, hydratedIDs); err != nil {
			return nil, nil, err
		}
		chats = mutableChats.GetChatListByIdList(c.MD.UserId, ruleIDs.ChatIdList...)
	}

	return users, chats, nil
}
