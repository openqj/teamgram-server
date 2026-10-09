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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCBotAdminRightServer: defaults are scoped to the authenticated bot and
// persisted in PostgreSQL. Telegram does not accept a target bot argument for
// either method; the caller's bot account is the owner of the settings.

func (c *ApiFullCore) BotsSetBotBroadcastDefaultAdminRights(in *mtproto.TLBotsSetBotBroadcastDefaultAdminRights) (*mtproto.Bool, error) {
	return c.setBotDefaultAdminRights(in != nil, in.GetAdminRights(), false)
}

func (c *ApiFullCore) BotsSetBotGroupDefaultAdminRights(in *mtproto.TLBotsSetBotGroupDefaultAdminRights) (*mtproto.Bool, error) {
	return c.setBotDefaultAdminRights(in != nil, in.GetAdminRights(), true)
}

func (c *ApiFullCore) setBotDefaultAdminRights(present bool, rights *mtproto.ChatAdminRights, group bool) (*mtproto.Bool, error) {
	botID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !present || rights == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if !validChatAdminRights(rights) {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: botID})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil || profile.Deleted() {
		return nil, mtproto.ErrBotInvalid
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	raw, err := json.Marshal(rights)
	if err != nil {
		return nil, err
	}
	if err = domain.SetBotDefaultAdminRights(botID, group, raw); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func validChatAdminRights(rights *mtproto.ChatAdminRights) bool {
	if rights == nil {
		return false
	}
	predicate := rights.GetPredicateName()
	constructor := rights.GetConstructor()
	if predicate != "" && predicate != mtproto.Predicate_chatAdminRights {
		return false
	}
	if constructor != mtproto.TLConstructor_CRC32_UNKNOWN && constructor != mtproto.TLConstructor_CRC32_chatAdminRights {
		return false
	}
	return predicate == mtproto.Predicate_chatAdminRights || constructor == mtproto.TLConstructor_CRC32_chatAdminRights
}
