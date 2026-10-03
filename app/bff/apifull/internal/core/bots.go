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
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCBotsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) BotsSetBotCommands(in *mtproto.TLBotsSetBotCommands) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var scope *mtproto.BotCommandScope
	var lang string
	var cmds []*mtproto.BotCommand
	if in != nil {
		scope = in.GetScope()
		lang = in.GetLangCode()
		cmds = in.GetCommands()
	}
	key := botCommandStoreKey(userId, scope, lang)
	if err := putBotCommands(key, cmds); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsResetBotCommands(in *mtproto.TLBotsResetBotCommands) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = putBotWrite(userId, "BotsResetBotCommands", in); err != nil {
		return nil, err
	}
	var scope *mtproto.BotCommandScope
	var lang string
	if in != nil {
		scope = in.GetScope()
		lang = in.GetLangCode()
	}
	if err = putBotCommands(botCommandStoreKey(userId, scope, lang), nil); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsGetBotCommands(in *mtproto.TLBotsGetBotCommands) (*mtproto.Vector_BotCommand, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var scope *mtproto.BotCommandScope
	var lang string
	if in != nil {
		scope = in.GetScope()
		lang = in.GetLangCode()
	}
	key := botCommandStoreKey(userId, scope, lang)
	cmds, err := getBotCommands(key)
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_BotCommand{Datas: cmds}, nil
}

func (c *ApiFullCore) BotsSetBotInfo(in *mtproto.TLBotsSetBotInfo) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsGetBotInfoDCD914FD(in *mtproto.TLBotsGetBotInfoDCD914FD) (*mtproto.Bots_BotInfo, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsGetAdminedBots(in *mtproto.TLBotsGetAdminedBots) (*mtproto.Vector_User, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsCheckUsername(in *mtproto.TLBotsCheckUsername) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsCreateBot(in *mtproto.TLBotsCreateBot) (*mtproto.User, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsExportBotToken(in *mtproto.TLBotsExportBotToken) (*mtproto.Bots_ExportedBotToken, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsGetAccessSettings(in *mtproto.TLBotsGetAccessSettings) (*mtproto.Bots_AccessSettings, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsEditAccessSettings(in *mtproto.TLBotsEditAccessSettings) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsSetJoinChatResults(in *mtproto.TLBotsSetJoinChatResults) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsGetBotInfo75EC12E6(in *mtproto.TLBotsGetBotInfo75EC12E6) (*mtproto.Vector_String, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

type botCommandKey struct {
	userId    int64
	lang      string
	scopeCtor int32
	peerId    int64
	scopeUser int64
}

func botCommandStoreKey(userId int64, scope *mtproto.BotCommandScope, lang string) string {
	key := botCommandKey{userId: userId, lang: lang}
	if scope != nil {
		key.scopeCtor = int32(scope.GetConstructor())
		if peer := scope.GetPeer(); peer != nil {
			key.peerId = peer.GetUserId()
			if key.peerId == 0 {
				key.peerId = peer.GetChatId()
			}
			if key.peerId == 0 {
				key.peerId = peer.GetChannelId()
			}
		}
		if u := scope.GetUserId(); u != nil {
			key.scopeUser = u.UserId
		}
	}
	return fmt.Sprintf("botcmd:%d:%d:%d:%d:%s", key.userId, key.scopeCtor, key.peerId, key.scopeUser, key.lang)
}

func putBotCommands(key string, cmds []*mtproto.BotCommand) error {
	raw, err := json.Marshal(cloneBotCommands(cmds))
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(raw))
}

func getBotCommands(key string) ([]*mtproto.BotCommand, error) {
	raw, err := persist.Default.Get(key)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return []*mtproto.BotCommand{}, nil
	}
	var cmds []*mtproto.BotCommand
	if err := json.Unmarshal([]byte(raw), &cmds); err != nil {
		return nil, err
	}
	if cmds == nil {
		cmds = []*mtproto.BotCommand{}
	}
	return cmds, nil
}

func putBotWrite(userId int64, method string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set(fmt.Sprintf("botw:%d:%s", userId, method), string(raw))
}

func cloneBotCommands(in []*mtproto.BotCommand) []*mtproto.BotCommand {
	out := make([]*mtproto.BotCommand, 0, len(in))
	for _, cmd := range in {
		if cmd == nil {
			continue
		}
		copied := *cmd
		out = append(out, &copied)
	}
	return out
}
