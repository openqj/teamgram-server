// Copyright 2026 Teamgram Authors
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
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	verification "github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/twofa"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCTwoFaServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) AccountGetPassword(in *mtproto.TLAccountGetPassword) (*mtproto.Account_Password, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	st, err := loadAcctPassword(userID)
	if err != nil {
		return nil, err
	}
	newAlgo, err := newPasswordAlgo()
	if err != nil {
		return nil, err
	}
	hasSecret := len(st.Secret) > 0
	out := &mtproto.Account_Password{
		HasRecovery:     hasSecret && st.Email != "" && !st.EmailPending,
		HasSecureValues: false,
		HasPassword:     hasSecret,
		NewAlgo:         passwordAlgoToMTProto(newAlgo),
		NewSecureAlgo: mtproto.MakeTLSecurePasswordKdfAlgoPBKDF2(&mtproto.SecurePasswordKdfAlgo{
			Salt: []byte{0x7D, 0x04, 0xB3, 0x4B, 0x94, 0x82, 0x8C, 0x3D},
		}).To_SecurePasswordKdfAlgo(),
		SecureRandom: crypto.RandomBytes(256),
	}
	if hasSecret {
		currentAlgo := passwordAlgoForState(st)
		if !twofa.ValidAlgo(currentAlgo) {
			return nil, mtproto.ErrPasswordHashInvalid
		}
		challenge, err := createSRPChallenge(userID, st, currentAlgo)
		if err != nil {
			return nil, err
		}
		out.CurrentAlgo = passwordAlgoToMTProto(currentAlgo)
		out.Srp_B = challenge.B
		out.SrpId = wrapperspb.Int64(challenge.SrpID)
		if st.Hint != "" {
			out.Hint = wrapperspb.String(st.Hint)
		}
	}
	if st.EmailPending && st.Email != "" {
		out.EmailUnconfirmedPattern = wrapperspb.String(passwordEmailPattern(st.Email))
	}
	return mtproto.MakeTLAccountPassword(out).To_Account_Password(), nil
}

func (c *ApiFullCore) AccountGetPasswordSettings(in *mtproto.TLAccountGetPasswordSettings) (*mtproto.Account_PasswordSettings, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	st, err := loadAcctPassword(userID)
	if err != nil {
		return nil, err
	}
	var check *mtproto.InputCheckPasswordSRP
	if in != nil {
		check = in.GetPassword()
	}
	if err = checkStoredPassword(userID, st, check); err != nil {
		return nil, err
	}
	settings := &mtproto.Account_PasswordSettings{}
	if st.Email != "" {
		settings.Email = wrapperspb.String(st.Email)
	}
	return mtproto.MakeTLAccountPasswordSettings(settings).To_Account_PasswordSettings(), nil
}

