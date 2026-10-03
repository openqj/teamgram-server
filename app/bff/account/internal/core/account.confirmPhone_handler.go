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
	"github.com/teamgram/proto/mtproto"
	"strings"
)

// AccountConfirmPhone
// account.confirmPhone#5f2178c3 phone_code_hash:string phone_code:string = Bool;
func (c *AccountCore) AccountConfirmPhone(in *mtproto.TLAccountConfirmPhone) (*mtproto.Bool, error) {
	phoneCodeHash := strings.TrimSpace(in.GetPhoneCodeHash())
	phoneCode := strings.TrimSpace(in.GetPhoneCode())
	if phoneCodeHash == "" {
		c.Logger.Errorf("account.confirmPhone - empty phone_code_hash")
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}
	if phoneCode == "" {
		c.Logger.Errorf("account.confirmPhone - empty phone_code")
		return nil, mtproto.ErrPhoneCodeEmpty
	}

	codeData, err := c.svcCtx.Dao.GetCachePhoneCode(c.ctx, c.MD.PermAuthKeyId, phoneCodeHash)
	if err != nil || codeData == nil {
		c.Logger.Errorf("account.confirmPhone - code not found")
		return nil, mtproto.ErrPhoneCodeExpired
	}
	if codeData.PhoneCodeHash != phoneCodeHash {
		c.Logger.Errorf("account.confirmPhone - phone_code_hash mismatch")
		return nil, mtproto.ErrPhoneCodeInvalid
	}
	if err = c.consumeSMSChallenge(phoneCodeHash, phoneCode, challengePurposeConfirmPhone); err != nil {
		return nil, err
	}

	if err = c.svcCtx.Dao.DeleteCachePhoneCode(c.ctx, c.MD.PermAuthKeyId, phoneCodeHash); err != nil {
		c.Logger.Errorf("account.confirmPhone - delete phone code cache: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	if codeData.PhoneNumber != "" && codeData.PhoneNumber != phoneCodeHash {
		if err = c.svcCtx.Dao.DeletePhoneCode(c.ctx, c.MD.PermAuthKeyId, codeData.PhoneNumber, codeData.PhoneCodeHash); err != nil {
			c.Logger.Errorf("account.confirmPhone - delete phone code index: %v", err)
			return nil, mtproto.ErrInternalServerError
		}
	}
	return mtproto.BoolTrue, nil
}
