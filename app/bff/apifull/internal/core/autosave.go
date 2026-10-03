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
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"encoding/json"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCAutosaveServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const autoSaveKeyPrefix = "autosave:"

type autoSaveException struct {
	Key string                     `json:"key"`
	Exc *mtproto.AutoSaveException `json:"exc,omitempty"`
}

type autoSaveState struct {
	Users      *mtproto.AutoSaveSettings `json:"users,omitempty"`
	Chats      *mtproto.AutoSaveSettings `json:"chats,omitempty"`
	Broadcasts *mtproto.AutoSaveSettings `json:"broadcasts,omitempty"`
	Exceptions []autoSaveException       `json:"exceptions,omitempty"`
}

func autoSaveKey(userID int64) string {
	return autoSaveKeyPrefix + strconv.FormatInt(userID, 10)
}

func loadAutoSave(userID int64) (*autoSaveState, error) {
	raw, err := persist.Default.Get(autoSaveKey(userID))
	if err != nil || raw == "" {
		return &autoSaveState{}, err
	}
	var st autoSaveState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return &autoSaveState{}, err
	}
	return &st, nil
}

func saveAutoSave(userID int64, st *autoSaveState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return persist.Default.Set(autoSaveKey(userID), string(b))
}

func emptyAutoSave() *mtproto.AutoSaveSettings {
	return mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{}).To_AutoSaveSettings()
}

func autoSaveView(st *autoSaveState) *mtproto.Account_AutoSaveSettings {
	out := mtproto.MakeTLAccountAutoSaveSettings(&mtproto.Account_AutoSaveSettings{
		UsersSettings:      emptyAutoSave(),
		ChatsSettings:      emptyAutoSave(),
		BroadcastsSettings: emptyAutoSave(),
		Exceptions:         []*mtproto.AutoSaveException{},
		Chats:              []*mtproto.Chat{},
		Users:              []*mtproto.User{},
	}).To_Account_AutoSaveSettings()
	if st == nil {
		return out
	}
	if st.Users != nil {
		out.UsersSettings = st.Users
	}
	if st.Chats != nil {
		out.ChatsSettings = st.Chats
	}
	if st.Broadcasts != nil {
		out.BroadcastsSettings = st.Broadcasts
	}
	if len(st.Exceptions) == 0 {
		return out
	}
	excs := make([]*mtproto.AutoSaveException, 0, len(st.Exceptions))
	for _, e := range st.Exceptions {
		if e.Exc != nil {
			excs = append(excs, e.Exc)
		}
	}
	out.Exceptions = excs
	return out
}

func (c *ApiFullCore) AccountGetAutoSaveSettings(in *mtproto.TLAccountGetAutoSaveSettings) (*mtproto.Account_AutoSaveSettings, error) {
	_ = in
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	st, err := loadAutoSave(userID)
	if err != nil {
		return nil, err
	}
	return autoSaveView(st), nil
}

func scheduledPeerKey(peer *mtproto.InputPeer) string {
	if peer == nil {
		return "nil"
	}
	return peer.GetPredicateName() + ":" + strconv.FormatInt(peer.GetUserId(), 10) + ":" + strconv.FormatInt(peer.GetChatId(), 10) + ":" + strconv.FormatInt(peer.GetChannelId(), 10)
}

func autosaveInputPeer(p *mtproto.InputPeer) *mtproto.Peer {
	if p == nil {
		return nil
	}
	switch {
	case p.GetChannelId() != 0:
		return mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: p.GetChannelId()}).To_Peer()
	case p.GetChatId() != 0:
		return mtproto.MakeTLPeerChat(&mtproto.Peer{ChatId: p.GetChatId()}).To_Peer()
	default:
		return mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: p.GetUserId()}).To_Peer()
	}
}

func (c *ApiFullCore) AccountSaveAutoSaveSettings(in *mtproto.TLAccountSaveAutoSaveSettings) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var settings *mtproto.AutoSaveSettings
	var peer *mtproto.InputPeer
	var users, chats, broadcasts bool
	if in != nil {
		settings = in.GetSettings()
		peer = in.GetPeer()
		users = in.GetUsers()
		chats = in.GetChats()
		broadcasts = in.GetBroadcasts()
	}
	st, err := loadAutoSave(userID)
	if err != nil {
		return nil, err
	}
	if peer != nil {
		key := scheduledPeerKey(peer)
		exc := mtproto.MakeTLAutoSaveException(&mtproto.AutoSaveException{
			Peer:     autosaveInputPeer(peer),
			Settings: settings,
		}).To_AutoSaveException()
		for i := range st.Exceptions {
			if st.Exceptions[i].Key == key {
				st.Exceptions[i].Exc = exc
				if err := saveAutoSave(userID, st); err != nil {
					return nil, err
				}
				return mtproto.BoolTrue, nil
			}
		}
		st.Exceptions = append(st.Exceptions, autoSaveException{Key: key, Exc: exc})
		if err := saveAutoSave(userID, st); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	if users {
		st.Users = settings
	}
	if chats {
		st.Chats = settings
	}
	if broadcasts {
		st.Broadcasts = settings
	}
	if err := saveAutoSave(userID, st); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountDeleteAutoSaveExceptions(in *mtproto.TLAccountDeleteAutoSaveExceptions) (*mtproto.Bool, error) {
	_ = in
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	raw, err := persist.Default.Get(autoSaveKey(userID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return mtproto.BoolTrue, nil
	}
	st, err := loadAutoSave(userID)
	if err != nil {
		return nil, err
	}
	st.Exceptions = nil
	if err := saveAutoSave(userID, st); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
