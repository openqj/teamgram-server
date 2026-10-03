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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountDeleteAccount
// account.deleteAccount#a2c0cf74 flags:# reason:string password:flags.0?InputCheckPasswordSRP = Bool;
func (c *AccountCore) AccountDeleteAccount(in *mtproto.TLAccountDeleteAccount) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.AuthsessionClient == nil || c.svcCtx.Dao.SyncClient == nil || c.svcCtx.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	me, err := c.svcCtx.Dao.UserClient.UserGetUserDataById(c.ctx, &user.TLUserGetUserDataById{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("account.deleteAccount - error: %v", err)
		return nil, err
	}

	if me == nil {
		return nil, mtproto.ErrInternalServerError
	}

	if me.Username != "" {
		usernameDeleted, deleteErr := c.svcCtx.Dao.UserClient.UserDeleteUsername(c.ctx, &user.TLUserDeleteUsername{
			Username: me.Username,
		})
		if deleteErr != nil {
			err = deleteErr
		}
		if err == nil && (usernameDeleted == nil || !mtproto.FromBool(usernameDeleted)) {
			err = mtproto.ErrInternalServerError
		}
		if err != nil {
			c.Logger.Errorf("account.deleteAccount - error: %v", err)
			return nil, err
		}
	}

	deleted, err := c.svcCtx.UserClient.UserDeleteUser(c.ctx, &user.TLUserDeleteUser{
		UserId: c.MD.UserId,
		Reason: in.GetReason(),
		Phone:  me.GetPhone(),
	})
	if err != nil {
		c.Logger.Errorf("account.deleteAccount - error: %v", err)
		return nil, err
	}
	if deleted == nil || !mtproto.FromBool(deleted) {
		return nil, mtproto.ErrInternalServerError
	}

	// s.AuthSessionRpcClient
	tKeyIdList, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionResetAuthorization(c.ctx, &authsession.TLAuthsessionResetAuthorization{
		UserId:    c.MD.UserId,
		AuthKeyId: 0,
		Hash:      0,
	})
	if err != nil {
		c.Logger.Errorf("account.resetAuthorization#df77f3bc - error: %v", err)
		return nil, err
	}
	if tKeyIdList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	for _, id := range tKeyIdList.Datas {
		// notify kill session
		upds := mtproto.MakeTLUpdateAccountResetAuthorization(&mtproto.Updates{
			UserId:    c.MD.UserId,
			AuthKeyId: id,
		}).To_Updates()
		if _, err = c.svcCtx.Dao.SyncClient.SyncUpdatesMe(
			c.ctx,
			&sync.TLSyncUpdatesMe{
				UserId:        c.MD.UserId,
				PermAuthKeyId: id,
				ServerId:      nil,
				AuthKeyId:     nil,
				SessionId:     nil,
				Updates:       upds,
			}); err != nil {
			c.Logger.Errorf("account.deleteAccount - notify reset: %v", err)
			return nil, err
		}
	}

	if _, err = c.svcCtx.Dao.AuthsessionClient.AuthsessionUnbindAuthKeyUser(c.ctx, &authsession.TLAuthsessionUnbindAuthKeyUser{
		AuthKeyId: 0,
		UserId:    c.MD.UserId,
	}); err != nil {
		c.Logger.Errorf("account.deleteAccount - unbind authorizations: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
