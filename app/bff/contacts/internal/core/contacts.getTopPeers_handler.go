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
// contacts.getTopPeers#973478b6 flags:# correspondents:flags.0?true bots_pm:flags.1?true bots_inline:flags.2?true phone_calls:flags.3?true forward_users:flags.4?true forward_chats:flags.5?true groups:flags.10?true channels:flags.15?true bots_app:flags.16?true bots_guestchat:flags.17?true offset:int limit:int hash:long = contacts.TopPeers;
func (c *ContactsCore) ContactsGetTopPeers(in *mtproto.TLContactsGetTopPeers) (*mtproto.Contacts_TopPeers, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetOffset() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if in.GetLimit() < 0 || in.GetLimit() > 100 {
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

	// A contact list is not a Telegram top-peer ranking. There is currently no
	// authoritative interaction/rating provider for any requested category, so
	// do not synthesize peers, ratings, pagination, or hash/not-modified state.
	return nil, mtproto.ErrMethodNotImpl
}
