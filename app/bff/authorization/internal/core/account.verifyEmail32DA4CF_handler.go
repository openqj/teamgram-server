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
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AccountVerifyEmail32DA4CF
// account.verifyEmail#32da4cf purpose:EmailVerifyPurpose verification:EmailVerification = account.EmailVerified;
func (c *AuthorizationCore) AccountVerifyEmail32DA4CF(in *mtproto.TLAccountVerifyEmail32DA4CF) (*mtproto.Account_EmailVerified, error) {
	if c == nil || in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return nil, mtproto.ErrEmailVerifyExpired
	}
	purpose := in.GetPurpose()
	verificationInput := in.GetVerification()
	if purpose == nil || verificationInput == nil {
		c.Logger.Errorf("account.verifyEmail - purpose or verification empty")
		return nil, mtproto.ErrEmailVerifyExpired
	}
	switch verificationInput.GetPredicateName() {
	case mtproto.Predicate_emailVerificationGoogle, mtproto.Predicate_emailVerificationApple:
		// No token verifier. Do not treat the token as a stored code.
		c.Logger.Errorf("account.verifyEmail - token unverified")
		return nil, mtproto.ErrAccessTokenInvalid
	case mtproto.Predicate_emailVerificationCode:
	default:
		if verificationInput.GetConstructor() != mtproto.CRC32_emailVerificationCode {
			c.Logger.Errorf("account.verifyEmail - token unverified")
			return nil, mtproto.ErrAccessTokenInvalid
		}
	}

	code := strings.TrimSpace(verificationInput.GetCode())
	codeData, err := c.svcCtx.Challenges.Consume(c.ctx, verification.VerifyRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposeVerifyEmail,
		Scope: c.challengeScope(), ChallengeID: emailPurposeID(purpose), Code: code,
	})
	if err != nil {
		c.Logger.Errorf("account.verifyEmail - error: %v", err)
		return nil, mapEmailChallengeError(err)
	}

	email := codeData.Subject
	if purpose.GetPredicateName() == mtproto.Predicate_emailVerifyPurposeLoginSetup {
		if sent := c.loginSentCode(purpose); sent != nil {
			return mtproto.MakeTLAccountEmailVerifiedLogin(&mtproto.Account_EmailVerified{
				Email:    email,
				SentCode: sent,
			}).To_Account_EmailVerified(), nil
		}
	}
	return mtproto.MakeTLAccountEmailVerified(&mtproto.Account_EmailVerified{
		Email: email,
	}).To_Account_EmailVerified(), nil
}

// loginSentCode is the already-issued phone login code. It is never auth.authorization.
func (c *AuthorizationCore) loginSentCode(purpose *mtproto.EmailVerifyPurpose) *mtproto.Auth_SentCode {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.MD == nil || purpose == nil {
		return nil
	}
	phone := strings.TrimSpace(purpose.GetPhoneNumber())
	hash := purpose.GetPhoneCodeHash()
	if phone == "" || hash == "" || validEmail(phone) {
		return nil
	}
	phoneCode, err := c.svcCtx.Dao.GetPhoneCode(c.ctx, c.MD.PermAuthKeyId, phone, hash)
	if err != nil || phoneCode == nil {
		return nil
	}
	switch phoneCode.SentCodeType {
	case model.SentCodeTypeApp, model.SentCodeTypeSms, model.SentCodeTypeCall, model.SentCodeTypeFlashCall:
		return phoneCode.ToAuthSentCode()
	default:
		return nil
	}
}
