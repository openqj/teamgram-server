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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AccountSendConfirmPhoneCode
// account.sendConfirmPhoneCode#1b3faa88 hash:string settings:CodeSettings = auth.SentCode;
func (c *AccountCore) AccountSendConfirmPhoneCode(in *mtproto.TLAccountSendConfirmPhoneCode) (*mtproto.Auth_SentCode, error) {
	if c == nil || in == nil || in.GetSettings() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	purposeHash := strings.TrimSpace(in.GetHash())
	if purposeHash == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD == nil || c.MD.GetPermAuthKeyId() == 0 || c.MD.GetUserId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.AuthLogic == nil || c.svcCtx.Challenges == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	user, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: c.MD.GetUserId()})
	if err != nil {
		return nil, err
	}
	if user == nil || strings.TrimSpace(user.GetUser().GetPhone()) == "" {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	phoneNumber := strings.TrimSpace(user.GetUser().GetPhone())
	if c.svcCtx.Plugin != nil {
		banned, _ := c.svcCtx.Plugin.CheckPhoneNumberBanned(c.ctx, phoneNumber)
		if banned {
			c.Logger.Errorf("account.sendConfirmPhoneCode - phone banned")
			return nil, mtproto.ErrPhoneNumberBanned
		}
	}

	settings := in.GetSettings()
	codeData, err := c.svcCtx.AuthLogic.DoAuthSendCode(
		c.ctx,
		c.MD.PermAuthKeyId,
		c.MD.SessionId,
		phoneNumber,
		settings.GetAllowFlashcall(),
		settings.GetCurrentNumber(),
		func(codeData2 *model.PhoneCodeTransaction) error {
			codeData2.PhoneCodeExtraData = purposeHash
			return c.issueSMSChallenge(codeData2, challengePurposeConfirmPhone)
		}, func(codeData2 *model.PhoneCodeTransaction) {
			if c.svcCtx.Challenges == nil || codeData2 == nil {
				return
			}
			_ = c.svcCtx.Challenges.Revoke(c.ctx, verification.VerifyRequest{
				Channel: verification.ChannelSMS, Purpose: challengePurposeConfirmPhone,
				Scope: c.challengeScope(), ChallengeID: codeData2.PhoneCodeHash,
			})
		})
	if err != nil {
		c.Logger.Errorf("account.sendConfirmPhoneCode - error: %v", err)
		return nil, err
	}

	// confirmPhone carries only the challenge hash, so index the metadata by hash.
	if codeData.PhoneCodeHash != "" && codeData.PhoneCodeHash != phoneNumber {
		if err = c.svcCtx.Dao.PutCachePhoneCode(c.ctx, c.MD.PermAuthKeyId, codeData.PhoneCodeHash, codeData); err != nil {
			c.Logger.Errorf("account.sendConfirmPhoneCode - index challenge metadata: %v", err)
			_ = c.svcCtx.Dao.DeleteCachePhoneCode(c.ctx, c.MD.PermAuthKeyId, codeData.PhoneNumber)
			_ = c.svcCtx.Dao.DeleteCachePhoneCode(c.ctx, c.MD.PermAuthKeyId, codeData.PhoneCodeHash)
			_ = c.svcCtx.Challenges.Revoke(c.ctx, verification.VerifyRequest{
				Channel: verification.ChannelSMS, Purpose: challengePurposeConfirmPhone,
				Scope: c.challengeScope(), ChallengeID: codeData.PhoneCodeHash,
			})
			return nil, mtproto.ErrInternalServerError
		}
	}

	return codeData.ToAuthSentCode(), nil
}
