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

// RPCTakeoutServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const takeoutKeyPrefix = "takeout:"

type takeoutSession struct {
	ID                int64 `json:"id"`
	Contacts          bool  `json:"contacts,omitempty"`
	MessageUsers      bool  `json:"message_users,omitempty"`
	MessageChats      bool  `json:"message_chats,omitempty"`
	MessageMegagroups bool  `json:"message_megagroups,omitempty"`
	MessageChannels   bool  `json:"message_channels,omitempty"`
	Files             bool  `json:"files,omitempty"`
	FileMaxSize       int64 `json:"file_max_size,omitempty"`
}

func takeoutUserKey(userID int64) string {
	return takeoutKeyPrefix + strconv.FormatInt(userID, 10)
}

func allocTakeoutID() (int64, error) {
	key := takeoutKeyPrefix + "next"
	raw, err := persist.Default.Get(key)
	if err != nil {
		return 0, err
	}
	var n int64
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			return 0, err
		}
	}
	n++
	buf, err := json.Marshal(n)
	if err != nil {
		return 0, err
	}
	if err := persist.Default.Set(key, string(buf)); err != nil {
		return 0, err
	}
	return n, nil
}

func (c *ApiFullCore) AccountInitTakeoutSession(in *mtproto.TLAccountInitTakeoutSession) (*mtproto.Account_Takeout, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	raw, err := persist.Default.Get(takeoutUserKey(userID))
	if err != nil {
		return nil, err
	}
	var sess takeoutSession
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &sess); err != nil {
			return nil, err
		}
	}
	if sess.ID == 0 {
		sess.ID, err = allocTakeoutID()
		if err != nil {
			return nil, err
		}
	}
	if in != nil {
		sess.Contacts = in.GetContacts()
		sess.MessageUsers = in.GetMessageUsers()
		sess.MessageChats = in.GetMessageChats()
		sess.MessageMegagroups = in.GetMessageMegagroups()
		sess.MessageChannels = in.GetMessageChannels()
		sess.Files = in.GetFiles()
		if v := in.GetFileMaxSize_FLAGINT64(); v != nil {
			sess.FileMaxSize = v.GetValue()
		} else if v := in.GetFileMaxSize_FLAGINT32(); v != nil {
			sess.FileMaxSize = int64(v.GetValue())
		}
	}
	buf, err := json.Marshal(sess)
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(takeoutUserKey(userID), string(buf)); err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountTakeout(&mtproto.Account_Takeout{Id: sess.ID}).To_Account_Takeout(), nil
}

func (c *ApiFullCore) AccountFinishTakeoutSession(in *mtproto.TLAccountFinishTakeoutSession) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	raw, err := persist.Default.Get(takeoutUserKey(userID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, mtproto.ErrTakeoutRequired
	}
	var sess takeoutSession
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, err
	}
	if sess.ID == 0 {
		return nil, mtproto.ErrTakeoutRequired
	}
	if err := persist.Default.Set(takeoutUserKey(userID), ""); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetSplitRanges(in *mtproto.TLMessagesGetSplitRanges) (*mtproto.Vector_MessageRange, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	// APIFull has no authoritative export-range index. Returning an empty
	// vector would make callers skip messages, so fail closed until the
	// message service exposes the complete range query.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) ChannelsGetLeftChannels(in *mtproto.TLChannelsGetLeftChannels) (*mtproto.Messages_Chats, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	// APIFull has no authoritative left-channel membership/history store.
	// Returning an empty chat list would make callers treat unknown history as
	// complete, so fail closed until a provider exposes the full query.
	return nil, mtproto.ErrMethodNotImpl
}
