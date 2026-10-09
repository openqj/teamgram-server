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
)

// AuthResetAuthorizations
// auth.resetAuthorizations#9fab0d1a = Bool;
func (c *AuthorizationCore) AuthResetAuthorizations(in *mtproto.TLAuthResetAuthorizations) (*mtproto.Bool, error) {
	_ = in
	if c == nil || c.MD == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.MD.GetUserId() == 0 {
		c.Logger.Errorf("auth.resetAuthorizations - user not bound")
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.MD.PermAuthKeyId == 0 {
		c.Logger.Errorf("auth.resetAuthorizations - perm auth key empty")
		return nil, mtproto.ErrAuthKeyInvalid
	}

	// hash 0 revokes every session except AuthKeyId (the current one).
	tKeyIdList, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionResetAuthorization(c.ctx, &authsession.TLAuthsessionResetAuthorization{
		UserId:    c.MD.GetUserId(),
		AuthKeyId: c.MD.PermAuthKeyId,
		Hash:      0,
	})
	if err != nil {
		c.Logger.Errorf("auth.resetAuthorizations - error: %v", err)
		return nil, err
	}
	if tKeyIdList == nil {
		c.Logger.Errorf("auth.resetAuthorizations - authsession returned no result")
		return nil, mtproto.ErrInternalServerError
	}

	for _, id := range tKeyIdList.Datas {
		if _, err = c.svcCtx.Dao.SyncClient.SyncUpdatesMe(
			c.ctx,
			&sync.TLSyncUpdatesMe{
				UserId:        c.MD.GetUserId(),
				PermAuthKeyId: id,
				Updates:       mtproto.MakeTLUpdatesTooLong(nil).To_Updates(),
			}); err != nil {
			c.Logger.Errorf("auth.resetAuthorizations - notify updates too long: %v", err)
			return nil, err
		}
		if _, err = c.svcCtx.Dao.SyncClient.SyncUpdatesMe(
			c.ctx,
			&sync.TLSyncUpdatesMe{
				UserId:        c.MD.GetUserId(),
				PermAuthKeyId: id,
				Updates: mtproto.MakeTLUpdateAccountResetAuthorization(&mtproto.Updates{
					UserId:    c.MD.GetUserId(),
					AuthKeyId: id,
				}).To_Updates(),
			}); err != nil {
			c.Logger.Errorf("auth.resetAuthorizations - notify reset: %v", err)
			return nil, err
		}
	}

	return mtproto.BoolTrue, nil
}
