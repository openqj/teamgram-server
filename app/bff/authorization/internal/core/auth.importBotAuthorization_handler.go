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
)

// AuthImportBotAuthorization
// auth.importBotAuthorization#67a3ff2c flags:int api_id:int api_hash:string bot_auth_token:string = auth.Authorization;
func (c *AuthorizationCore) AuthImportBotAuthorization(in *mtproto.TLAuthImportBotAuthorization) (*mtproto.Auth_Authorization, error) {
	if in == nil || in.GetBotAuthToken() == "" {
		if c != nil {
			c.Logger.Errorf("auth.importBotAuthorization - empty bot_auth_token")
		}
		return nil, mtproto.ErrAccessTokenInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.MD == nil || c.MD.GetPermAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if err := c.svcCtx.Dao.CheckApiIdAndHash(in.GetApiId(), in.GetApiHash()); err != nil {
		c.Logger.Errorf("auth.importBotAuthorization - api: %v", err)
		return nil, err
	}
	var expectedID int64
	// The User service owns bot tokens in PostgreSQL. Use the signed external
	// verifier when configured, but keep a complete local path for deployments
	// that intentionally make that service the authority for bot credentials.
	if c.authProviderConfigured() {
		verified, err := c.authProvider(c.ctx, authProviderRequest{
			Operation:    "verify_bot_authorization",
			APIID:        in.GetApiId(),
			APIHash:      in.GetApiHash(),
			BotAuthToken: in.GetBotAuthToken(),
		})
		if err != nil {
			return nil, err
		}
		expectedID = verified.UserID
	}
	user, err := c.loadBotByProviderToken(in.GetBotAuthToken(), expectedID)
	if err != nil {
		return nil, err
	}
	return c.bindProviderUser(user)
}