func (c *ApiFullCore) AccountUpdatePasswordSettings(in *mtproto.TLAccountUpdatePasswordSettings) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	st, err := loadAcctPassword(userID)
	if err != nil {
		return nil, err
	}
	var check *mtproto.InputCheckPasswordSRP
	if in != nil {
		check = in.GetPassword()
	}
	if err = checkStoredPassword(userID, st, check); err != nil {
		return nil, err
	}
	passwordChanged := false
	if in != nil && in.GetNewSettings() != nil {
		ns := in.GetNewSettings()
		if ns.GetHint() != nil {
			st.Hint = ns.GetHint().GetValue()
		}
		if ns.GetEmail() != nil {
			st.Email = ns.GetEmail().GetValue()
			st.EmailPending = st.Email != ""
		}
		if newAlgo := ns.GetNewAlgo(); newAlgo != nil {
			passwordChanged = true
			if newAlgo.GetPredicateName() == mtproto.Predicate_passwordKdfAlgoUnknown {
				if len(ns.GetNewPasswordHash()) != 0 {
					return nil, mtproto.ErrPasswordHashInvalid
				}
				st.Secret = nil
				st.Algo = nil
			} else {
				algo, algoErr := passwordAlgoFromMTProto(newAlgo)
				if algoErr != nil || len(ns.GetNewPasswordHash()) == 0 || len(ns.GetNewPasswordHash()) > 256 {
					return nil, mtproto.ErrPasswordHashInvalid
				}
				st.Secret = append([]byte(nil), ns.GetNewPasswordHash()...)
				st.Algo = algo
			}
		} else if len(ns.GetNewPasswordHash()) > 0 {
			return nil, mtproto.ErrPasswordHashInvalid
		}
	}
	st.RecoveryCode = ""
	st.RecoveryExpire = 0
	if passwordChanged {
		st.ResetDeclined = false
		st.ResetRequestedAt = 0
		st.ResetUntilDate = 0
		st.ResetRetryDate = 0
	}
	st.HasPassword = len(st.Secret) > 0
	if err := saveAcctPassword(userID, st); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountConfirmPasswordEmail(in *mtproto.TLAccountConfirmPasswordEmail) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	st, err := loadAcctPassword(userID)
	if err != nil {
		return nil, err
	}
	if st.Email == "" {
		return nil, mtproto.ErrEmailNotSetup
	}
	if in == nil || in.GetCode() == "" {
		return nil, mtproto.ErrCodeEmpty
	}
	if !st.EmailPending || st.EmailCode == "" || c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return nil, mtproto.ErrCodeInvalid
	}
	_, err = c.svcCtx.Challenges.Consume(c.ctx, verification.VerifyRequest{
		Channel: verification.ChannelEmail, Purpose: "account.password_email",
		Scope: passwordEmailScope(c), ChallengeID: st.EmailCode, Code: strings.TrimSpace(in.GetCode()),
	})
	if err != nil {
		switch {
		case errors.Is(err, verification.ErrChallengeInvalid):
			return nil, mtproto.ErrCodeInvalid
		case errors.Is(err, verification.ErrChallengeNotFound), errors.Is(err, verification.ErrChallengeExpired):
			return nil, mtproto.ErrEmailVerifyExpired
		default:
			return nil, mtproto.ErrInternalServerError
		}
	}
	st.EmailPending = false
	st.EmailCode = ""
	if err = saveAcctPassword(userID, st); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountResendPasswordEmail(in *mtproto.TLAccountResendPasswordEmail) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	st, err := loadAcctPassword(userID)
	if err != nil {
		return nil, err
	}
	if st.Email == "" {
		return nil, mtproto.ErrEmailNotSetup
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return nil, mtproto.ErrSendCodeUnavailable
	}
	if st.EmailCode != "" {
		if err = c.svcCtx.Challenges.Revoke(c.ctx, verification.VerifyRequest{
			Channel: verification.ChannelEmail, Purpose: "account.password_email",
			Scope: passwordEmailScope(c), ChallengeID: st.EmailCode,
		}); err != nil {
			return nil, err
		}
	}
	issued, err := c.svcCtx.Challenges.Issue(c.ctx, verification.IssueRequest{
		Channel: verification.ChannelEmail, Purpose: "account.password_email",
		Subject: st.Email, Scope: passwordEmailScope(c), CodeLength: 6,
	})
	if err != nil {
		if errors.Is(err, verification.ErrRateLimited) || verification.ProviderError(err) {
			return nil, mtproto.ErrSendCodeUnavailable
		}
		return nil, mtproto.ErrInternalServerError
	}
	st.EmailCode = issued.ID
	st.EmailPending = true
	if err = saveAcctPassword(userID, st); err != nil {
		_ = c.svcCtx.Challenges.Revoke(c.ctx, verification.VerifyRequest{
			Channel: verification.ChannelEmail, Purpose: "account.password_email",
			Scope: passwordEmailScope(c), ChallengeID: issued.ID,
		})
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func passwordEmailScope(c *ApiFullCore) string {
	if c == nil || c.MD == nil {
		return ""
	}
	return verification.ScopeID(c.MD.PermAuthKeyId)
}

func (c *ApiFullCore) AccountCancelPasswordEmail(in *mtproto.TLAccountCancelPasswordEmail) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	st, err := loadAcctPassword(userID)
	if err != nil {
		return nil, err
	}
	if st.EmailCode != "" && c.svcCtx != nil && c.svcCtx.Challenges != nil {
		if err = c.svcCtx.Challenges.Revoke(c.ctx, verification.VerifyRequest{
			Channel: verification.ChannelEmail, Purpose: "account.password_email",
			Scope: passwordEmailScope(c), ChallengeID: st.EmailCode,
		}); err != nil {
			return nil, err
		}
	}
	st.Email = ""
	st.EmailPending = false
	st.EmailCode = ""
	if err := saveAcctPassword(userID, st); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountDeclinePasswordReset(in *mtproto.TLAccountDeclinePasswordReset) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	key := twofa.PasswordKey(userID)
	for attempt := 0; attempt < 4; attempt++ {
		raw, readErr := persist.Default.Get(key)
		if readErr != nil {
			return nil, readErr
		}
		var st acctPassword
		if raw != "" {
			if readErr = json.Unmarshal([]byte(raw), &st); readErr != nil {
				return nil, readErr
			}
		}
		st.ResetDeclined = true
		st.HasPassword = len(st.Secret) > 0
		encoded, marshalErr := json.Marshal(st)
		if marshalErr != nil {
			return nil, marshalErr
		}
		updated, swapErr := persist.CompareAndSwap(key, raw, string(encoded))
		if swapErr != nil {
			return nil, swapErr
		}
		if updated {
			return mtproto.BoolTrue, nil
		}
	}
	return nil, mtproto.ErrInternalServerError
}

