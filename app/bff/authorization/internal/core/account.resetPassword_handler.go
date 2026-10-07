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
	"encoding/json"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

const passwordResetWait = 7 * 24 * 60 * 60

func ensurePasswordResetWait(st *acctPasswordState, now int64) bool {
	changed := false
	if st.ResetRequestedAt == 0 {
		st.ResetRequestedAt = now
		changed = true
	}
	if st.ResetUntilDate == 0 {
		st.ResetUntilDate = st.ResetRequestedAt + passwordResetWait
		changed = true
	}
	if st.ResetDeclined && st.ResetRetryDate == 0 {
		st.ResetRetryDate = now + passwordResetWait
		changed = true
	}
	return changed
}

// AccountResetPassword
// account.resetPassword#9308ce1b = account.ResetPasswordResult;
func (c *AuthorizationCore) AccountResetPassword(in *mtproto.TLAccountResetPassword) (*mtproto.Account_ResetPasswordResult, error) {
	_ = in
	if c.MD.GetUserId() == 0 {
		c.Logger.Errorf("account.resetPassword - user not bound")
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	userID := c.MD.GetUserId()
	for attempt := 0; attempt < 4; attempt++ {
		raw, err := persist.Default.Get(acctPasswordKey(userID))
		if err != nil {
			c.Logger.Errorf("account.resetPassword - error: %v", err)
			return nil, err
		}
		var st acctPasswordState
		if raw != "" {
			if err = json.Unmarshal([]byte(raw), &st); err != nil {
				return nil, err
			}
		}
		if !st.HasPassword {
			return mtproto.MakeTLAccountResetPasswordOk(nil).To_Account_ResetPasswordResult(), nil
		}
		if ensurePasswordResetWait(&st, time.Now().Unix()) {
			if swapped, saveErr := compareAndSaveAcctPasswordState(userID, raw, st); saveErr != nil {
				c.Logger.Errorf("account.resetPassword - save wait state: %v", saveErr)
				return nil, saveErr
			} else if !swapped {
				continue
			}
		}
		if st.ResetDeclined {
			return mtproto.MakeTLAccountResetPasswordFailedWait(&mtproto.Account_ResetPasswordResult{
				RetryDate: int32(st.ResetRetryDate),
			}).To_Account_ResetPasswordResult(), nil
		}
		return mtproto.MakeTLAccountResetPasswordRequestedWait(&mtproto.Account_ResetPasswordResult{
			UntilDate: int32(st.ResetUntilDate),
		}).To_Account_ResetPasswordResult(), nil
	}
	return nil, mtproto.ErrInternalServerError
}
