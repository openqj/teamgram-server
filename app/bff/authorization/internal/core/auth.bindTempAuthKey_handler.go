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

// AuthBindTempAuthKey
// auth.bindTempAuthKey#cdd42a05 perm_auth_key_id:long nonce:long expires_at:int encrypted_message:bytes = Bool;
func (c *AuthorizationCore) AuthBindTempAuthKey(in *mtproto.TLAuthBindTempAuthKey) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	authID := c.MD.GetAuthId()
	if authID == 0 || (c.MD.PermAuthKeyId != 0 && authID == c.MD.PermAuthKeyId) {
		c.Logger.Errorf("auth.bindTempAuthKey - temp auth key empty")
		return nil, mtproto.ErrTempAuthKeyEmpty
	}
	// authsession decrypts message[8:24] and message[24:]. Keep the same
	// framing gate here so malformed requests never reach a lower-level slice.
	if in == nil || in.GetPermAuthKeyId() == 0 || len(in.GetEncryptedMessage()) < 56 || (len(in.GetEncryptedMessage())-24)%16 != 0 {
		c.Logger.Errorf("auth.bindTempAuthKey - encrypted message invalid")
		return nil, mtproto.ErrEncryptedMessageInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.AuthsessionClient == nil {
		c.Logger.Errorf("auth.bindTempAuthKey - authsession provider unavailable")
		return nil, mtproto.ErrMethodNotImpl
	}

	ok, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionBindTempAuthKey(c.ctx, &authsession.TLAuthsessionBindTempAuthKey{
		PermAuthKeyId:    in.PermAuthKeyId,
		Nonce:            in.Nonce,
		ExpiresAt:        in.ExpiresAt,
		EncryptedMessage: in.EncryptedMessage,
	})
	if err != nil {
		c.Logger.Errorf("auth.bindTempAuthKey - error: %v", err)
		return nil, err
	}
	return ok, nil
}
