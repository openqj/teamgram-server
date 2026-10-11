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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCDeepLinksServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesStartBot(in *mtproto.TLMessagesStartBot) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || in.GetPeer() == nil || in.GetRandomId() == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if !validBotStartParam(in.GetStartParam()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	bot := in.GetBot()
	peer := in.GetPeer()
	if bot.GetPredicateName() != mtproto.Predicate_inputUser || bot.GetUserId() <= 0 || bot.GetAccessHash() == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil || d.ScheduledMessageSender == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := d.UserClient.UserGetImmutableUser(callContext(c), &userpb.TLUserGetImmutableUser{Id: bot.GetUserId()})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil || profile.Deleted() {
		return nil, mtproto.ErrBotInvalid
	}
	if profile.GetUser().GetAccessHash() != bot.GetAccessHash() {
		return nil, mtproto.ErrUserIdInvalid
	}

	peerType, peerID, err := c.validateBotStartPeer(userID, bot.GetUserId(), bot.GetAccessHash(), peer)
	if err != nil {
		return nil, err
	}

	message := "/start"
	if in.GetStartParam() != "" {
		message += " " + in.GetStartParam()
	}
	updates, err := d.ScheduledMessageSender.MsgSendMessageV2(callContext(c), &msgpb.TLMsgSendMessageV2{
		UserId:    userID,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peerType,
		PeerId:    peerID,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage: true,
			RandomId:  in.GetRandomId(),
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Out:     true,
				FromId:  mtproto.MakePeerUser(userID),
				PeerId:  startBotPeer(peerType, peerID),
				Date:    int32(time.Now().Unix()),
				Message: message,
			}).To_Message(),
		}).To_OutboxMessage()},
	})
	if err != nil {
		return nil, err
	}
	if updates == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return updates, nil
}

// validateBotStartPeer checks that a start command is sent only to a peer
// where both the caller and target bot currently have an active membership.
// User peers are restricted to the bot itself; group membership is resolved
// by the authoritative Chat service before MsgSendMessageV2 is called.
func (c *ApiFullCore) validateBotStartPeer(callerID, botID, botAccessHash int64, peer *mtproto.InputPeer) (int32, int64, error) {
	if peer == nil {
		return 0, 0, mtproto.ErrInputRequestInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() != botID || peer.GetAccessHash() == 0 || peer.GetAccessHash() != botAccessHash {
			return 0, 0, mtproto.ErrUserIdInvalid
		}
		return mtproto.PEER_USER, botID, nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		d := c.apifullDao()
		if d == nil || d.ChatClient == nil {
			return 0, 0, mtproto.ErrMethodNotImpl
		}
		chat, err := d.ChatClient.ChatGetMutableChat(callContext(c), &chatpb.TLChatGetMutableChat{ChatId: peer.GetChatId()})
		if err != nil {
			return 0, 0, err
		}
		if chat == nil || chat.GetChat() == nil || chat.GetChat().GetId() != peer.GetChatId() {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		callerMember, botMember := false, false
		for _, participant := range chat.GetChatParticipants() {
			if participant == nil || !participant.IsChatMemberStateNormal() {
				continue
			}
			switch participant.GetUserId() {
			case callerID:
				callerMember = true
			case botID:
				botMember = true
			}
		}
		if !callerMember {
			return 0, 0, mtproto.ErrUserNotParticipant
		}
		if !botMember {
			return 0, 0, mtproto.ErrBotInvalid
		}
		return mtproto.PEER_CHAT, peer.GetChatId(), nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 || peer.GetAccessHash() == 0 {
			return 0, 0, mtproto.ErrChannelInvalid
		}
		// Channels are owned by the APIFull PostgreSQL domain. Do not fall
		// back to a synthetic peer when the authoritative store is absent.
		if !domain.Ready() {
			return 0, 0, mtproto.ErrMethodNotImpl
		}
		channel, ok, loadErr := domain.LoadChannel(peer.GetChannelId())
		if loadErr != nil {
			return 0, 0, mtproto.ErrInternalServerError
		}
		if !ok || channel.AccessHash != peer.GetAccessHash() {
			return 0, 0, mtproto.ErrChannelInvalid
		}
		callerMember, callerErr := domain.ChannelIsMember(peer.GetChannelId(), callerID)
		if callerErr != nil {
			return 0, 0, mtproto.ErrInternalServerError
		}
		if !callerMember {
			return 0, 0, mtproto.ErrUserNotParticipant
		}
		botMember, botErr := domain.ChannelIsMember(peer.GetChannelId(), botID)
		if botErr != nil {
			return 0, 0, mtproto.ErrInternalServerError
		}
		if !botMember {
			return 0, 0, mtproto.ErrBotInvalid
		}
		return mtproto.PEER_CHANNEL, peer.GetChannelId(), nil
	default:
		return 0, 0, mtproto.ErrMethodNotImpl
	}
}

func startBotPeer(peerType int32, peerID int64) *mtproto.Peer {
	if peerType == mtproto.PEER_CHAT {
		return mtproto.MakePeerChat(peerID)
	}
	if peerType == mtproto.PEER_CHANNEL {
		return mtproto.MakePeerChannel(peerID)
	}
	return mtproto.MakePeerUser(peerID)
}

func validBotStartParam(param string) bool {
	if len(param) > 64 {
		return false
	}
	for i := 0; i < len(param); i++ {
		ch := param[i]
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '_' && ch != '-' {
			return false
		}
	}
	return true
}

func (c *ApiFullCore) HelpGetRecentMeUrls(in *mtproto.TLHelpGetRecentMeUrls) (*mtproto.Help_RecentMeUrls, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	// There is no recent-me URL producer in the canonical user/chat services
	// yet.  An empty result is the protocol representation for a user with no
	// recorded recent links; do not invent URLs or entities locally.
	return mtproto.MakeTLHelpRecentMeUrls(&mtproto.Help_RecentMeUrls{
		Urls:  []*mtproto.RecentMeUrl{},
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Help_RecentMeUrls(), nil
}

func (c *ApiFullCore) HelpGetDeepLinkInfo(in *mtproto.TLHelpGetDeepLinkInfo) (*mtproto.Help_DeepLinkInfo, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPath() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	// Deep-link resolution belongs to the authoritative bot/link registry.
	// Until that registry is wired, report the standard empty constructor for
	// an otherwise valid path instead of returning an unrelated error or
	// fabricating a message preview.
	return mtproto.MakeTLHelpDeepLinkInfoEmpty(nil).To_Help_DeepLinkInfo(), nil
}
