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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
)

// AccountConfirmBotConnection
// account.confirmBotConnection#67ed1f68 bot_id:InputUser = Bool;
func (c *AccountCore) AccountConfirmBotConnection(in *mtproto.TLAccountConfirmBotConnection) (*mtproto.Bool, error) {
	botId := confirmBotUserId(in.GetBotId(), c.MD.UserId)
	if botId == 0 {
		// No bot-connection verifier in this build. Empty query is not stored.
		return mtproto.BoolFalse, nil
	}

	token := crypto.GenerateStringNonce(16)
	key := fmt.Sprintf("bot_connection_%d", botId)
	codeData := &model.PhoneCodeTransaction{
		AuthKeyId:     c.MD.PermAuthKeyId,
		SessionId:     c.MD.SessionId,
		PhoneNumber:   key,
		PhoneCode:     token,
		PhoneCodeHash: token,
		State:         model.CodeStateOk,
	}
	if err := c.svcCtx.Dao.PutCachePhoneCode(c.ctx, c.MD.PermAuthKeyId, key, codeData); err != nil {
		c.Logger.Errorf("account.confirmBotConnection - store token error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
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
