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

// DialogSetChatTheme
// dialog.setChatTheme user_id:long peer_type:int peer_id:long theme_emoticon:string = Bool;
func (c *DialogCore) DialogSetChatTheme(in *dialog.TLDialogSetChatTheme) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, mtproto.ErrInternalServerError
	}
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer func() { _ = tx.Rollback(c.ctx) }()
		values := map[string]any{"theme_emoticon": in.ThemeEmoticon}
		_, err = c.svcCtx.Dao.Postgres.Store.Dialogs.UpdateCustomMapTx(c.ctx, tx, values, in.UserId, in.PeerType, in.PeerId)
		if err == nil {
			_, err = c.svcCtx.Dao.Postgres.Store.Dialogs.UpdateCustomMapTx(c.ctx, tx, values, in.PeerId, in.PeerType, in.UserId)
		}
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		c.Logger.Errorf("dialog.setChatTheme - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
