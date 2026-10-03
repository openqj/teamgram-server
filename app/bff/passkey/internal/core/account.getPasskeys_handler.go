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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// AccountGetPasskeys
// account.getPasskeys#ea1f0c52 = account.Passkeys;
func (c *PasskeyCore) AccountGetPasskeys(in *mtproto.TLAccountGetPasskeys) (*mtproto.Account_Passkeys, error) {
	userID, err := passkeyRequireUser(c)
	if err != nil {
		return nil, err
	}
	if err = c.requireProvider(); err != nil {
		return nil, err
	}
	if c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_ = in
	credentials, err := c.svcCtx.Dao.ListCredentials(c.ctx, userID)
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	passkeys := make([]*mtproto.Passkey, 0, len(credentials))
	for _, credential := range credentials {
		var lastUsage *wrapperspb.Int32Value
		if credential.LastUsageDate > 0 {
			lastUsage = wrapperspb.Int32(int32(credential.LastUsageDate))
		}
		passkeys = append(passkeys, &mtproto.Passkey{
			Id:            base64.RawURLEncoding.EncodeToString(credential.ID),
			Name:          credential.Name,
			Date:          int32(credential.Date),
			LastUsageDate: lastUsage,
		})
	}
	return mtproto.MakeTLAccountPasskeys(&mtproto.Account_Passkeys{Passkeys: passkeys}).To_Account_Passkeys(), nil
}
