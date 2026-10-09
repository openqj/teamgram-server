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
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetAccountUsername
// user.getAccountUsername user_id:long = UsernameData;
func (c *UserCore) UserGetAccountUsername(in *user.TLUserGetAccountUsername) (*user.UsernameData, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Username == nil {
		return nil, mtproto.ErrInternalServerError
	}
	v := new(dataobject.UsernameDO)
	list, err := c.svcCtx.Dao.Postgres.Store.Username.SelectByUserID(c.ctx, in.GetUserId())
	if err == nil && len(list) > 0 {
		*v = list[0]
	}
	if err != nil {
		return nil, err
	}

	return user.MakeTLUsernameData(&user.UsernameData{
		Username: v.Username,
		Peer:     mtproto.MakePeerUser(v.PeerId),
		Editable: v.Editable,
		Active:   v.Active,
	}).To_UsernameData(), nil
}
