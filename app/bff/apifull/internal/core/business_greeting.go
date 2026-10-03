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

// RPCBusinessGreetingServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func greetKey(userId int64) string {
	return "greet:" + strconv.FormatInt(userId, 10)
}

func (c *ApiFullCore) AccountUpdateBusinessGreetingMessage(in *mtproto.TLAccountUpdateBusinessGreetingMessage) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	key := greetKey(c.MD.UserId)
	if in == nil || in.GetMessage() == nil {
		if err := persist.Default.Set(key, ""); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	msg := in.GetMessage()
	blob, err := json.Marshal(&mtproto.InputBusinessGreetingMessage{
		PredicateName:  msg.GetPredicateName(),
		ShortcutId:     msg.GetShortcutId(),
		NoActivityDays: msg.GetNoActivityDays(),
		Recipients:     msg.GetRecipients(),
	})
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(key, string(blob)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func businessGreetingMessage(userId int64) *mtproto.InputBusinessGreetingMessage {
	raw, err := persist.Default.Get(greetKey(userId))
	if err != nil || raw == "" {
		return nil
	}
	var msg mtproto.InputBusinessGreetingMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil
	}
	return &msg
}

func awayKey(userId int64) string {
	return "greet:away:" + strconv.FormatInt(userId, 10)
}

func (c *ApiFullCore) AccountUpdateBusinessAwayMessage(in *mtproto.TLAccountUpdateBusinessAwayMessage) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	key := awayKey(c.MD.UserId)
	if in == nil || in.GetMessage() == nil {
		if err := persist.Default.Set(key, ""); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	msg := in.GetMessage()
	blob, err := json.Marshal(&mtproto.InputBusinessAwayMessage{
		OfflineOnly: msg.GetOfflineOnly(),
		ShortcutId:  msg.GetShortcutId(),
		Schedule:    msg.GetSchedule(),
		Recipients:  msg.GetRecipients(),
	})
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(key, string(blob)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
