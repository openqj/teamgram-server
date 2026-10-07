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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/pkg/twofa"
)

type acctPasswordState twofa.PasswordState

func acctPasswordKey(userID int64) string {
	return twofa.PasswordKey(userID)
}

func loadAcctPasswordState(userID int64) (acctPasswordState, error) {
	state, err := twofa.LoadPasswordState(persist.Default, userID)
	return acctPasswordState(state), err
}

func saveAcctPasswordState(userID int64, st acctPasswordState) error {
	b, err := json.Marshal(twofa.PasswordState(st))
	if err != nil {
		return err
	}
	return persist.Default.Set(acctPasswordKey(userID), string(b))
}

func compareAndSaveAcctPasswordState(userID int64, expected string, st acctPasswordState) (bool, error) {
	b, err := json.Marshal(twofa.PasswordState(st))
	if err != nil {
		return false, err
	}
	return persist.CompareAndSwap(acctPasswordKey(userID), expected, string(b))
}

// recoveryEmail is the confirmed recovery address. A pending address is not set.
func (st acctPasswordState) recoveryEmail() string {
	if st.Email == "" || st.EmailPending {
		return ""
	}
	return st.Email
}

// rejectUndeliveredPasswordRecovery invalidates legacy locally generated codes.
func rejectUndeliveredPasswordRecovery(st *acctPasswordState) (bool, error) {
	changed := st.RecoveryCode != "" || st.RecoveryExpire != 0
	st.RecoveryCode = ""
	st.RecoveryExpire = 0
	if st.recoveryEmail() == "" {
		return changed, mtproto.ErrPasswordRecoveryNa
	}
	return changed, nil
}

func (st acctPasswordState) checkRecoveryCode(value string) error {
	if st.recoveryEmail() == "" {
		return mtproto.ErrPasswordRecoveryNa
	}
	if strings.TrimSpace(value) == "" {
		return mtproto.ErrCodeEmpty
	}
	return nil
}

// applyRecoveredPassword stores the same verifier returned by the client as password updates.
func applyRecoveredPassword(st *acctPasswordState, ns *mtproto.Account_PasswordInputSettings) error {
	if ns == nil {
		st.HasPassword = false
		st.Secret = nil
		st.Algo = nil
	} else {
		if ns.GetNewAlgo() == nil {
			return mtproto.ErrPasswordHashInvalid
		}
		if ns.GetNewAlgo().GetPredicateName() == mtproto.Predicate_passwordKdfAlgoUnknown {
			if len(ns.GetNewPasswordHash()) != 0 {
				return mtproto.ErrPasswordHashInvalid
			}
			st.Secret = nil
			st.Algo = nil
		} else {
			algo := ns.GetNewAlgo()
			if algo.GetPredicateName() != mtproto.Predicate_passwordKdfAlgoModPow || len(ns.GetNewPasswordHash()) == 0 || len(ns.GetNewPasswordHash()) > 256 {
				return mtproto.ErrPasswordHashInvalid
			}
			parsed := &twofa.PasswordAlgo{
				Salt1: append([]byte(nil), algo.GetSalt1()...),
				Salt2: append([]byte(nil), algo.GetSalt2()...),
				G:     algo.GetG(),
				P:     append([]byte(nil), algo.GetP()...),
			}
			if !twofa.ValidAlgo(*parsed) {
				return mtproto.ErrPasswordHashInvalid
			}
			st.Secret = append([]byte(nil), ns.GetNewPasswordHash()...)
			st.Algo = parsed
		}
		if ns.GetHint() != nil {
			st.Hint = ns.GetHint().GetValue()
		}
		if ns.GetEmail() != nil {
			st.Email = ns.GetEmail().GetValue()
			st.EmailPending = st.Email != ""
		}
		st.HasPassword = len(st.Secret) > 0
	}
	st.RecoveryCode = ""
	st.RecoveryExpire = 0
	st.ResetDeclined = false
	st.ResetRequestedAt = 0
	st.ResetUntilDate = 0
	st.ResetRetryDate = 0
	return nil
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func authAuthorization(user *mtproto.ImmutableUser) *mtproto.Auth_Authorization {
	return mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
		SetupPasswordRequired: false,
		User:                  user.ToSelfUser(),
	}).To_Auth_Authorization()
}
