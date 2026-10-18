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
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	verification "github.com/teamgram/teamgram-server/pkg/code"
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
	keys := make([]string, 0, len(want)+1)
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
		if user == nil || user.GetUser() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if user.Phone() != "" {
			keys = append(keys, user.Phone())
		}
	}
	for code := range want {
		keys = append(keys, code)
	}
	if c.svcCtx.Challenges == nil {
		if c.Logger != nil {
			c.Logger.Errorf("account.invalidateSignInCodes - challenge provider unavailable")
		}
		return nil, mtproto.ErrMethodNotImpl
	}
	matched, err := matchingSignInCodeChallenges(c.ctx, c.svcCtx.Dao, c.svcCtx.Challenges, authKeyId, c.challengeScope(), keys, want)
	if err != nil {
		return nil, err
	}
	for _, item := range matched {
		if err := revokeAuthLoginChallenges(c.ctx, c.svcCtx.Challenges, c.challengeScope(), item.data.PhoneCodeHash); err != nil {
			return nil, err
		}
	}
	for _, item := range matched {
		if err := deleteSignInCodeAliases(c.ctx, c.svcCtx.Dao, authKeyId, item.key, item.data); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}

type matchedSignInCode struct {
	key  string
	data *model.PhoneCodeTransaction
}

func matchingSignInCodeChallenges(ctx context.Context, store signInCodeStore, challenges *verification.ChallengeService, authKeyId int64, scope string, keys []string, want map[string]struct{}) ([]matchedSignInCode, error) {
	if challenges == nil || scope == "" {
		return nil, mtproto.ErrMethodNotImpl
	}
	seen := make(map[string]struct{})
	matched := make([]matchedSignInCode, 0, len(keys))
	for _, key := range keys {
		if key == "" || key == authorizationSettingsKey {
			continue
		}
		codeData, err := store.GetCachePhoneCode(ctx, authKeyId, key)
		if err != nil {
			return nil, err
		}
		if codeData == nil || codeData.PhoneCodeHash == "" {
			continue
		}
		isMatch := signInCodeMatches(codeData, want)
		if !isMatch {
			isMatch, err = challengeMatchesSignInCode(ctx, challenges, scope, codeData, want)
			if err != nil {
				return nil, err
			}
		}
		if !isMatch {
			continue
		}
		if _, ok := seen[codeData.PhoneCodeHash]; ok {
			continue
		}
		seen[codeData.PhoneCodeHash] = struct{}{}
		matched = append(matched, matchedSignInCode{key: key, data: codeData})
	}
	return matched, nil
}

func challengeMatchesSignInCode(ctx context.Context, challenges *verification.ChallengeService, scope string, codeData *model.PhoneCodeTransaction, want map[string]struct{}) (bool, error) {
	if codeData == nil || codeData.PhoneCodeHash == "" {
		return false, nil
	}
	for code := range want {
		for _, channel := range []verification.Channel{verification.ChannelSMS, verification.ChannelApp} {
			record, err := challenges.Check(ctx, verification.VerifyRequest{
				Channel: channel, Purpose: challengePurposeAuthLogin,
				Scope: scope, ChallengeID: codeData.PhoneCodeHash, Code: code,
			})
			if err == nil {
				if record != nil && normalizeChallengePhone(record.Subject) == normalizeChallengePhone(codeData.PhoneNumber) {
					return true, nil
				}
				continue
			}
			if errors.Is(err, verification.ErrChallengeNotFound) || errors.Is(err, verification.ErrChallengeExpired) || errors.Is(err, verification.ErrChallengeInvalid) {
				continue
			}
			return false, err
		}
	}
	return false, nil
}

func revokeMatchingSignInCodeChallenges(ctx context.Context, store signInCodeStore, challenges *verification.ChallengeService, authKeyId int64, scope string, keys []string, want map[string]struct{}) error {
	matched, err := matchingSignInCodeChallenges(ctx, store, challenges, authKeyId, scope, keys, want)
	if err != nil {
		return err
	}
	for _, item := range matched {
		if err = revokeAuthLoginChallenges(ctx, challenges, scope, item.data.PhoneCodeHash); err != nil {
			return err
		}
	}
	return nil
}

func matchingSignInCode(ctx context.Context, store signInCodeStore, authKeyId int64, key string, want map[string]struct{}) (*model.PhoneCodeTransaction, error) {
	if key == "" || key == authorizationSettingsKey {
		return nil, nil
	}
	codeData, err := store.GetCachePhoneCode(ctx, authKeyId, key)
	if err != nil {
		return nil, err
	}
	if codeData == nil || !signInCodeMatches(codeData, want) {
		return nil, nil
	}
	return codeData, nil
}

func deleteMatchingSignInCode(ctx context.Context, store signInCodeStore, authKeyId int64, key string, want map[string]struct{}) error {
	codeData, err := matchingSignInCode(ctx, store, authKeyId, key, want)
	if err != nil || codeData == nil {
		return err
	}
	return deleteSignInCodeAliases(ctx, store, authKeyId, key, codeData)
}

func deleteSignInCodeAliases(ctx context.Context, store signInCodeStore, authKeyId int64, key string, codeData *model.PhoneCodeTransaction) error {
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
