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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AuthResetLoginEmail
// auth.resetLoginEmail#7e960193 phone_number:string phone_code_hash:string = auth.SentCode;
func (c *AuthorizationCore) AuthResetLoginEmail(in *mtproto.TLAuthResetLoginEmail) (*mtproto.Auth_SentCode, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, phoneNumber, err := checkPhoneNumberInvalid(in.GetPhoneNumber())
	if err != nil {
		c.Logger.Errorf("auth.resetLoginEmail - phone: %v", err)
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	if in.GetPhoneCodeHash() == "" {
		c.Logger.Errorf("auth.resetLoginEmail - empty phone_code_hash")
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Challenges == nil || c.MD == nil {
		if c != nil {
			c.Logger.Errorf("auth.resetLoginEmail - SMS provider unavailable")
		}
		return nil, mtproto.ErrSmsCodeCreateFailed
	}

	codeData, err := c.svcCtx.Dao.GetPhoneCode(c.ctx, c.MD.PermAuthKeyId, phoneNumber, in.GetPhoneCodeHash())
	if err != nil {
		c.Logger.Errorf("auth.resetLoginEmail - code: %v", err)
		return nil, err
	}
	if codeData == nil || codeData.PhoneCodeHash == "" {
		c.Logger.Errorf("auth.resetLoginEmail - empty phone code challenge")
		return nil, mtproto.ErrPhoneCodeEmpty
	}
	if codeData.PhoneCodeExpired <= 0 || time.Now().Unix() > int64(codeData.PhoneCodeExpired) {
		c.Logger.Errorf("auth.resetLoginEmail - phone code expired")
		return nil, mtproto.ErrPhoneCodeExpired
	}
	if codeData.State != model.CodeStateSent && codeData.State != model.CodeStateSignIn {
		c.Logger.Errorf("auth.resetLoginEmail - invalid phone code state: %d", codeData.State)
		return nil, mtproto.ErrPhoneCodeInvalid
	}
	if _, err = c.issuePhoneChallenge(codeData, verification.ChannelSMS, challengePurposeAuthLogin, ""); err != nil {
		c.Logger.Errorf("auth.resetLoginEmail - delivery: %v", err)
		return nil, err
	}

	codeData.SentCodeType = model.SentCodeTypeSms
	codeData.NextCodeType = model.CodeTypeNone
	codeData.State = model.CodeStateSent
	if err = c.svcCtx.Dao.UpdatePhoneCodeData(c.ctx, c.MD.PermAuthKeyId, phoneNumber, in.GetPhoneCodeHash(), codeData); err != nil {
		c.Logger.Errorf("auth.resetLoginEmail - update: %v", err)
		return nil, err
	}

	return codeData.ToAuthSentCode(), nil
}
