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
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// AuthDropTempAuthKeys
// auth.dropTempAuthKeys#8e48a188 except_auth_keys:Vector<long> = Bool;
func (c *AuthorizationCore) AuthDropTempAuthKeys(in *mtproto.TLAuthDropTempAuthKeys) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.GetPermAuthKeyId() == 0 {
		if c != nil {
			c.Logger.Errorf("auth.dropTempAuthKeys - perm auth key empty")
		}
		return nil, mtproto.ErrAuthKeyInvalid
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.AuthsessionClient == nil {
		c.Logger.Errorf("auth.dropTempAuthKeys - authsession provider unavailable")
		return nil, mtproto.ErrMethodNotImpl
	}
	ok, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionDropTempAuthKeys(c.ctx, &authsession.TLAuthsessionDropTempAuthKeys{ExceptAuthKeys: in.GetExceptAuthKeys()})
	if err != nil {
		c.Logger.Errorf("auth.dropTempAuthKeys - error: %v", err)
		return nil, err
	}
	if ok == nil || ok.GetPredicateName() != mtproto.Predicate_boolTrue {
		c.Logger.Errorf("auth.dropTempAuthKeys - authsession returned no success result")
		return nil, mtproto.ErrInternalServerError
	}
	return ok, nil
}
