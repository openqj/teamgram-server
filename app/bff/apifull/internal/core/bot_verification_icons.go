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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	apifullpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/status"
)

// RPCBotVerificationIconsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const customVerificationDescriptionLimit = 70

var errBotVerifierForbidden = status.Error(mtproto.ErrForbidden, "BOT_VERIFIER_FORBIDDEN")

func (c *ApiFullCore) BotsSetCustomVerification(in *mtproto.TLBotsSetCustomVerification) (*mtproto.Bool, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.resolveCustomVerificationBot(callerID, in.GetBot())
	if err != nil {
		return nil, err
	}
	peerType, peerID, err := c.resolveCustomVerificationPeer(callerID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	description := ""
	if value := in.GetCustomDescription(); value != nil {
		description = value.GetValue()
		if len([]byte(description)) > customVerificationDescriptionLimit {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	var localResult struct {
		Value *bool `json:"value"`
	}
	if local, loadErr := c.loadLocalBusinessResultForCaller("bots.setCustomVerification", fmt.Sprintf("bot:%d:peer:%d:%d", botID, peerType, peerID), &localResult); loadErr != nil {
		return nil, loadErr
	} else if local {
		if localResult.Value == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if *localResult.Value {
			return mtproto.BoolTrue, nil
		}
		return mtproto.BoolFalse, nil
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
	// A botVerifierSettings record is the authority that supplies the icon,
	// organization and custom-description permission. The current User service
	// does not expose that authority yet, so never persist a local success.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) resolveCustomVerificationBot(callerID int64, input *mtproto.InputUser) (int64, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	caller, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: callerID})
	if err != nil {
		return 0, err
	}
	if caller == nil || caller.GetUser() == nil || caller.Deleted() {
		return 0, mtproto.ErrUserIdInvalid
	}
	callerIsBot := caller.GetUser().GetBot() != nil
	if input == nil {
		if !callerIsBot {
			return 0, mtproto.ErrBotInvalid
		}
		return callerID, nil
	}
	if callerIsBot {
		// The TL contract requires bot callers to omit the optional bot field.
		return 0, mtproto.ErrBotInvalid
	}
	botID, err := c.verifyMiniBot(input)
	if err != nil {
		return 0, err
	}
	bot, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: botID})
	if err != nil {
		return 0, err
	}
	if bot == nil || bot.GetUser() == nil || bot.GetUser().GetBot() == nil || bot.Deleted() {
		return 0, mtproto.ErrBotInvalid
	}
	if bot.GetUser().GetBot().GetCreator() != callerID {
		return 0, errBotVerifierForbidden
	}
	return botID, nil
}

func (c *ApiFullCore) resolveCustomVerificationPeer(selfID int64, peer *mtproto.InputPeer) (int32, int64, error) {
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf, mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerChat, mtproto.Predicate_inputPeerChannel:
		// These constructors carry enough canonical identity for the authority
		// lookups below. Username and FromMessage peers need a resolver/context
		// that this mutation does not have.
	default:
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
	peerType, peerID, err := customVerificationPeer(selfID, peer)
	if err != nil {
		return 0, 0, err
	}
	switch peerType {
	case mtproto.PEER_SELF:
		return peerType, peerID, nil
	case mtproto.PEER_USER:
		if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
			return 0, 0, mtproto.ErrMethodNotImpl
		}
		profile, getErr := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: peerID})
		if getErr != nil {
			return 0, 0, getErr
		}
		if profile == nil || profile.GetUser() == nil || profile.Deleted() || profile.GetUser().GetAccessHash() != peer.GetAccessHash() {
			return 0, 0, mtproto.ErrUserIdInvalid
		}
		return peerType, peerID, nil
	case mtproto.PEER_CHAT:
		if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
			return 0, 0, mtproto.ErrMethodNotImpl
		}
		chat, getErr := c.svcCtx.Dao.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peerID})
		if getErr != nil {
			return 0, 0, getErr
		}
		if chat == nil || chat.GetChat() == nil || chat.GetChat().GetId() != peerID {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		return peerType, peerID, nil
	case mtproto.PEER_CHANNEL:
		channel, found, getErr := domain.LoadChannel(peerID)
		if getErr != nil {
			return 0, 0, getErr
		}
		if !found || channel.AccessHash != peer.GetAccessHash() {
			return 0, 0, mtproto.ErrChannelInvalid
		}
		return peerType, peerID, nil
	default:
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
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
	if local, loadErr := c.loadLocalBusinessResult("bots.getBotRecommendations", fmt.Sprintf("bot:%d", botID), result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if result == nil || len(result.GetUsers()) > 10000 {
			return nil, mtproto.ErrInternalServerError
		}
		return result, nil
	}
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
