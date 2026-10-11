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

import "github.com/teamgram/proto/mtproto"

// RPCBotVerificationIconsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) BotsSetCustomVerification(in *mtproto.TLBotsSetCustomVerification) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var result struct {
		Value bool `json:"value"`
	}
	if err := c.callBusinessProvider("bots.setCustomVerification", map[string]any{
		"enabled": in.GetEnabled(), "bot": in.GetBot(), "peer": in.GetPeer(), "custom_description": in.GetCustomDescription(),
	}, &result); err != nil {
		return nil, err
	}
	if result.Value {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (c *ApiFullCore) BotsGetBotRecommendations(in *mtproto.TLBotsGetBotRecommendations) (*mtproto.Users_Users, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.verifyMiniBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	result := new(mtproto.Users_Users)
	if err = c.callBusinessProvider("bots.getBotRecommendations", map[string]any{
		"caller_user_id": uid, "bot_id": botID,
	}, result); err != nil {
		return nil, err
	}
	if result == nil || len(result.GetUsers()) > 10000 {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}
