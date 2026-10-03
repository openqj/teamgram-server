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
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AccountSendVerifyEmailCode
// account.sendVerifyEmailCode#98e037bb purpose:EmailVerifyPurpose email:string = account.SentEmailCode;
func (c *AuthorizationCore) AccountSendVerifyEmailCode(in *mtproto.TLAccountSendVerifyEmailCode) (*mtproto.Account_SentEmailCode, error) {
	if c == nil || in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return nil, mtproto.ErrSendCodeUnavailable
	}
	email := strings.TrimSpace(in.GetEmail())
	if !validEmail(email) {
		c.Logger.Errorf("account.sendVerifyEmailCode - invalid email")
		return nil, mtproto.ErrEmailInvalid
	}

	purpose := in.GetPurpose()
	if purpose == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	switch purpose.GetPredicateName() {
	case mtproto.Predicate_emailVerifyPurposeLoginSetup,
		mtproto.Predicate_emailVerifyPurposeLoginChange,
		mtproto.Predicate_emailVerifyPurposePassport:
	default:
		return nil, mtproto.ErrInputRequestInvalid
	}

	purposeID := emailPurposeID(purpose)
	issued, err := c.svcCtx.Challenges.Issue(c.ctx, verification.IssueRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposeVerifyEmail,
		Subject: email, Scope: c.challengeScope(), ChallengeID: purposeID,
		CodeLength: 6,
	})
	if err != nil {
		return nil, mapEmailChallengeError(err)
	}
	return mtproto.MakeTLAccountSentEmailCode(&mtproto.Account_SentEmailCode{
		EmailPattern: maskEmail(email),
		Length:       int32(len(issued.Code)),
	}).To_Account_SentEmailCode(), nil
}
