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
	"errors"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AuthRecoverPassword
// auth.recoverPassword#37096c70 flags:# code:string new_settings:flags.0?account.PasswordInputSettings = auth.Authorization;
func (c *AuthorizationCore) AuthRecoverPassword(in *mtproto.TLAuthRecoverPassword) (*mtproto.Auth_Authorization, error) {
	if c == nil || in == nil {
		if c == nil {
			return nil, mtproto.ErrAuthKeyUnregistered
		}
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD == nil || c.MD.GetUserId() == 0 {
		c.Logger.Errorf("auth.recoverPassword - user not bound")
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		c.Logger.Errorf("auth.recoverPassword - challenge provider unavailable")
		return nil, mtproto.ErrPasswordRecoveryExpired
	}

	st, err := loadAcctPasswordState(c.MD.GetUserId())
	if err != nil {
		c.Logger.Errorf("auth.recoverPassword - error: %v", err)
		return nil, err
	}
	if st.recoveryEmail() == "" {
		c.Logger.Errorf("auth.recoverPassword - no recovery email")
		return nil, mtproto.ErrPasswordRecoveryNa
	}
	if err = st.checkRecoveryCode(in.GetCode()); err != nil {
		c.Logger.Errorf("auth.recoverPassword - code: %v", err)
		return nil, err
	}
	_, err = c.svcCtx.Challenges.Consume(c.ctx, verification.VerifyRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposePasswordRecovery,
		Scope:       verification.ScopeID(c.MD.GetUserId()),
		ChallengeID: verification.PurposeID(challengePurposePasswordRecovery, verification.ScopeID(c.MD.GetUserId())),
		Code:        in.GetCode(),
	})
	if err != nil {
		if errors.Is(err, verification.ErrChallengeInvalid) {
			return nil, mtproto.ErrCodeInvalid
		}
		if errors.Is(err, verification.ErrChallengeNotFound) || errors.Is(err, verification.ErrChallengeExpired) {
			return nil, mtproto.ErrPasswordRecoveryExpired
		}
		return nil, mtproto.ErrInternalServerError
	}
	if err = applyRecoveredPassword(&st, in.GetNewSettings()); err != nil {
		c.Logger.Errorf("auth.recoverPassword - settings: %v", err)
		return nil, err
	}
	if err = saveAcctPasswordState(c.MD.GetUserId(), st); err != nil {
		c.Logger.Errorf("auth.recoverPassword - save: %v", err)
		return nil, err
	}

	user, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: c.MD.GetUserId(),
	})
	if err != nil {
		c.Logger.Errorf("auth.recoverPassword - user: %v", err)
		return nil, err
	}
	if user == nil {
		c.Logger.Errorf("auth.recoverPassword - user is nil")
		return nil, mtproto.ErrInternalServerError
	}

	return authAuthorization(user), nil
}
