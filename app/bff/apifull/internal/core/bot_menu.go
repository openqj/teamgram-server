// Copyright 2026 Teamgram Authors
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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCBotMenuServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func b17Key(userID int64, part string) string {
	return "b17:" + strconv.FormatInt(userID, 10) + ":" + part
}

func loadB17AttachBot(userID int64) (*mtproto.AttachMenuBot, error) {
	raw, err := persist.Default.Get(b17Key(userID, "bot"))
	if err != nil || raw == "" {
		return nil, err
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id == 0 {
		return nil, nil
	}
	return mtproto.MakeTLAttachMenuBot(&mtproto.AttachMenuBot{BotId: id}).To_AttachMenuBot(), nil
}

func (c *ApiFullCore) MessagesGetAttachMenuBots(in *mtproto.TLMessagesGetAttachMenuBots) (*mtproto.AttachMenuBots, error) {
	_ = in
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	bot, err := loadB17AttachBot(uid)
	if err != nil {
		return nil, err
	}
	bots := []*mtproto.AttachMenuBot{}
	if bot != nil {
		bots = []*mtproto.AttachMenuBot{bot}
	}
	return mtproto.MakeTLAttachMenuBots(&mtproto.AttachMenuBots{
		Bots:  bots,
		Users: []*mtproto.User{},
	}).To_AttachMenuBots(), nil
}

func (c *ApiFullCore) MessagesGetAttachMenuBot(in *mtproto.TLMessagesGetAttachMenuBot) (*mtproto.AttachMenuBotsBot, error) {
	_ = in
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	bot, err := loadB17AttachBot(uid)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		bot = mtproto.MakeTLAttachMenuBot(&mtproto.AttachMenuBot{}).To_AttachMenuBot()
	}
	return mtproto.MakeTLAttachMenuBotsBot(&mtproto.AttachMenuBotsBot{
		Bot:   bot,
		Users: []*mtproto.User{},
	}).To_AttachMenuBotsBot(), nil
}

func (c *ApiFullCore) MessagesToggleBotInAttachMenu(in *mtproto.TLMessagesToggleBotInAttachMenu) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = miscSave(uid, "MessagesToggleBotInAttachMenu", in); err != nil {
		return nil, err
	}
	var id int64
	enabled := true
	if in != nil && in.GetBot() != nil {
		id = in.GetBot().GetUserId()
	}
	if in != nil && in.GetEnabled() != nil {
		enabled = mtproto.FromBool(in.GetEnabled())
	}
	if !enabled {
		raw, getErr := persist.Default.Get(b17Key(uid, "bot"))
		if getErr != nil {
			return nil, getErr
		}
		if id == 0 || raw == strconv.FormatInt(id, 10) {
			if err = persist.Default.Set(b17Key(uid, "bot"), ""); err != nil {
				return nil, err
			}
		}
		return mtproto.BoolTrue, nil
	}
	if id != 0 {
		if err = persist.Default.Set(b17Key(uid, "bot"), strconv.FormatInt(id, 10)); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}
