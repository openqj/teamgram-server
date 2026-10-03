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
	"github.com/teamgram/teamgram-server/app/bff/apifull/layer229"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AuthRequestFirebaseSms
// auth.requestFirebaseSms#8e39261e flags:# phone_number:string phone_code_hash:string safety_net_token:flags.0?string play_integrity_token:flags.2?string ios_push_secret:flags.1?string = Bool;
func (c *AuthorizationCore) AuthRequestFirebaseSms(in *mtproto.TLAuthRequestFirebaseSms) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, phoneNumber, err := checkPhoneNumberInvalid(in.GetPhoneNumber())
	if err != nil {
		c.Logger.Errorf("auth.requestFirebaseSms - phone: %v", err)
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	if in.GetPhoneCodeHash() == "" {
		c.Logger.Errorf("auth.requestFirebaseSms - empty phone_code_hash")
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}

	token := ""
	if in.GetPlayIntegrityToken().GetValue() != "" {
		token = in.GetPlayIntegrityToken().GetValue()
	} else if in.GetSafetyNetToken().GetValue() != "" {
		token = in.GetSafetyNetToken().GetValue()
	} else if in.GetIosPushSecret().GetValue() != "" {
		token = in.GetIosPushSecret().GetValue()
	}
	if token == "" {
		c.Logger.Errorf("auth.requestFirebaseSms - empty firebase token")
		return nil, mtproto.ErrAccessTokenInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Challenges == nil || c.MD == nil {
		c.Logger.Errorf("auth.requestFirebaseSms - phone-code store unavailable")
		return nil, mtproto.ErrMethodNotImpl
	}

	codeData, err := c.svcCtx.Dao.GetPhoneCode(c.ctx, c.MD.PermAuthKeyId, phoneNumber, in.GetPhoneCodeHash())
	if err != nil {
		c.Logger.Errorf("auth.requestFirebaseSms - code: %v", err)
		return nil, err
	}
	if codeData == nil || codeData.PhoneCodeHash == "" {
		c.Logger.Errorf("auth.requestFirebaseSms - empty phone code")
		return nil, mtproto.ErrPhoneCodeEmpty
	}
	if codeData.PhoneCodeExpired <= 0 || time.Now().Unix() >= int64(codeData.PhoneCodeExpired) {
		c.Logger.Errorf("auth.requestFirebaseSms - phone code expired")
		return nil, mtproto.ErrPhoneCodeExpired
	}
	if codeData.State != model.CodeStateSent && codeData.State != model.CodeStateSignIn {
		c.Logger.Errorf("auth.requestFirebaseSms - invalid phone code state: %d", codeData.State)
		return nil, mtproto.ErrPhoneCodeInvalid
	}

	_, tokenPhone, err := layer229.VerifyFirebaseToken(c.ctx, token)
	if err != nil {
		c.Logger.Errorf("auth.requestFirebaseSms - firebase: %v", err)
		return nil, mtproto.ErrAccessTokenInvalid
	}
	if digitsOnly(tokenPhone) == "" || digitsOnly(tokenPhone) != phoneNumber {
		c.Logger.Errorf("auth.requestFirebaseSms - token phone mismatch")
		return nil, mtproto.ErrPhoneNumberInvalid
	}

	if _, err = c.issuePhoneChallenge(codeData, verification.ChannelSMS, challengePurposeAuthLogin, ""); err != nil {
		c.Logger.Errorf("auth.requestFirebaseSms - delivery: %v", err)
		return nil, err
	}
	codeData.SentCodeType = model.SentCodeTypeSms
	codeData.NextCodeType = model.CodeTypeNone
	codeData.State = model.CodeStateSent
	if err = c.svcCtx.Dao.UpdatePhoneCodeData(c.ctx, c.MD.PermAuthKeyId, phoneNumber, in.GetPhoneCodeHash(), codeData); err != nil {
		_ = c.svcCtx.Challenges.Revoke(c.ctx, verification.VerifyRequest{
			Channel: verification.ChannelSMS, Purpose: challengePurposeAuthLogin,
			Scope: c.challengeScope(), ChallengeID: in.GetPhoneCodeHash(),
		})
		c.Logger.Errorf("auth.requestFirebaseSms - update: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
