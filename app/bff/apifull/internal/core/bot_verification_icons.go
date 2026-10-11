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
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	apifullpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// RPCBotVerificationIconsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) BotsSetCustomVerification(in *mtproto.TLBotsSetCustomVerification) (*mtproto.Bool, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.verifyMiniBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	if botID != callerID {
		return nil, mtproto.ErrUserIdInvalid
	}
	if c.businessProviderConfigured() {
		var result struct {
			Value *bool `json:"value"`
		}
		if err = c.callBusinessProvider("bots.setCustomVerification", map[string]any{
			"enabled": in.GetEnabled(), "bot": in.GetBot(), "peer": in.GetPeer(), "custom_description": in.GetCustomDescription(),
		}, &result); err != nil {
			return nil, err
		}
		if result.Value == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if *result.Value {
			return mtproto.BoolTrue, nil
		}
		return mtproto.BoolFalse, nil
	}
	peerType, peerID, err := customVerificationPeer(callerID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	description := ""
	if value := in.GetCustomDescription(); value != nil {
		description = strings.TrimSpace(value.GetValue())
		if len(description) > 512 {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	state := botCustomVerificationState{Enabled: in.GetEnabled(), Description: description, UpdatedAt: time.Now().Unix()}
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = apifullpersist.Update(customVerificationKey(botID, peerType, peerID), func(string) (string, error) {
		return string(encoded), nil
	}); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

type botCustomVerificationState struct {
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	UpdatedAt   int64  `json:"updated_at"`
}

func customVerificationKey(botID int64, peerType int32, peerID int64) string {
	return apifullpersist.BotCustomVerificationKey(botID, peerType, peerID)
}

func customVerificationPeer(selfID int64, peer *mtproto.InputPeer) (int32, int64, error) {
	if !validThreadInputPeer(selfID, peer) {
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerUser:
		if peer.GetAccessHash() == 0 {
			return 0, 0, mtproto.ErrUserIdInvalid
		}
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetAccessHash() == 0 {
			return 0, 0, mtproto.ErrChannelInvalid
		}
	}
	peerType, peerID := apifullPeerTypeID(selfID, peer)
	if peerType == mtproto.PEER_USER && peerID == selfID {
		peerType = mtproto.PEER_SELF
	}
	switch peerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		if peerID > 0 {
			return peerType, peerID, nil
		}
	}
	return 0, 0, mtproto.ErrPeerIdInvalid
}

func (c *ApiFullCore) BotsGetBotRecommendations(in *mtproto.TLBotsGetBotRecommendations) (*mtproto.Users_Users, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.verifyMiniBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	result := new(mtproto.Users_Users)
	if err = c.callBusinessProvider("bots.getBotRecommendations", map[string]any{
		"caller_user_id": uid, "bot_id": botID,
	}, result); err != nil {
		return nil, err
	}
	if result == nil || len(result.GetUsers()) > 10000 {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}
