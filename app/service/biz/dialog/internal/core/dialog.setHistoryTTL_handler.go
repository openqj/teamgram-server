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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogSetHistoryTTL
// dialog.setHistoryTTL user_id:long peer_type:int peer_id:long ttl_period:int = Bool;
func (c *DialogCore) DialogSetHistoryTTL(in *dialog.TLDialogSetHistoryTTL) (*mtproto.Bool, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetPeerId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	switch in.GetPeerType() {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetTtlPeriod() < 0 {
		return nil, mtproto.ErrTtlPeriodInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrInternalServerError
	}
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer func() { _ = tx.Rollback(c.ctx) }()
		values := map[string]any{"ttl_period": in.TtlPeriod}
		_, err = c.svcCtx.Dao.Postgres.Store.Dialogs.UpdateCustomMapTx(c.ctx, tx, values, in.UserId, in.PeerType, in.PeerId)
		if err == nil {
			_, err = c.svcCtx.Dao.Postgres.Store.Dialogs.UpdateCustomMapTx(c.ctx, tx, values, in.PeerId, in.PeerType, in.UserId)
		}
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("dialog.setHistoryTTL - error: %v", err)
		}
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
