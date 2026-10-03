// Copyright 2025 Teamgram Authors
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
	"encoding/base64"

	"github.com/teamgram/proto/mtproto"
)

// AccountDeletePasskey
// account.deletePasskey#f5b5563f id:string = Bool;
func (c *PasskeyCore) AccountDeletePasskey(in *mtproto.TLAccountDeletePasskey) (*mtproto.Bool, error) {
	userID, err := passkeyRequireUser(c)
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetId() == "" {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	if err = c.requireProvider(); err != nil {
		return nil, err
	}
	if c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	id, err := base64.RawURLEncoding.DecodeString(in.GetId())
	if err != nil || len(id) == 0 {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	deleted, err := c.svcCtx.Dao.DeleteCredential(c.ctx, userID, id)
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	if !deleted {
		return mtproto.BoolFalse, nil
	}
	return mtproto.BoolTrue, nil
}
