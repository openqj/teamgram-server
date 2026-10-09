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
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
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

// requireTakeoutSession validates the invokeWithTakeout metadata against the
// caller-owned active session. The wrapper ID is untrusted input; accepting it
// without checking the persisted record would let a finished or fabricated
// takeout request reach export handlers.
func (c *ApiFullCore) requireTakeoutSession(userID int64) (*takeoutSession, error) {
	if c == nil || c.MD == nil || c.MD.Takeout == nil || c.MD.Takeout.Id <= 0 {
		return nil, mtproto.ErrTakeoutRequired
	}
	raw, err := persist.LoadTakeout(userID)
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
	if sess.ID == 0 || sess.ID != c.MD.Takeout.Id {
		return nil, mtproto.ErrTakeoutRequired
	}
	return &sess, nil
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
	var sess takeoutSession
	var sessionID int64
	_, _, err = persist.MutateTakeout(userID, func(id int64, raw string) (string, error) {
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &sess); err != nil {
				return "", err
			}
		}
		if sess.ID == 0 {
			sess.ID = id
			if sess.ID == 0 {
				sess.ID, err = allocTakeoutID()
				if err != nil {
					return "", err
				}
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
		sessionID = sess.ID
		buf, err := json.Marshal(sess)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountTakeout(&mtproto.Account_Takeout{Id: sessionID}).To_Account_Takeout(), nil
}

func (c *ApiFullCore) AccountFinishTakeoutSession(in *mtproto.TLAccountFinishTakeoutSession) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	consumed, err := persist.ConsumeTakeout(userID, func(raw string) error {
		var sess takeoutSession
		if err := json.Unmarshal([]byte(raw), &sess); err != nil {
			return err
		}
		if sess.ID == 0 {
			return mtproto.ErrTakeoutRequired
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, mtproto.ErrTakeoutRequired
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetSplitRanges(in *mtproto.TLMessagesGetSplitRanges) (*mtproto.Vector_MessageRange, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if _, err := c.requireTakeoutSession(userID); err != nil {
		return nil, err
	}
	_ = in
	ranges, err := persist.GetMessageSplitRanges(userID)
	if errors.Is(err, persist.ErrMessageProviderUnavailable) {
		// The in-memory Store is used by unit tests and cannot prove that an
		// empty result means the account has no exportable messages.
		return nil, mtproto.ErrMethodNotImpl
	}
	if err != nil {
		return nil, err
	}
	result := make([]*mtproto.MessageRange, 0, len(ranges))
	for _, item := range ranges {
		result = append(result, &mtproto.MessageRange{MinId: item.MinID, MaxId: item.MaxID})
	}
	return &mtproto.Vector_MessageRange{Datas: result}, nil
}

func (c *ApiFullCore) ChannelsGetLeftChannels(in *mtproto.TLChannelsGetLeftChannels) (*mtproto.Messages_Chats, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if _, err := c.requireTakeoutSession(userID); err != nil {
		return nil, err
	}
	offset := int32(0)
	if in != nil {
		offset = in.GetOffset()
	}
	if offset < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	left, err := domain.ListLeftChannels(userID, offset, 100)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	chats := make([]*mtproto.Chat, 0, len(left))
	for _, channel := range left {
		chats = append(chats, channelview.Chat(channel, false))
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}
