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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCBotMenuButtonServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) BotsSetBotMenuButton(in *mtproto.TLBotsSetBotMenuButton) (*mtproto.Bool, error) {
	ownerId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var target int64
	var button *mtproto.BotMenuButton
	if in != nil {
		if u := in.GetUserId(); u != nil {
			target = u.UserId
		}
		button = in.GetButton()
	}
	raw, err := json.Marshal(button)
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(botMenuButtonPersistKey(ownerId, target), string(raw)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsGetBotMenuButton(in *mtproto.TLBotsGetBotMenuButton) (*mtproto.BotMenuButton, error) {
	ownerId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var target int64
	if in != nil {
		if u := in.GetUserId(); u != nil {
			target = u.UserId
		}
	}
	raw, err := persist.Default.Get(botMenuButtonPersistKey(ownerId, target))
	if err != nil {
		return nil, err
	}
	if raw == "" || raw == "null" {
		return mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{}).To_BotMenuButton(), nil
	}
	button := &mtproto.BotMenuButton{}
	if err := json.Unmarshal([]byte(raw), button); err != nil {
		return nil, err
	}
	return button, nil
}

func botMenuButtonPersistKey(ownerId, userId int64) string {
	return fmt.Sprintf("botmenu:%d:%d", ownerId, userId)
}
