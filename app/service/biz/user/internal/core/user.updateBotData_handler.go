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
	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserUpdateBotData
// user.updateBotData flags:# user_id:long bot_chat_history:flags.15?Bool bot_nochats:flags.16?Bool bot_inline_geo:flags.21?Bool bot_attach_menu:flags.27?Bool bot_inline_placeholder:flags.19?string = Bool;
func (c *UserCore) UserUpdateBotData(in *user.TLUserUpdateBotData) (*mtproto.Bool, error) {
	if in == nil || in.GetBotId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Bots == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrMethodNotImpl
	}

	changes := make(map[string]any, 6)
	if in.GetBotChatHistory() != nil {
		changes["bot_chat_history"] = mtproto.FromBool(in.GetBotChatHistory())
	}
	if in.GetBotNochats() != nil {
		changes["bot_nochats"] = mtproto.FromBool(in.GetBotNochats())
	}
	if in.GetBotInlineGeo() != nil {
		changes["bot_inline_geo"] = mtproto.FromBool(in.GetBotInlineGeo())
	}
	if in.GetBotAttachMenu() != nil {
		changes["bot_attach_menu"] = mtproto.FromBool(in.GetBotAttachMenu())
	}
	if in.GetBotInlinePlaceholder() != nil {
		changes["bot_inline_placeholder"] = in.GetBotInlinePlaceholder().GetValue()
	}
	if in.GetBotHasMainApp() != nil {
		changes["bot_has_main_app"] = mtproto.FromBool(in.GetBotHasMainApp())
	}
	err := c.svcCtx.Dao.Postgres.InTx(c.ctx, func(tx pgx.Tx) error {
		botDO, err := c.svcCtx.Dao.Postgres.Store.Bots.SelectForUpdateTx(c.ctx, tx, in.GetBotId())
		if err != nil {
			return err
		}
		if botDO == nil || botDO.BotId != in.GetBotId() {
			return mtproto.ErrBotInvalid
		}
		if botDO.CreatorUserId <= 0 {
			return mtproto.ErrMethodNotImpl
		}
		if botDO.CreatorUserId != c.MD.GetUserId() {
			return mtproto.ErrForbiddenUserBotInvalid
		}
		if len(changes) == 0 {
			return nil
		}
		_, err = c.svcCtx.Dao.Postgres.Store.Bots.UpdateTx(c.ctx, tx, changes, in.GetBotId())
		return err
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
