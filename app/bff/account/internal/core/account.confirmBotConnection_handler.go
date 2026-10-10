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

import "github.com/teamgram/proto/mtproto"

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
	// Telegram confirmation requires an authoritative bot-connection verifier
	// and a durable confirmation record. No such provider is configured here;
	// fail closed instead of minting a local token that clients could mistake
	// for a confirmed connection.
	return nil, mtproto.ErrMethodNotImpl
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
