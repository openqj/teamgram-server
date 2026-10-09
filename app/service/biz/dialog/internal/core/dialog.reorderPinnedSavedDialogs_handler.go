// Copyright 2024 Teamgram Authors
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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogReorderPinnedSavedDialogs
// dialog.reorderPinnedSavedDialogs user_id:long force:Bool order:Vector<PeerUtil> = Bool;
func (c *DialogCore) DialogReorderPinnedSavedDialogs(in *dialog.TLDialogReorderPinnedSavedDialogs) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.SavedDialogs == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		userId      = in.GetUserId()
		force       = mtproto.FromBool(in.GetForce())
		orderPinned = time.Now().Unix()
	)
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(c.ctx) }()
	if force {
		if _, err = c.svcCtx.Dao.Postgres.Store.SavedDialogs.UpdateUserUnPinnedTx(c.ctx, tx, userId); err != nil {
			return nil, err
		}
	}
	for _, id := range in.Order {
		if _, err = c.svcCtx.Dao.Postgres.Store.SavedDialogs.UpdateUserPeerPinnedTx(c.ctx, tx, orderPinned<<32, userId, id.PeerType, id.PeerId); err != nil {
			return nil, err
		}
		orderPinned--
	}
	if err = tx.Commit(c.ctx); err != nil {
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
