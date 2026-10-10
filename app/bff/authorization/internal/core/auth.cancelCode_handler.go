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
	"context"

	"github.com/teamgram/proto/mtproto"
	verification "github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

// AuthCancelCode
// auth.cancelCode#1f040578 phone_number:string phone_code_hash:string = Bool;
func (c *AuthorizationCore) AuthCancelCode(in *mtproto.TLAuthCancelCode) (*mtproto.Bool, error) {
	if c == nil || in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD == nil || c.MD.PermAuthKeyId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.AuthLogic == nil || c.svcCtx.Challenges == nil {
		c.Logger.Errorf("auth.cancelCode - provider unavailable")
		return nil, mtproto.ErrMethodNotImpl
	}
	//// 1. check phone code
	//if request.PhoneCodeHash == "" {
	//	err := mtproto.NewRpcError2(mtproto.TLRpcErrorCodes_PHONE_CODE_EMPTY)
	//	return nil, err
	//}
	//
	// 2. check number
	// 客户端发送的手机号格式为: "+86 111 1111 1111"，归一化
	_, phoneNumber, err := phonenumber.CheckPhoneNumberInvalid(in.GetPhoneNumber())
	if err != nil {
		c.Logger.Errorf("check phone_number error - %v", err)
		err = mtproto.ErrPhoneNumberInvalid
		return nil, err
	}

	if in.GetPhoneCodeHash() == "" {
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}
	// Revoke the shared challenges even when the legacy phone-code row has
	// already expired or was evicted.  The shared challenge is independently
	// usable by the sign-in verifier, so returning early here would leave a
	// cancelled code replayable.  Keep the phone-code error as the response
	// while making the revocation best effort for both channels.
	phoneCodeErr := c.svcCtx.AuthLogic.DoAuthCancelCode(c.ctx, c.MD.PermAuthKeyId, phoneNumber, in.GetPhoneCodeHash())
	revokeErr := revokeAuthLoginChallenges(c.ctx, c.svcCtx.Challenges, c.challengeScope(), in.GetPhoneCodeHash())
	if phoneCodeErr != nil {
		return nil, phoneCodeErr
	}
	if revokeErr != nil {
		return nil, revokeErr
	}

	return mtproto.BoolTrue, nil
}

// revokeAuthLoginChallenges attempts both challenge channels before returning
// an error.  A provider failure for one channel must not leave the other
// channel usable after auth.cancelCode has been requested.
func revokeAuthLoginChallenges(ctx context.Context, challenges *verification.ChallengeService, scope, challengeID string) error {
	var firstErr error
	for _, channel := range []verification.Channel{verification.ChannelSMS, verification.ChannelApp} {
		if err := challenges.Revoke(ctx, verification.VerifyRequest{
			Channel: channel, Purpose: challengePurposeAuthLogin, Scope: scope,
			ChallengeID: challengeID,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
