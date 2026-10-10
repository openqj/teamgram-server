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
	"hash/fnv"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
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
	rec := attachMenuRecord{}
	if json.Unmarshal([]byte(raw), &rec) != nil {
		// Keep reads compatible with the old numeric value while all new writes
		// use the structured, atomically updated PostgreSQL KV record.
		rec.BotID, err = strconv.ParseInt(raw, 10, 64)
	}
	if rec.BotID == 0 {
		return nil, nil
	}
	return mtproto.MakeTLAttachMenuBot(&mtproto.AttachMenuBot{
		BotId:              rec.BotID,
		RequestWriteAccess: rec.WriteAllowed,
		ShowInAttachMenu:   true,
	}).To_AttachMenuBot(), nil
}

type attachMenuRecord struct {
	BotID        int64 `json:"bot_id"`
	WriteAllowed bool  `json:"write_allowed"`
}

func (c *ApiFullCore) attachMenuUser(in *mtproto.InputUser) (int64, *mtproto.ImmutableUser, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, nil, err
	}
	if in == nil {
		return 0, nil, mtproto.ErrInputUserDeactivated
	}
	peer := mtproto.FromInputUser(uid, in)
	if peer == nil || peer.PeerId <= 0 || (peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_SELF) {
		return 0, nil, mtproto.ErrUserIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, nil, mtproto.ErrMethodNotImpl
	}
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{Id: []int64{uid, peer.PeerId}})
	if err != nil {
		return 0, nil, err
	}
	if users == nil {
		return 0, nil, mtproto.ErrInternalServerError
	}
	bot, ok := users.GetImmutableUser(peer.PeerId)
	if !ok || bot == nil || bot.GetUser() == nil || bot.GetUser().GetBot() == nil || bot.GetUser().GetDeleted() {
		return 0, nil, mtproto.ErrBotInvalid
	}
	if in.GetPredicateName() == mtproto.Predicate_inputUser && in.GetAccessHash() != bot.GetUser().GetAccessHash() {
		return 0, nil, mtproto.ErrBotInvalid
	}
	data := bot.GetUser().GetBot()
	if !data.GetBotAttachMenu() || !data.GetAttachMenuEnabled() {
		return 0, nil, mtproto.ErrBotInvalid
	}
	return peer.PeerId, bot, nil
}

func attachMenuBotView(bot *mtproto.ImmutableUser, writeAllowed bool) *mtproto.AttachMenuBot {
	data := bot.GetUser().GetBot()
	return mtproto.MakeTLAttachMenuBot(&mtproto.AttachMenuBot{
		BotId:              bot.GetUser().GetId(),
		ShortName:          bot.GetUser().GetUsername(),
		RequestWriteAccess: writeAllowed,
		ShowInAttachMenu:   data.GetAttachMenuEnabled(),
	}).To_AttachMenuBot()
}

func attachMenuHash(bot *mtproto.AttachMenuBot) int64 {
	if bot == nil {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.FormatInt(bot.GetBotId(), 10)))
	if bot.GetRequestWriteAccess() {
		_, _ = h.Write([]byte{1})
	}
	return int64(h.Sum64())
}

func (c *ApiFullCore) MessagesGetAttachMenuBots(in *mtproto.TLMessagesGetAttachMenuBots) (*mtproto.AttachMenuBots, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	bot, err := loadB17AttachBot(uid)
	if err != nil {
		return nil, err
	}
	bots := []*mtproto.AttachMenuBot{}
	usersOut := []*mtproto.User{}
	if bot != nil {
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
			return nil, mtproto.ErrMethodNotImpl
		}
		users, getErr := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{Id: []int64{uid, bot.GetBotId()}})
		if getErr != nil {
			return nil, getErr
		}
		if users == nil {
			return nil, mtproto.ErrInternalServerError
		}
		immutable, ok := users.GetImmutableUser(bot.GetBotId())
		if !ok || immutable == nil || immutable.GetUser() == nil || immutable.GetUser().GetBot() == nil {
			return nil, mtproto.ErrBotInvalid
		}
		bot = attachMenuBotView(immutable, bot.GetRequestWriteAccess())
		if in != nil && in.GetHash() != 0 && in.GetHash() == attachMenuHash(bot) {
			return mtproto.MakeTLAttachMenuBotsNotModified(&mtproto.AttachMenuBots{}).To_AttachMenuBots(), nil
		}
		bots = []*mtproto.AttachMenuBot{bot}
		usersOut = []*mtproto.User{immutable.ToUser(uid)}
	}
	return mtproto.MakeTLAttachMenuBots(&mtproto.AttachMenuBots{
		Hash: func() int64 {
			if len(bots) == 0 {
				return 0
			}
			return attachMenuHash(bots[0])
		}(),
		Bots:  bots,
		Users: usersOut,
	}).To_AttachMenuBots(), nil
}

func (c *ApiFullCore) MessagesGetAttachMenuBot(in *mtproto.TLMessagesGetAttachMenuBot) (*mtproto.AttachMenuBotsBot, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputUserDeactivated
	}
	botID, immutable, err := c.attachMenuUser(in.GetBot())
	if err != nil {
		return nil, err
	}
	record, _ := loadB17AttachBot(uid)
	writeAllowed := record != nil && record.GetBotId() == botID && record.GetRequestWriteAccess()
	return mtproto.MakeTLAttachMenuBotsBot(&mtproto.AttachMenuBotsBot{
		Bot:   attachMenuBotView(immutable, writeAllowed),
		Users: []*mtproto.User{immutable.ToUser(uid)},
	}).To_AttachMenuBotsBot(), nil
}

func (c *ApiFullCore) MessagesToggleBotInAttachMenu(in *mtproto.TLMessagesToggleBotInAttachMenu) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputUserDeactivated
	}
	id, _, err := c.attachMenuUser(in.GetBot())
	if err != nil {
		return nil, err
	}
	enabled := true
	if in != nil && in.GetEnabled() != nil {
		enabled = mtproto.FromBool(in.GetEnabled())
	}
	if !enabled {
		if err = persist.Update(b17Key(uid, "bot"), func(current string) (string, error) {
			if current == "" || current == strconv.FormatInt(id, 10) {
				return "", nil
			}
			var record attachMenuRecord
			if json.Unmarshal([]byte(current), &record) == nil && record.BotID == id {
				return "", nil
			}
			return current, nil
		}); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	record := attachMenuRecord{BotID: id, WriteAllowed: in.GetWriteAllowed()}
	encoded, _ := json.Marshal(record)
	if err = persist.Update(b17Key(uid, "bot"), func(string) (string, error) { return string(encoded), nil }); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
