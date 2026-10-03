// Copyright 2024 Teamgram Authors
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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/status"
)

const outboxReadDateExpirePeriod = 7 * 24 * time.Hour

// MessagesGetOutboxReadDate
// messages.getOutboxReadDate#8c4bfe5d peer:InputPeer msg_id:int = OutboxReadDate;
func (c *MessagesCore) MessagesGetOutboxReadDate(in *mtproto.TLMessagesGetOutboxReadDate) (*mtproto.OutboxReadDate, error) {
	// Possible errors
	// Code	Type	Description
	// 400	MESSAGE_ID_INVALID	The provided message id is invalid.
	// 400	MESSAGE_NOT_READ_YET	The specified message wasn't read yet.
	// 400	MESSAGE_TOO_OLD	The message is too old, the requested information is not available.
	// 400	PEER_ID_INVALID	The provided peer id is invalid.
	// 403	USER_PRIVACY_RESTRICTED	The user's privacy settings do not allow you to do this.
	// 403	YOUR_PRIVACY_RESTRICTED	You cannot fetch the read date of this message because you have disallowed other users to do so for your messages; to fix, allow other users to see your exact last online date OR purchase a Telegram Premium subscription.
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}

	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if !peer.IsUser() || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType == mtproto.PEER_USER && peer.AccessHash == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peerUser, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id:      peer.PeerId,
		Privacy: true,
		Contacts: []int64{
			c.MD.UserId,
		},
	})
	if err != nil {
		c.Logger.Errorf("messages.getOutboxReadDate - peer user lookup error: %v", err)
		return nil, err
	}
	if peerUser == nil || peerUser.GetUser() == nil || peerUser.GetUser().GetId() != peer.PeerId {
		return nil, mtproto.ErrInternalServerError
	}
	if peerUser.GetUser().GetDeleted() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType == mtproto.PEER_USER && peerUser.GetUser().GetAccessHash() != peer.AccessHash {
		return nil, mtproto.ErrPeerIdInvalid
	}

	msgBox, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &message.TLMessageGetUserMessage{
		UserId: c.MD.UserId,
		Id:     in.GetMsgId(),
	})
	if err != nil {
		c.Logger.Errorf("messages.getOutboxReadDate - message lookup error: %v", err)
		return nil, err
	}
	if msgBox == nil || msgBox.GetMessage() == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if msgBox.GetUserId() != c.MD.UserId || !msgBox.IsOut(c.MD.UserId) || msgBox.GetMessageId() != in.GetMsgId() || msgBox.GetMessage().GetId() != in.GetMsgId() {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if msgBox.GetPeerType() != mtproto.PEER_USER || msgBox.GetPeerId() != peer.PeerId {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if time.Since(time.Unix(int64(msgBox.GetMessage().GetDate()), 0)) >= outboxReadDateExpirePeriod {
		return nil, status.Error(mtproto.ErrBadRequest, "MESSAGE_TOO_OLD")
	}

	peerPrivacyAllows, err := checkStatusTimestampPrivacy(peerUser, c.MD.UserId)
	if err != nil {
		return nil, err
	}
	if !peerPrivacyAllows {
		return nil, mtproto.ErrUserPrivacyRestricted
	}

	selfUser, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id:      c.MD.UserId,
		Privacy: true,
		Contacts: []int64{
			peer.PeerId,
		},
	})
	if err != nil {
		c.Logger.Errorf("messages.getOutboxReadDate - self user lookup error: %v", err)
		return nil, err
	}
	if selfUser == nil || selfUser.GetUser() == nil || selfUser.GetUser().GetId() != c.MD.UserId {
		return nil, mtproto.ErrInternalServerError
	}
	selfPrivacyAllows, err := checkStatusTimestampPrivacy(selfUser, peer.PeerId)
	if err != nil {
		return nil, err
	}
	if !selfPrivacyAllows {
		return nil, status.Error(mtproto.ErrForbidden, "YOUR_PRIVACY_RESTRICTED")
	}

	rList, err := c.svcCtx.Dao.MessageClient.MessageGetOutboxReadDate(c.ctx, &message.TLMessageGetOutboxReadDate{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		MsgId:    in.GetMsgId(),
	})
	if err != nil {
		c.Logger.Errorf("messages.getOutboxReadDate - read date lookup error: %v", err)
		return nil, err
	}
	if rList == nil {
		return nil, mtproto.ErrInternalServerError
	}
	readDates := rList.GetDatas()
	if len(readDates) == 0 {
		return nil, mtproto.ErrMessageNotReadYet
	}
	if len(readDates) != 1 || readDates[0] == nil || readDates[0].GetUserId() != peer.PeerId {
		return nil, mtproto.ErrInternalServerError
	}

	return mtproto.MakeTLOutboxReadDate(&mtproto.OutboxReadDate{
		Date: readDates[0].GetDate(),
	}).To_OutboxReadDate(), nil
}

func checkStatusTimestampPrivacy(user *mtproto.ImmutableUser, peerID int64) (bool, error) {
	if user == nil || user.GetUser() == nil {
		return false, mtproto.ErrInternalServerError
	}

	var privacyRules *mtproto.PrivacyKeyRules
	for _, rules := range user.GetKeysPrivacyRules() {
		if rules == nil {
			return false, mtproto.ErrInternalServerError
		}
		if rules.GetKey() == int32(mtproto.STATUS_TIMESTAMP) {
			if privacyRules != nil {
				return false, mtproto.ErrInternalServerError
			}
			privacyRules = rules
		}
	}
	if privacyRules == nil || len(privacyRules.GetRules()) == 0 {
		return false, mtproto.ErrInternalServerError
	}

	baseRules := 0
	for _, rule := range privacyRules.GetRules() {
		if rule == nil {
			return false, mtproto.ErrInternalServerError
		}
		switch rule.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowAll,
			mtproto.Predicate_privacyValueAllowContacts,
			mtproto.Predicate_privacyValueDisallowAll:
			baseRules++
		case mtproto.Predicate_privacyValueAllowUsers,
			mtproto.Predicate_privacyValueDisallowUsers:
		default:
			return false, mtproto.ErrMethodNotImpl
		}
	}

	if baseRules != 1 {
		return false, mtproto.ErrInternalServerError
	}
	return user.CheckPrivacy(mtproto.STATUS_TIMESTAMP, peerID), nil
}
