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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

// RPCBotMenuButtonServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) BotsSetBotMenuButton(in *mtproto.TLBotsSetBotMenuButton) (*mtproto.Bool, error) {
	ownerId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	var target int64
	if u := in.GetUserId(); u != nil {
		target = u.UserId
	}
	if target <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	button := in.GetButton()
	raw, err := json.Marshal(button)
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if err = domain.SetBotMenuButton(ownerId, target, raw); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsGetBotMenuButton(in *mtproto.TLBotsGetBotMenuButton) (*mtproto.BotMenuButton, error) {
	ownerId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	var target int64
	if u := in.GetUserId(); u != nil {
		target = u.UserId
	}
	if target <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	raw, found, err := domain.GetBotMenuButton(ownerId, target)
	if err != nil {
		return nil, err
	}
	if !found || len(raw) == 0 || string(raw) == "null" {
		return mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{}).To_BotMenuButton(), nil
	}
	button := &mtproto.BotMenuButton{}
	if err := json.Unmarshal(raw, button); err != nil {
		return nil, err
	}
	return button, nil
}
