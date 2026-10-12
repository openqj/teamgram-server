// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
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

package core

import (
	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountConfirmBotConnection
// account.confirmBotConnection#67ed1f68 bot_id:InputUser = Bool;
func (c *AccountCore) AccountConfirmBotConnection(in *mtproto.TLAccountConfirmBotConnection) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetBotId() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID := confirmBotUserId(in.GetBotId(), c.MD.UserId)
	if botID <= 0 || in.GetBotId().GetPredicateName() == mtproto.Predicate_inputUserEmpty {
		return nil, mtproto.ErrUserIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	bot, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: botID})
	if err != nil {
		return nil, err
	}
	if bot == nil || bot.GetUser() == nil || bot.Deleted() || bot.GetUser().GetId() != botID {
		return nil, mtproto.ErrUserIdInvalid
	}
	if !bot.IsBot() {
		return nil, mtproto.ErrBotInvalid
	}
	if in.GetBotId().GetPredicateName() == mtproto.Predicate_inputUser &&
		(in.GetBotId().GetAccessHash() == 0 || bot.GetUser().GetAccessHash() != in.GetBotId().GetAccessHash()) {
		return nil, mtproto.ErrUserIdInvalid
	}
	var result botConnectionProviderResult
	if err := c.callBotConnectionProvider("account.confirmBotConnection", struct {
		BotID int64              `json:"bot_id"`
		Bot   *mtproto.InputUser `json:"bot"`
	}{BotID: botID, Bot: in.GetBotId()}, &result); err != nil {
		return nil, err
	}
	if result.Bool != nil {
		if result.Bool.GetPredicateName() == mtproto.Predicate_boolTrue || result.Bool.GetConstructor() == mtproto.CRC32_boolTrue {
			return mtproto.BoolTrue, nil
		}
		if result.Bool.GetPredicateName() == mtproto.Predicate_boolFalse || result.Bool.GetConstructor() == mtproto.CRC32_boolFalse {
			return mtproto.BoolFalse, nil
		}
		return nil, mtproto.ErrInternalServerError
	}
	if result.Confirmed {
		return mtproto.BoolTrue, nil
	}
	return nil, mtproto.ErrInternalServerError
}

func confirmBotUserId(bot *mtproto.InputUser, selfUserId int64) int64 {
	if bot == nil || bot.GetPredicateName() == mtproto.Predicate_inputUserEmpty {
		return 0
	}
	if bot.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		return selfUserId
	}
	return bot.GetUserId()
}
