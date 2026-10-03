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
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// topPeersState preserves the existing top-peer toggle and reset settings.
type topPeersState struct {
	Enabled *bool               `json:"enabled,omitempty"`
	Hidden  map[string][]string `json:"hidden,omitempty"`
}

func topPeersKey(userId int64) string {
	return fmt.Sprintf("contacts:%d:top_peers", userId)
}

func loadTopPeersState(userId int64) (*topPeersState, error) {
	raw, err := persist.Default.Get(topPeersKey(userId))
	if err != nil {
		return nil, err
	}
	st := &topPeersState{Hidden: map[string][]string{}}
	if raw == "" {
		return st, nil
	}
	if err = json.Unmarshal([]byte(raw), st); err != nil {
		return nil, err
	}
	if st.Hidden == nil {
		st.Hidden = map[string][]string{}
	}
	return st, nil
}

func saveTopPeersState(userId int64, st *topPeersState) error {
	if st.Hidden == nil {
		st.Hidden = map[string][]string{}
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return persist.Default.Set(topPeersKey(userId), string(raw))
}

func topPeerCategory(pred string) *mtproto.TopPeerCategory {
	empty := &mtproto.TopPeerCategory{}
	switch pred {
	case mtproto.Predicate_topPeerCategoryCorrespondents:
		return mtproto.MakeTLTopPeerCategoryCorrespondents(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryBotsPM:
		return mtproto.MakeTLTopPeerCategoryBotsPM(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryBotsInline:
		return mtproto.MakeTLTopPeerCategoryBotsInline(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryPhoneCalls:
		return mtproto.MakeTLTopPeerCategoryPhoneCalls(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryForwardUsers:
		return mtproto.MakeTLTopPeerCategoryForwardUsers(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryForwardChats:
		return mtproto.MakeTLTopPeerCategoryForwardChats(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryGroups:
		return mtproto.MakeTLTopPeerCategoryGroups(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryChannels:
		return mtproto.MakeTLTopPeerCategoryChannels(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryBotsApp:
		return mtproto.MakeTLTopPeerCategoryBotsApp(empty).To_TopPeerCategory()
	case mtproto.Predicate_topPeerCategoryBotsGuestChat:
		return mtproto.MakeTLTopPeerCategoryBotsGuestChat(empty).To_TopPeerCategory()
	default:
		return nil
	}
}

func knownTopPeerCategory(pred string) bool {
	return topPeerCategory(pred) != nil
}

// ContactsGetTopPeers
// contacts.getTopPeers#973478b6 flags:# correspondents:flags.0?true bots_pm:flags.1?true bots_inline:flags.2?true phone_calls:flags.3?true forward_users:flags.4?true forward_chats:flags.5?true groups:flags.10?true channels:flags.15?true offset:int limit:int hash:long = contacts.TopPeers;
func (c *ContactsCore) ContactsGetTopPeers(in *mtproto.TLContactsGetTopPeers) (*mtproto.Contacts_TopPeers, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetOffset() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if in.GetLimit() < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if persist.Default == nil {
		return nil, mtproto.ErrInternalServerError
	}

	state, err := loadTopPeersState(c.MD.UserId)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("contacts.getTopPeers - load state error: %v", err)
		}
		return nil, err
	}
	if state.Enabled != nil && !*state.Enabled {
		return mtproto.MakeTLContactsTopPeersDisabled(&mtproto.Contacts_TopPeers{
			Categories: []*mtproto.TopPeerCategoryPeers{},
			Chats:      []*mtproto.Chat{},
			Users:      []*mtproto.User{},
		}).To_Contacts_TopPeers(), nil
	}

	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	contactList, err := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("contacts.getTopPeers - user.getContactList error: %v", err)
		}
		return nil, err
	}
	if contactList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	idList := make([]int64, 0, len(contactList.GetDatas()))
	for _, contact := range contactList.GetDatas() {
		if contact == nil || contact.GetContactUserId() <= 0 {
			return nil, mtproto.ErrContactIdInvalid
		}
		idList = append(idList, contact.GetContactUserId())
	}

	users := &userpb.Vector_ImmutableUser{}
	if len(idList) > 0 {
		users, err = c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: append([]int64{c.MD.UserId}, idList...),
			To: []int64{c.MD.UserId},
		})
		if err != nil {
			if c.Logger != nil {
				c.Logger.Errorf("contacts.getTopPeers - user.getMutableUsers error: %v", err)
			}
			return nil, err
		}
		if users == nil {
			return nil, mtproto.ErrInternalServerError
		}
		for _, user := range users.GetDatas() {
			if user == nil || user.GetUser() == nil {
				return nil, mtproto.ErrInternalServerError
			}
		}
		if !users.CheckExistUser(append([]int64{c.MD.UserId}, idList...)...) {
			return nil, mtproto.ErrInternalServerError
		}
	}

	categoryIDs := make(map[string][]int64)
	categoryIDs[mtproto.Predicate_topPeerCategoryCorrespondents] = append([]int64(nil), idList...)
	botsPM := make([]int64, 0)
	botsInline := make([]int64, 0)
	botsApp := make([]int64, 0)
	for _, id := range idList {
		immutable, ok := users.GetImmutableUser(id)
		if !ok || immutable == nil || immutable.GetUser() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		bot := immutable.GetUser().GetBot()
		if bot == nil || bot.GetBotNochats() {
			continue
		}
		botsPM = append(botsPM, id)
		if bot.GetBotInlinePlaceholder() != nil || bot.GetBotInlineGeo() {
			botsInline = append(botsInline, id)
		}
		if bot.GetBotHasMainApp() {
			botsApp = append(botsApp, id)
		}
	}
	categoryIDs[mtproto.Predicate_topPeerCategoryBotsPM] = botsPM
	categoryIDs[mtproto.Predicate_topPeerCategoryBotsInline] = botsInline
	categoryIDs[mtproto.Predicate_topPeerCategoryBotsApp] = botsApp
	categoryIDs[mtproto.Predicate_topPeerCategoryBotsGuestChat] = []int64{}

	requested := []struct {
		pred string
		want bool
	}{
		{mtproto.Predicate_topPeerCategoryCorrespondents, in.GetCorrespondents()},
		{mtproto.Predicate_topPeerCategoryBotsPM, in.GetBotsPm()},
		{mtproto.Predicate_topPeerCategoryBotsInline, in.GetBotsInline()},
		{mtproto.Predicate_topPeerCategoryPhoneCalls, in.GetPhoneCalls()},
		{mtproto.Predicate_topPeerCategoryForwardUsers, in.GetForwardUsers()},
		{mtproto.Predicate_topPeerCategoryForwardChats, in.GetForwardChats()},
		{mtproto.Predicate_topPeerCategoryGroups, in.GetGroups()},
		{mtproto.Predicate_topPeerCategoryChannels, in.GetChannels()},
		{mtproto.Predicate_topPeerCategoryBotsApp, in.GetBotsApp()},
		{mtproto.Predicate_topPeerCategoryBotsGuestChat, in.GetBotsGuestchat()},
	}

	categories := make([]*mtproto.TopPeerCategoryPeers, 0, len(requested))
	selectedIDs := make([]int64, 0)
	selected := make(map[int64]struct{})
	for _, category := range requested {
		if !category.want {
			continue
		}
		allIDs := categoryIDs[category.pred]
		hidden := make(map[string]struct{}, len(state.Hidden[category.pred]))
		for _, key := range state.Hidden[category.pred] {
			hidden[key] = struct{}{}
		}
		visibleIDs := make([]int64, 0, len(allIDs))
		for _, id := range allIDs {
			if _, ok := hidden[fmt.Sprintf("%d:%d", mtproto.PEER_USER, id)]; ok {
				continue
			}
			visibleIDs = append(visibleIDs, id)
		}

		count := len(visibleIDs)
		start := int(in.GetOffset())
		if start > count {
			start = count
		}
		end := count
		if limit := int(in.GetLimit()); limit < end-start {
			end = start + limit
		}
		peers := make([]*mtproto.TopPeer, 0, end-start)
		for _, id := range visibleIDs[start:end] {
			peers = append(peers, mtproto.MakeTLTopPeer(&mtproto.TopPeer{
				Peer:   mtproto.MakePeerUser(id),
				Rating: 0,
			}).To_TopPeer())
			if _, ok := selected[id]; !ok {
				selected[id] = struct{}{}
				selectedIDs = append(selectedIDs, id)
			}
		}
		categories = append(categories, mtproto.MakeTLTopPeerCategoryPeers(&mtproto.TopPeerCategoryPeers{
			Category: topPeerCategory(category.pred),
			Count:    int32(count),
			Peers:    peers,
		}).To_TopPeerCategoryPeers())
	}

	responseUsers := make([]*mtproto.User, 0, len(selectedIDs))
	if len(selectedIDs) > 0 {
		responseUsers = users.GetUserListByIdList(c.MD.UserId, selectedIDs...)
	}
	return mtproto.MakeTLContactsTopPeers(&mtproto.Contacts_TopPeers{
		Categories: categories,
		Chats:      []*mtproto.Chat{},
		Users:      responseUsers,
	}).To_Contacts_TopPeers(), nil
}
