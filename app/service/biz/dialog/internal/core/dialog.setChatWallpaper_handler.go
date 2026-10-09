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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogSetChatWallpaper
// dialog.setChatWallpaper flags:# user_id:long peer_type:int peer_id:long wallpaper_id:long wallpaper_overridden:flags.0?true = Bool;
func (c *DialogCore) DialogSetChatWallpaper(in *dialog.TLDialogSetChatWallpaper) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	store := c.svcCtx.Dao.Postgres.Store
	values := map[string]interface{}{
		"wallpaper_id":         in.WallpaperId,
		"wallpaper_overridden": in.WallpaperOverridden,
	}
	if in.WallpaperId == 0 {
		values["wallpaper_overridden"] = false
	}
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer func() { _ = tx.Rollback(c.ctx) }()
		_, err = store.Dialogs.UpdateCustomMapTx(c.ctx, tx, values, in.UserId, in.PeerType, in.PeerId)
		if err == nil && in.WallpaperOverridden {
			_, err = store.Dialogs.UpdateCustomMapTx(c.ctx, tx, values, in.PeerId, in.PeerType, in.UserId)
		}
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("dialog.setChatWallpaper - error: %v", err)
		}
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
