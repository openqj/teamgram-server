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
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
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
	if peer.GetPredicateName() != mtproto.Predicate_inputPeerUser {
		return nil, mtproto.ErrMethodNotImpl
	}
	if peer.GetUserId() != bot.GetUserId() || peer.GetAccessHash() == 0 || peer.GetAccessHash() != bot.GetAccessHash() {
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

	message := "/start"
	if in.GetStartParam() != "" {
		message += " " + in.GetStartParam()
	}
	updates, err := d.ScheduledMessageSender.MsgSendMessageV2(callContext(c), &msgpb.TLMsgSendMessageV2{
		UserId:    userID,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_USER,
		PeerId:    bot.GetUserId(),
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage: true,
			RandomId:  in.GetRandomId(),
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Out:     true,
				FromId:  mtproto.MakePeerUser(userID),
				PeerId:  mtproto.MakePeerUser(bot.GetUserId()),
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
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) HelpGetDeepLinkInfo(in *mtproto.TLHelpGetDeepLinkInfo) (*mtproto.Help_DeepLinkInfo, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPath() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return nil, mtproto.ErrMethodNotImpl
}
