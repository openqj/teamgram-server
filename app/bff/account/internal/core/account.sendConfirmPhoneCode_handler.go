// Copyright 2022 Teamgram Authors
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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountSendConfirmPhoneCode
// account.sendConfirmPhoneCode#1b3faa88 hash:string settings:CodeSettings = auth.SentCode;
func (c *AccountCore) AccountSendConfirmPhoneCode(in *mtproto.TLAccountSendConfirmPhoneCode) (*mtproto.Auth_SentCode, error) {
	purposeHash := strings.TrimSpace(in.GetHash())
	if purposeHash == "" {
		c.Logger.Errorf("account.sendConfirmPhoneCode - empty hash")
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD.GetUserId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	user, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: c.MD.GetUserId()})
	if err != nil {
		return nil, err
	}
	if user == nil || strings.TrimSpace(user.GetUser().GetPhone()) == "" {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	phoneNumber := strings.TrimSpace(user.GetUser().GetPhone())
	if c.svcCtx.Plugin != nil {
		banned, _ := c.svcCtx.Plugin.CheckPhoneNumberBanned(c.ctx, phoneNumber)
		if banned {
			c.Logger.Errorf("account.sendConfirmPhoneCode - phone banned")
			return nil, mtproto.ErrPhoneNumberBanned
		}
	}

	settings := in.GetSettings()
	codeData, err := c.svcCtx.AuthLogic.DoAuthSendCode(
		c.ctx,
		c.MD.PermAuthKeyId,
		c.MD.SessionId,
		phoneNumber,
		settings.GetAllowFlashcall(),
		settings.GetCurrentNumber(),
		func(codeData2 *model.PhoneCodeTransaction) error {
			codeData2.PhoneCodeExtraData = purposeHash
			return c.issueSMSChallenge(codeData2, challengePurposeConfirmPhone)
		})
	if err != nil {
		c.Logger.Errorf("account.sendConfirmPhoneCode - error: %v", err)
		return nil, err
	}

	// confirmPhone carries only the challenge hash, so index the metadata by hash.
	if codeData.PhoneCodeHash != "" && codeData.PhoneCodeHash != phoneNumber {
		if err = c.svcCtx.Dao.PutCachePhoneCode(c.ctx, c.MD.PermAuthKeyId, codeData.PhoneCodeHash, codeData); err != nil {
			c.Logger.Errorf("account.sendConfirmPhoneCode - index challenge metadata: %v", err)
			return nil, mtproto.ErrInternalServerError
		}
	}

	return codeData.ToAuthSentCode(), nil
}
