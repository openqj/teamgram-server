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
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountInvalidateSignInCodes
// account.invalidateSignInCodes#ca8ae8ba codes:Vector<string> = Bool;
func (c *AuthorizationCore) AccountInvalidateSignInCodes(in *mtproto.TLAccountInvalidateSignInCodes) (*mtproto.Bool, error) {
	want := make(map[string]struct{})
	for _, code := range in.GetCodes() {
		if code != "" {
			want[code] = struct{}{}
		}
	}
	if len(want) == 0 {
		return mtproto.BoolTrue, nil
	}

	authKeyId := c.MD.PermAuthKeyId
	if c.MD.UserId != 0 && c.svcCtx.Dao.UserClient != nil {
		user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
			Id: c.MD.UserId,
		})
		if err == nil && user != nil && user.User != nil && user.Phone() != "" {
			c.deleteMatchingSignInCode(authKeyId, user.Phone(), want)
		}
	}
	for code := range want {
		c.deleteMatchingSignInCode(authKeyId, code, want)
	}
	return mtproto.BoolTrue, nil
}

func (c *AuthorizationCore) deleteMatchingSignInCode(authKeyId int64, key string, want map[string]struct{}) {
	if key == "" || key == authorizationSettingsKey {
		return
	}
	codeData, err := c.svcCtx.Dao.GetCachePhoneCode(c.ctx, authKeyId, key)
	if err != nil || codeData == nil {
		return
	}
	if !signInCodeMatches(codeData, want) {
		return
	}
	_ = c.svcCtx.Dao.DeleteCachePhoneCode(c.ctx, authKeyId, key)
	if codeData.PhoneNumber != "" && codeData.PhoneNumber != key {
		_ = c.svcCtx.Dao.DeleteCachePhoneCode(c.ctx, authKeyId, codeData.PhoneNumber)
	}
	if codeData.PhoneCodeHash != "" && codeData.PhoneCodeHash != key {
		_ = c.svcCtx.Dao.DeleteCachePhoneCode(c.ctx, authKeyId, codeData.PhoneCodeHash)
	}
}

func signInCodeMatches(codeData *model.PhoneCodeTransaction, want map[string]struct{}) bool {
	if codeData.PhoneCode != "" {
		if _, ok := want[codeData.PhoneCode]; ok {
			return true
		}
	}
	if codeData.PhoneCodeHash != "" {
		if _, ok := want[codeData.PhoneCodeHash]; ok {
			return true
		}
	}
	return false
}
