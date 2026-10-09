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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserUpdatePremium
// user.updatePremium flags:# user_id:long premium:Bool months:flags.1?int = Bool;
func (c *UserCore) UserUpdatePremium(in *user.TLUserUpdatePremium) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	provider := in.GetProvider()
	transactionID := in.GetTransactionId()
	if provider != "" || transactionID != "" {
		months := in.GetMonths().GetValue()
		if !mtproto.FromBool(in.Premium) || provider == "" || transactionID == "" || months < 1 || months > 36 {
			return nil, mtproto.ErrInputRequestInvalid
		}
		ok, err := c.svcCtx.Dao.GrantUserPremium(c.ctx, in.GetUserId(), months, provider, transactionID)
		if err != nil {
			return nil, err
		}
		return mtproto.ToBool(ok), nil
	}

	err := c.svcCtx.Dao.UpdateUserPremium(
		c.ctx,
		in.GetUserId(),
		mtproto.FromBool(in.Premium),
		in.GetMonths().GetValue())

	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
