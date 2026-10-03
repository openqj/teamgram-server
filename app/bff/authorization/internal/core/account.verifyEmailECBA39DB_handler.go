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

// AccountVerifyEmailECBA39DB
// account.verifyEmail#ecba39db email:string code:string = Bool;
func (c *AuthorizationCore) AccountVerifyEmailECBA39DB(in *mtproto.TLAccountVerifyEmailECBA39DB) (*mtproto.Bool, error) {
	if c == nil || in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return nil, mtproto.ErrEmailVerifyExpired
	}
	email := strings.TrimSpace(in.GetEmail())
	code := strings.TrimSpace(in.GetCode())
	if !validEmail(email) {
		return nil, mtproto.ErrEmailInvalid
	}
	if code == "" {
		return nil, mtproto.ErrCodeEmpty
	}
	request := verification.VerifyRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposeVerifyEmail,
		Scope:       c.challengeScope(),
		ChallengeID: verification.PurposeID(mtproto.Predicate_emailVerifyPurposePassport, "", ""),
		Code:        code,
	}
	challenge, err := c.svcCtx.Challenges.Check(c.ctx, request)
	if err != nil {
		c.Logger.Errorf("account.verifyEmail - error: %v", err)
		return nil, mapEmailChallengeError(err)
	}
	if challenge == nil || !strings.EqualFold(strings.TrimSpace(challenge.Subject), email) {
		return nil, mtproto.ErrCodeInvalid
	}
	if _, err = c.svcCtx.Challenges.Consume(c.ctx, request); err != nil {
		c.Logger.Errorf("account.verifyEmail - consume error: %v", err)
		return nil, mapEmailChallengeError(err)
	}
	return mtproto.BoolTrue, nil
}