type acctPassword = twofa.PasswordState

func checkStoredPassword(userID int64, st acctPassword, in *mtproto.InputCheckPasswordSRP) error {
	return twofa.VerifyPassword(accountPasswordProofStore{}, userID, st, in)
}

type accountPasswordProofStore struct{}

func (accountPasswordProofStore) Get(key string) (string, error) {
	return persist.Default.Get(key)
}

func (accountPasswordProofStore) CompareAndDelete(key, expected string) (bool, error) {
	return persist.CompareAndDelete(key, expected)
}

func passwordEmailPattern(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "***"
	}
	local := email[:at]
	r := []rune(local)
	if len(r) == 0 {
		return "***" + email[at:]
	}
	return string(r[0]) + "***" + email[at:]
}

func acctPut(key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(b))
}

func loadAcctPassword(userID int64) (acctPassword, error) {
	var st acctPassword
	raw, err := persist.Default.Get(twofa.PasswordKey(userID))
	if err != nil || raw == "" {
		return st, err
	}
	err = json.Unmarshal([]byte(raw), &st)
	return st, err
}

func saveAcctPassword(userID int64, st acctPassword) error {
	st.HasPassword = len(st.Secret) > 0
	return acctPut(twofa.PasswordKey(userID), st)
}

func newPasswordAlgo() (twofa.PasswordAlgo, error) {
	a := twofa.PasswordAlgo{Salt1: make([]byte, 32), Salt2: make([]byte, 32), G: 3, P: twofa.SupportedPrime()}
	if _, err := rand.Read(a.Salt1); err != nil {
		return twofa.PasswordAlgo{}, err
	}
	if _, err := rand.Read(a.Salt2); err != nil {
		return twofa.PasswordAlgo{}, err
	}
	return a, nil
}

func passwordAlgoToMTProto(a twofa.PasswordAlgo) *mtproto.PasswordKdfAlgo {
	return mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
		Salt1: append([]byte(nil), a.Salt1...),
		Salt2: append([]byte(nil), a.Salt2...),
		G:     a.G,
		P:     append([]byte(nil), a.P...),
	}).To_PasswordKdfAlgo()
}

func passwordAlgoFromMTProto(a *mtproto.PasswordKdfAlgo) (*twofa.PasswordAlgo, error) {
	if a == nil || a.GetPredicateName() != mtproto.Predicate_passwordKdfAlgoModPow {
		return nil, mtproto.ErrPasswordHashInvalid
	}
	value := &twofa.PasswordAlgo{
		Salt1: append([]byte(nil), a.GetSalt1()...),
		Salt2: append([]byte(nil), a.GetSalt2()...),
		G:     a.GetG(),
		P:     append([]byte(nil), a.GetP()...),
	}
	if !twofa.ValidAlgo(*value) {
		return nil, mtproto.ErrPasswordHashInvalid
	}
	return value, nil
}

func passwordAlgoForState(st acctPassword) twofa.PasswordAlgo {
	if st.Algo == nil {
		return twofa.LegacyAlgo()
	}
	return *st.Algo
}

func createSRPChallenge(userID int64, st acctPassword, algo twofa.PasswordAlgo) (twofa.SRPChallenge, error) {
	id, err := rand.Int(rand.Reader, big.NewInt(1<<63-1))
	if err != nil {
		return twofa.SRPChallenge{}, err
	}
	challenge := twofa.SRPChallenge{SrpID: id.Int64() + 1, ExpiresAt: time.Now().Add(5 * time.Minute).Unix()}
	util := crypto.MakeSRPUtil(&crypto.PasswordKdfAlgoModPow{
		Salt1: algo.Salt1,
		Salt2: algo.Salt2,
		G:     algo.G,
		P:     algo.P,
	})
	challenge.BNonce, challenge.B = util.CalcSRPB(st.Secret)
	if len(challenge.BNonce) == 0 || len(challenge.B) == 0 {
		return twofa.SRPChallenge{}, mtproto.ErrPasswordHashInvalid
	}
	raw, err := json.Marshal(challenge)
	if err != nil {
		return twofa.SRPChallenge{}, err
	}
	if err = persist.Default.Set(twofa.ChallengeKey(userID, challenge.SrpID), string(raw)); err != nil {
		return twofa.SRPChallenge{}, err
	}
	return challenge, nil
}
