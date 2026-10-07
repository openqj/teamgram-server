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
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type signInCodeStore interface {
	GetCachePhoneCode(context.Context, int64, string) (*model.PhoneCodeTransaction, error)
	DeleteCachePhoneCode(context.Context, int64, string) error
}

// AccountInvalidateSignInCodes
// account.invalidateSignInCodes#ca8ae8ba codes:Vector<string> = Bool;
func (c *AuthorizationCore) AccountInvalidateSignInCodes(in *mtproto.TLAccountInvalidateSignInCodes) (*mtproto.Bool, error) {
	if c == nil || in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD == nil || c.MD.PermAuthKeyId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

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
	if c.MD.UserId != 0 {
		if c.svcCtx.Dao.UserClient == nil {
			return nil, mtproto.ErrMethodNotImpl
		}
		user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
			Id: c.MD.UserId,
		})
		if err != nil {
			return nil, err
		}
		if user != nil && user.User != nil && user.Phone() != "" {
			if err = deleteMatchingSignInCode(c.ctx, c.svcCtx.Dao, authKeyId, user.Phone(), want); err != nil {
				return nil, err
			}
		}
	}
	for code := range want {
		if err := deleteMatchingSignInCode(c.ctx, c.svcCtx.Dao, authKeyId, code, want); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}

func deleteMatchingSignInCode(ctx context.Context, store signInCodeStore, authKeyId int64, key string, want map[string]struct{}) error {
	if key == "" || key == authorizationSettingsKey {
		return nil
	}
	codeData, err := store.GetCachePhoneCode(ctx, authKeyId, key)
	if err != nil {
		return err
	}
	if codeData == nil || !signInCodeMatches(codeData, want) {
		return nil
	}

	keys := []string{key, codeData.PhoneNumber, codeData.PhoneCodeHash}
	seen := make(map[string]struct{}, len(keys))
	for _, cacheKey := range keys {
		if cacheKey == "" || cacheKey == authorizationSettingsKey {
			continue
		}
		if _, ok := seen[cacheKey]; ok {
			continue
		}
		seen[cacheKey] = struct{}{}
		if err := store.DeleteCachePhoneCode(ctx, authKeyId, cacheKey); err != nil {
			return err
		}
	}
	return nil
}

func signInCodeMatches(codeData *model.PhoneCodeTransaction, want map[string]struct{}) bool {
	if codeData == nil {
		return false
	}
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
