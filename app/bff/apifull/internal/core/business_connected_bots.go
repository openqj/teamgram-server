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
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/dccontext"
	"google.golang.org/grpc/status"
)

var errBusinessConnectionIDInvalid = status.Error(mtproto.ErrBadRequest, "CONNECTION_ID_INVALID")

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

func marshalConnectedJSON(value any) ([]byte, error) {
	if value == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(value)
}

func connectedBotPeer(in *mtproto.InputPeer) (int64, int64, error) {
	if in == nil {
		return 0, 0, mtproto.ErrInputRequestInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPeerUser:
		if in.GetUserId() <= 0 || in.GetAccessHash() == 0 {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		return 0, in.GetUserId(), nil
	case mtproto.Predicate_inputPeerChat:
		if in.GetChatId() <= 0 {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		return 1, in.GetChatId(), nil
	case mtproto.Predicate_inputPeerChannel:
		if in.GetChannelId() <= 0 || in.GetAccessHash() == 0 {
			return 0, 0, mtproto.ErrChannelInvalid
		}
		return 2, in.GetChannelId(), nil
	}
	return 0, 0, mtproto.ErrPeerIdInvalid
}

func inputUserIDs(users []*mtproto.InputUser) ([]int64, error) {
	out := make([]int64, 0, len(users))
	for _, u := range users {
		if u == nil || u.GetPredicateName() != mtproto.Predicate_inputUser {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		if u.GetUserId() <= 0 || u.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		out = append(out, u.GetUserId())
	}
	return out, nil
}

// hydrateCanonicalUsers resolves only the requested IDs through the
// authoritative User service. Connected-bot state stores IDs, not a second
// copy of Telegram User constructors, so a missing or malformed profile must
// fail closed instead of producing a synthetic user.
func (c *ApiFullCore) hydrateCanonicalUsers(ids []int64, viewerID int64, botIDs map[int64]struct{}) ([]*mtproto.User, error) {
	if len(ids) == 0 {
		return []*mtproto.User{}, nil
	}
	if viewerID <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	requested := make([]int64, 0, len(ids))
	wanted := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		if _, ok := wanted[id]; ok {
			continue
		}
		wanted[id] = struct{}{}
		requested = append(requested, id)
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profiles, err := d.UserGetMutableUsers(callContext(c), &userpb.TLUserGetMutableUsers{Id: requested})
	if err != nil {
		return nil, err
	}
	if profiles == nil {
		return nil, mtproto.ErrInternalServerError
	}
	byID := make(map[int64]*mtproto.ImmutableUser, len(profiles.GetDatas()))
	for _, profile := range profiles.GetDatas() {
		if profile == nil || profile.GetUser() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		id := profile.GetUser().GetId()
		if _, ok := wanted[id]; !ok {
			continue
		}
		if _, duplicate := byID[id]; duplicate {
			return nil, mtproto.ErrInternalServerError
		}
		byID[id] = profile
	}

	users := make([]*mtproto.User, 0, len(requested))
	for _, id := range requested {
		profile := byID[id]
		if profile == nil || profile.GetUser() == nil || profile.GetUser().GetId() != id || profile.Deleted() {
			return nil, mtproto.ErrUserIdInvalid
		}
		if profile.GetUser().GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		_, requireBot := botIDs[id]
		if requireBot && profile.GetUser().GetBot() == nil {
			return nil, mtproto.ErrBotInvalid
		}
		if !requireBot && profile.GetUser().GetBot() != nil {
			return nil, mtproto.ErrUserIsBot
		}
		user := profile.ToUser(viewerID)
		if user == nil || user.GetId() != id || user.GetAccessHash() == nil || user.GetAccessHash().GetValue() == 0 {
			return nil, mtproto.ErrInternalServerError
		}
		users = append(users, user)
	}
	return users, nil
}

func businessBotRecipientsFromInput(in *mtproto.InputBusinessBotRecipients) (*mtproto.BusinessBotRecipients, error) {
	if in == nil {
		return nil, nil
	}
	users, err := inputUserIDs(in.GetUsers())
	if err != nil {
		return nil, err
	}
	excludeUsers, err := inputUserIDs(in.GetExcludeUsers())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLBusinessBotRecipients(&mtproto.BusinessBotRecipients{
		ExistingChats:   in.GetExistingChats(),
		NewChats:        in.GetNewChats(),
		Contacts:        in.GetContacts(),
		NonContacts:     in.GetNonContacts(),
		ExcludeSelected: in.GetExcludeSelected(),
		Users:           users,
		ExcludeUsers:    excludeUsers,
	}).To_BusinessBotRecipients(), nil
}

func businessRecipientsFromInput(in *mtproto.InputBusinessRecipients) (*mtproto.BusinessRecipients, error) {
	if in == nil {
		return nil, nil
	}
	users, err := inputUserIDs(in.GetUsers())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLBusinessRecipients(&mtproto.BusinessRecipients{
		ExistingChats:   in.GetExistingChats(),
		NewChats:        in.GetNewChats(),
		Contacts:        in.GetContacts(),
		NonContacts:     in.GetNonContacts(),
		ExcludeSelected: in.GetExcludeSelected(),
		Users:           users,
	}).To_BusinessRecipients(), nil
}

func (c *ApiFullCore) verifyConnectedBot(input *mtproto.InputUser) (int64, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return 0, err
	}
	if input == nil {
		return 0, mtproto.ErrInputRequestInvalid
	}
	botID := callerID
	explicitInputUser := false
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
	case mtproto.Predicate_inputUser:
		explicitInputUser = true
		botID = input.GetUserId()
		if botID <= 0 || input.GetAccessHash() == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
	default:
		return 0, mtproto.ErrInputConstructorInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	profile, err := d.UserGetImmutableUser(callContext(c), &userpb.TLUserGetImmutableUser{Id: botID})
	if err != nil {
		return 0, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetId() != botID || profile.Deleted() || profile.GetUser().GetBot() == nil {
		return 0, mtproto.ErrBotInvalid
	}
	if explicitInputUser && profile.GetUser().GetAccessHash() != input.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return botID, nil
}

func (c *ApiFullCore) AccountUpdateConnectedBot(in *mtproto.TLAccountUpdateConnectedBot) (*mtproto.Updates, error) {
	if err := c.requirePremium(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.verifyConnectedBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	deleted, canReply := in.GetDeleted(), in.GetCanReply()
	if domain.Ready() {
		dcID := int32(0)
		if c.svcCtx != nil {
			dcID = c.svcCtx.Config.DcId
		}
		if actual, ok := dccontext.DCID(c.requestContext()); ok {
			dcID = actual
		}
		if dcID <= 0 {
			return nil, mtproto.ErrDcIdInvalid
		}
		rights, err := marshalConnectedJSON(in.GetRights())
		if err != nil {
			return nil, err
		}
		recipientsBotValue, err := businessBotRecipientsFromInput(in.GetRecipients_INPUTBUSINESSBOTRECIPIENTS())
		if err != nil {
			return nil, err
		}
		recipientsBot, err := marshalConnectedJSON(recipientsBotValue)
		if err != nil {
			return nil, err
		}
		recipientsValue, err := businessRecipientsFromInput(in.GetRecipients_INPUTBUSINESSRECIPIENTS())
		if err != nil {
			return nil, err
		}
		recipients, err := marshalConnectedJSON(recipientsValue)
		if err != nil {
			return nil, err
		}
		if in.GetRecipients_INPUTBUSINESSRECIPIENTS() == nil {
			recipients = nil
		}
		if _, err := domain.UpsertConnectedBotAndConnection(c.MD.UserId, botID, canReply,
			rights, recipientsBot, recipients, dcID, deleted); err != nil {
			return nil, err
		}
		return mtproto.MakeEmptyUpdates(), nil
	}
	list, err := loadConnectedBots(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	if deleted {
		if botID == 0 {
			list = []connectedBotJSON{}
		} else {
			next := list[:0]
			for _, row := range list {
				if row.BotId != botID {
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
		recBot, err = businessBotRecipientsFromInput(in.GetRecipients_INPUTBUSINESSBOTRECIPIENTS())
		if err != nil {
			return nil, err
		}
		rec, err = businessRecipientsFromInput(in.GetRecipients_INPUTBUSINESSRECIPIENTS())
		if err != nil {
			return nil, err
		}
	}
	row := connectedBotJSON{
		BotId:         botID,
		CanReply:      canReply,
		Rights:        rights,
		RecipientsBot: recBot,
		Recipients:    rec,
	}
	found := false
	for i := range list {
		if list[i].BotId == botID {
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
	if err := c.requirePremium(); err != nil {
		return nil, err
	}
	_ = in
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	rows, err := domain.ListConnectedBots(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	botIDs := make(map[int64]struct{}, len(rows))
	ids := make([]int64, 0, len(rows))
	bots := make([]*mtproto.ConnectedBot, 0, len(rows))
	for _, row := range rows {
		if row.BotID <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		botIDs[row.BotID] = struct{}{}
		ids = append(ids, row.BotID)
		rights := mtproto.MakeTLBusinessBotRights(&mtproto.BusinessBotRights{}).To_BusinessBotRights()
		if len(row.Rights) > 0 && string(row.Rights) != "null" {
			if err := json.Unmarshal(row.Rights, rights); err != nil {
				return nil, err
			}
		}
		recipients := mtproto.MakeTLBusinessBotRecipients(&mtproto.BusinessBotRecipients{}).To_BusinessBotRecipients()
		if len(row.RecipientsBot) > 0 && string(row.RecipientsBot) != "null" {
			if err := json.Unmarshal(row.RecipientsBot, recipients); err != nil {
				return nil, err
			}
		}
		var direct *mtproto.BusinessRecipients
		if len(row.Recipients) > 0 && string(row.Recipients) != "null" {
			direct = mtproto.MakeTLBusinessRecipients(&mtproto.BusinessRecipients{}).To_BusinessRecipients()
			if err := json.Unmarshal(row.Recipients, direct); err != nil {
				return nil, err
			}
		}
		bots = append(bots, mtproto.MakeTLConnectedBot(&mtproto.ConnectedBot{
			BotId: row.BotID, CanReply: row.CanReply, Rights: rights,
			Recipients_BUSINESSBOTRECIPIENTS: recipients, Recipients_BUSINESSRECIPIENTS: direct,
		}).To_ConnectedBot())
	}
	users, err := c.hydrateCanonicalUsers(ids, c.MD.UserId, botIDs)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountConnectedBots(&mtproto.Account_ConnectedBots{
		ConnectedBots: bots,
		Users:         users,
	}).To_Account_ConnectedBots(), nil
}

func (c *ApiFullCore) AccountGetBotBusinessConnection(in *mtproto.TLAccountGetBotBusinessConnection) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetConnectionId() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if domain.Ready() {
		record, found, loadErr := domain.LoadBusinessConnection(uid, in.GetConnectionId())
		if loadErr != nil {
			return nil, loadErr
		}
		if !found {
			return nil, errBusinessConnectionIDInvalid
		}
		if record.OwnerUserID == uid {
			return nil, mtproto.ErrUserIdInvalid
		}
		users, err := c.hydrateCanonicalUsers([]int64{uid, record.OwnerUserID}, uid, map[int64]struct{}{uid: {}})
		if err != nil {
			return nil, err
		}
		return businessConnectionUpdatesWithUsers(record, users[1:])
	}
	var result mtproto.Updates
	if err = c.callBusinessProvider("account.getBotBusinessConnection", map[string]any{
		"user_id": uid, "connection_id": in.GetConnectionId(),
	}, &result); err != nil {
		return nil, err
	}
	ownerID, err := validateBotBusinessConnectionUpdates(&result, in.GetConnectionId())
	if err != nil {
		return nil, err
	}
	if ownerID == uid {
		return nil, mtproto.ErrUserIdInvalid
	}
	users, err := c.hydrateCanonicalUsers([]int64{uid, ownerID}, uid, map[int64]struct{}{uid: {}})
	if err != nil {
		return nil, err
	}
	result.SetUsers(users[1:])
	return &result, nil
}

func validateBotBusinessConnectionUpdates(updates *mtproto.Updates, connectionID string) (int64, error) {
	if updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates || len(updates.GetUpdates()) != 1 {
		return 0, mtproto.ErrInternalServerError
	}
	update := updates.GetUpdates()[0]
	if update == nil || update.GetPredicateName() != mtproto.Predicate_updateBotBusinessConnect || update.GetConnection() == nil {
		return 0, mtproto.ErrInternalServerError
	}
	connection := update.GetConnection()
	if connection.GetConnectionId() != connectionID || connection.GetUserId() <= 0 || connection.GetDcId() <= 0 || connection.GetDate() <= 0 || update.GetQts() <= 0 {
		return 0, mtproto.ErrInternalServerError
	}
	return connection.GetUserId(), nil
}

func businessConnectionUpdates(record domain.BusinessConnectionRecord) (*mtproto.Updates, error) {
	return businessConnectionUpdatesWithUsers(record, nil)
}

func businessConnectionUpdatesWithUsers(record domain.BusinessConnectionRecord, users []*mtproto.User) (*mtproto.Updates, error) {
	if record.ConnectionID == "" || record.OwnerUserID <= 0 || record.BotUserID <= 0 || record.DCID <= 0 || record.Date <= 0 || record.QTS <= 0 || len(record.Rights) == 0 {
		return nil, mtproto.ErrInternalServerError
	}
	rights := mtproto.MakeTLBusinessBotRights(&mtproto.BusinessBotRights{}).To_BusinessBotRights()
	if err := json.Unmarshal(record.Rights, rights); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	connection := mtproto.MakeTLBotBusinessConnection(&mtproto.BotBusinessConnection{
		Disabled: record.Disabled, ConnectionId: record.ConnectionID,
		UserId: record.OwnerUserID, DcId: record.DCID, Date: record.Date,
		Rights: rights, CanReply: record.CanReply,
	}).To_BotBusinessConnection()
	update := mtproto.MakeTLUpdateBotBusinessConnect(&mtproto.Update{
		Connection: connection, Qts: record.QTS,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdatesUsers(users, update), nil
}

func (c *ApiFullCore) AccountToggleConnectedBotPaused(in *mtproto.TLAccountToggleConnectedBotPaused) (*mtproto.Bool, error) {
	if err := c.requirePremium(); err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
	}
	peerType, peerID, err := connectedBotPeer(peer)
	if err != nil {
		return nil, err
	}
	paused := false
	var peerUser, peerChat, peerChannel int64
	if in.GetPaused() != nil {
		paused = mtproto.FromBool(in.GetPaused())
	}
	if peer != nil {
		peerUser, peerChat, peerChannel = peer.GetUserId(), peer.GetChatId(), peer.GetChannelId()
	}
	if domain.Ready() {
		if err := domain.SetConnectedBotPeerState(c.MD.UserId, peerType, peerID, &paused, nil); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
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
	if err := c.requirePremium(); err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
	}
	peerType, peerID, err := connectedBotPeer(peer)
	if err != nil {
		return nil, err
	}
	var peerUser, peerChat, peerChannel int64
	if peer != nil {
		peerUser, peerChat, peerChannel = peer.GetUserId(), peer.GetChatId(), peer.GetChannelId()
	}
	if domain.Ready() {
		disabled := true
		if err := domain.SetConnectedBotPeerState(c.MD.UserId, peerType, peerID, nil, &disabled); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
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
