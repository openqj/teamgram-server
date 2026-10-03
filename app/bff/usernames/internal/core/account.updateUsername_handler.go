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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountUpdateUsername
// account.updateUsername#3e0bdd7c username:string = User;
func (c *UsernamesCore) AccountUpdateUsername(in *mtproto.TLAccountUpdateUsername) (*mtproto.User, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}

	username := in.GetUsername()
	if username != "" && usernameFormatInvalid(username) {
		return nil, mtproto.ErrUsernameInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("account.updateUsername - error: %v", err)
		return nil, err
	}
	if me == nil || me.GetUser() == nil {
		c.Logger.Errorf("account.updateUsername - user service returned an empty user")
		return nil, mtproto.ErrInternalServerError
	}

	ok, err := c.svcCtx.Dao.UserClient.UserUpdateUsername(c.ctx, &userpb.TLUserUpdateUsername{
		UserId:   c.MD.UserId,
		Username: username,
	})
	if err != nil {
		c.Logger.Errorf("account.updateUsername - error: %v", err)
		return nil, err
	}
	if !mtproto.FromBool(ok) {
		return nil, mtproto.ErrInternalServerError
	}

	me.SetUsername(username)
	synced, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateUserName(&mtproto.Update{
			UserId:    c.MD.UserId,
			FirstName: me.FirstName(),
			LastName:  me.LastName(),
			Username:  username,
		}).To_Update()),
	})
	if err != nil {
		c.Logger.Errorf("account.updateUsername - sync error: %v", err)
		return nil, fmt.Errorf("account.updateUsername: username was saved but sync update failed: %w", err)
	}
	if synced == nil {
		c.Logger.Errorf("account.updateUsername - sync returned an empty response")
		return nil, fmt.Errorf("account.updateUsername: username was saved but sync returned an empty response")
	}

	return me.ToSelfUser(), nil
}
