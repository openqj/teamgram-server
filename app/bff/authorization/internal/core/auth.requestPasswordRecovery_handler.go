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
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AuthRequestPasswordRecovery
// auth.requestPasswordRecovery#d897bc66 = auth.PasswordRecovery;
func (c *AuthorizationCore) AuthRequestPasswordRecovery(in *mtproto.TLAuthRequestPasswordRecovery) (*mtproto.Auth_PasswordRecovery, error) {
	_ = in
	if c == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.MD == nil || c.MD.GetUserId() == 0 {
		c.Logger.Errorf("auth.requestPasswordRecovery - user not bound")
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		c.Logger.Errorf("auth.requestPasswordRecovery - challenge provider unavailable")
		return nil, mtproto.ErrSendCodeUnavailable
	}

	st, err := loadAcctPasswordState(c.MD.GetUserId())
	if err != nil {
		c.Logger.Errorf("auth.requestPasswordRecovery - error: %v", err)
		return nil, err
	}
	changed, err := rejectUndeliveredPasswordRecovery(&st)
	if changed {
		if saveErr := saveAcctPasswordState(c.MD.GetUserId(), st); saveErr != nil {
			c.Logger.Errorf("auth.requestPasswordRecovery - clear legacy recovery code: %v", saveErr)
			return nil, saveErr
		}
	}
	if err != nil {
		c.Logger.Errorf("auth.requestPasswordRecovery - unavailable: %v", err)
		return nil, err
	}
	email := st.recoveryEmail()
	issued, err := c.svcCtx.Challenges.Issue(c.ctx, verification.IssueRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposePasswordRecovery,
		Subject: email, Scope: verification.ScopeID(c.MD.GetUserId()),
		ChallengeID: verification.PurposeID(challengePurposePasswordRecovery, verification.ScopeID(c.MD.GetUserId())),
		CodeLength:  6,
	})
	_ = issued
	if err != nil {
		c.Logger.Errorf("auth.requestPasswordRecovery - delivery: %v", err)
		return nil, mapEmailChallengeError(err)
	}
	return mtproto.MakeTLAuthPasswordRecovery(&mtproto.Auth_PasswordRecovery{
		EmailPattern: maskEmail(email),
	}).To_Auth_PasswordRecovery(), nil
}
