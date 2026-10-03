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

// RPCBusinessConnectedBotsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type connectedBotJSON struct {
	BotId         int64                          `json:"bot_id"`
	CanReply      bool                           `json:"can_reply"`
	Rights        *mtproto.BusinessBotRights     `json:"rights,omitempty"`
	RecipientsBot *mtproto.BusinessBotRecipients `json:"recipients_bot,omitempty"`
	Recipients    *mtproto.BusinessRecipients    `json:"recipients,omitempty"`
}

func connectedBotsKey(userId int64) string {
	return fmt.Sprintf("cbot:%d", userId)
}

func loadConnectedBots(userId int64) ([]connectedBotJSON, error) {
	raw, err := persist.Default.Get(connectedBotsKey(userId))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []connectedBotJSON
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveConnectedBots(userId int64, list []connectedBotJSON) error {
	if list == nil {
		list = []connectedBotJSON{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(connectedBotsKey(userId), string(b))
}

func inputUserIDs(users []*mtproto.InputUser) []int64 {
	out := make([]int64, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, u.GetUserId())
	}
	return out
}

func businessBotRecipientsFromInput(in *mtproto.InputBusinessBotRecipients) *mtproto.BusinessBotRecipients {
	if in == nil {
		return nil
	}
	return mtproto.MakeTLBusinessBotRecipients(&mtproto.BusinessBotRecipients{
		ExistingChats:   in.GetExistingChats(),
		NewChats:        in.GetNewChats(),
		Contacts:        in.GetContacts(),
		NonContacts:     in.GetNonContacts(),
		ExcludeSelected: in.GetExcludeSelected(),
		Users:           inputUserIDs(in.GetUsers()),
		ExcludeUsers:    inputUserIDs(in.GetExcludeUsers()),
	}).To_BusinessBotRecipients()
}

func businessRecipientsFromInput(in *mtproto.InputBusinessRecipients) *mtproto.BusinessRecipients {
	if in == nil {
		return nil
	}
	return mtproto.MakeTLBusinessRecipients(&mtproto.BusinessRecipients{
		ExistingChats:   in.GetExistingChats(),
		NewChats:        in.GetNewChats(),
		Contacts:        in.GetContacts(),
		NonContacts:     in.GetNonContacts(),
		ExcludeSelected: in.GetExcludeSelected(),
		Users:           inputUserIDs(in.GetUsers()),
	}).To_BusinessRecipients()
}

func (c *ApiFullCore) AccountUpdateConnectedBot(in *mtproto.TLAccountUpdateConnectedBot) (*mtproto.Updates, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	var deleted, canReply bool
	var botId int64
	if in != nil {
		deleted = in.GetDeleted()
		canReply = in.GetCanReply()
		if in.GetBot() != nil {
			botId = in.GetBot().GetUserId()
		}
	}
	list, err := loadConnectedBots(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	if deleted {
		if botId == 0 {
			list = []connectedBotJSON{}
		} else {
			next := list[:0]
			for _, row := range list {
				if row.BotId != botId {
					next = append(next, row)
				}
			}
			list = next
		}
		if err := saveConnectedBots(c.MD.UserId, list); err != nil {
			return nil, err
		}
		return mtproto.MakeEmptyUpdates(), nil
	}
	var rights *mtproto.BusinessBotRights
	var recBot *mtproto.BusinessBotRecipients
	var rec *mtproto.BusinessRecipients
	if in != nil {
		rights = in.GetRights()
		recBot = businessBotRecipientsFromInput(in.GetRecipients_INPUTBUSINESSBOTRECIPIENTS())
		rec = businessRecipientsFromInput(in.GetRecipients_INPUTBUSINESSRECIPIENTS())
	}
	row := connectedBotJSON{
		BotId:         botId,
		CanReply:      canReply,
		Rights:        rights,
		RecipientsBot: recBot,
		Recipients:    rec,
	}
	found := false
	for i := range list {
		if list[i].BotId == botId {
			list[i] = row
			found = true
			break
		}
	}
	if !found {
		list = append(list, row)
	}
	if err := saveConnectedBots(c.MD.UserId, list); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) AccountGetConnectedBots(in *mtproto.TLAccountGetConnectedBots) (*mtproto.Account_ConnectedBots, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	_ = in
	list, err := loadConnectedBots(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	bots := make([]*mtproto.ConnectedBot, 0, len(list))
	for _, row := range list {
		rights := row.Rights
		if rights == nil {
			rights = mtproto.MakeTLBusinessBotRights(&mtproto.BusinessBotRights{}).To_BusinessBotRights()
		}
		recipients := row.RecipientsBot
		if recipients == nil {
			recipients = mtproto.MakeTLBusinessBotRecipients(&mtproto.BusinessBotRecipients{}).To_BusinessBotRecipients()
		}
		bots = append(bots, mtproto.MakeTLConnectedBot(&mtproto.ConnectedBot{
			BotId:                            row.BotId,
			CanReply:                         row.CanReply,
			Rights:                           rights,
			Recipients_BUSINESSBOTRECIPIENTS: recipients,
			Recipients_BUSINESSRECIPIENTS:    row.Recipients,
		}).To_ConnectedBot())
	}
	return mtproto.MakeTLAccountConnectedBots(&mtproto.Account_ConnectedBots{
		ConnectedBots: bots,
		Users:         []*mtproto.User{},
	}).To_Account_ConnectedBots(), nil
}

func (c *ApiFullCore) AccountGetBotBusinessConnection(in *mtproto.TLAccountGetBotBusinessConnection) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetConnectionId() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	// Connected-bot settings do not store bot-facing business connection IDs.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) AccountToggleConnectedBotPaused(in *mtproto.TLAccountToggleConnectedBotPaused) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	paused := false
	var peerUser, peerChat, peerChannel int64
	if in != nil {
		if in.GetPaused() != nil {
			paused = mtproto.FromBool(in.GetPaused())
		}
		if p := in.GetPeer(); p != nil {
			peerUser, peerChat, peerChannel = p.GetUserId(), p.GetChatId(), p.GetChannelId()
		}
	}
	b, err := json.Marshal(struct {
		Paused      bool  `json:"paused"`
		PeerUser    int64 `json:"peer_user"`
		PeerChat    int64 `json:"peer_chat"`
		PeerChannel int64 `json:"peer_channel"`
	}{paused, peerUser, peerChat, peerChannel})
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(fmt.Sprintf("cbot:pause:%d", c.MD.UserId), string(b)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountDisablePeerConnectedBot(in *mtproto.TLAccountDisablePeerConnectedBot) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	var peerUser, peerChat, peerChannel int64
	if in != nil {
		if p := in.GetPeer(); p != nil {
			peerUser, peerChat, peerChannel = p.GetUserId(), p.GetChatId(), p.GetChannelId()
		}
	}
	b, err := json.Marshal(struct {
		PeerUser    int64 `json:"peer_user"`
		PeerChat    int64 `json:"peer_chat"`
		PeerChannel int64 `json:"peer_channel"`
	}{peerUser, peerChat, peerChannel})
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(fmt.Sprintf("cbot:off:%d", c.MD.UserId), string(b)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
