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

// AuthImportWebTokenAuthorization
// auth.importWebTokenAuthorization#2db873a9 api_id:int api_hash:string web_auth_token:string = auth.Authorization;
func (c *AuthorizationCore) AuthImportWebTokenAuthorization(in *mtproto.TLAuthImportWebTokenAuthorization) (*mtproto.Auth_Authorization, error) {
	if in == nil || in.GetWebAuthToken() == "" {
		if c != nil {
			c.Logger.Errorf("auth.importWebTokenAuthorization - empty web_auth_token")
		}
		return nil, mtproto.ErrAccessTokenInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.MD == nil || c.MD.GetPermAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if err := c.svcCtx.Dao.CheckApiIdAndHash(in.GetApiId(), in.GetApiHash()); err != nil {
		c.Logger.Errorf("auth.importWebTokenAuthorization - api: %v", err)
		return nil, err
	}

	// No web-token verifier is configured. Do not create a user from an unchecked token.
	c.Logger.Errorf("auth.importWebTokenAuthorization - verifier unavailable")
	return nil, mtproto.ErrMethodNotImpl
}
