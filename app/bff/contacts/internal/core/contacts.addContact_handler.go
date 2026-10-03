// Copyright 2022 Teamgram Authors
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
	"github.com/teamgram/proto/mtproto"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsAddContact
/***
	## contacts.addContact
	Add an existing telegram user as contact.

	Use contacts.importContacts to add contacts by phone number, without knowing their Telegram ID.
**/
// contacts.addContact#e8f463d0 flags:# add_phone_privacy_exception:flags.0?true id:InputUser first_name:string last_name:string phone:string = Updates;
func (c *ContactsCore) ContactsAddContact(in *mtproto.TLContactsAddContact) (*mtproto.Updates, error) {
	// 400	CONTACT_NAME_EMPTY	Contact name empty.
	if in.FirstName == "" && in.LastName == "" {
		err := mtproto.ErrContactNameEmpty
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		return nil, err
	}

	if in.GetId() == nil {
		return nil, mtproto.ErrContactIdInvalid
	}
	var userId int64
	if in.Id.GetPredicateName() == mtproto.Predicate_inputUserFromMessage {
		var err error
		userId, err = c.resolveUserFromMessage(in.Id)
		if err != nil {
			c.Logger.Errorf("contacts.addContact - error: %v", err)
			return nil, mtproto.ErrContactIdInvalid
		}
	} else {
		id := mtproto.FromInputUser(c.MD.UserId, in.Id)
		if !id.IsUser() || id.IsSelf() {
			err := mtproto.ErrContactIdInvalid
			c.Logger.Errorf("contacts.addContact - error: %v", err)
			return nil, err
		}
		userId = id.PeerId
	}
	if userId <= 0 || userId == c.MD.UserId {
		err := mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		return nil, err
	}

	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      []int64{c.MD.UserId, userId},
		Privacy: true,
		HasTo:   true,
		To:      []int64{userId},
	})
	if err != nil {
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		err = mtproto.ErrContactIdInvalid
		return nil, err
	}

	if users == nil || !users.CheckExistUser(c.MD.UserId, userId) {
		err = mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		return nil, err
	}

	changeMutual, err := c.svcCtx.Dao.UserClient.UserAddContact(c.ctx, &userpb.TLUserAddContact{
		UserId:                   c.MD.UserId,
		AddPhonePrivacyException: mtproto.ToBool(in.AddPhonePrivacyException),
		Id:                       userId,
		FirstName:                in.FirstName,
		LastName:                 in.LastName,
		Phone:                    in.Phone,
	})
	if err != nil {
		c.Logger.Errorf("contacts.addContact - error: %v", err)
		err = mtproto.ErrContactIdInvalid
		return nil, err
	}

	cUser, _ := users.GetUnsafeUser(c.MD.UserId, userId)
	cUser.Contact = true
	cUser.MutualContact = mtproto.FromBool(changeMutual)
	cUser.FirstName = mtproto.MakeFlagsString(in.FirstName)
	cUser.LastName = mtproto.MakeFlagsString(in.LastName)
	cUser.Phone = mtproto.MakeFlagsString(in.Phone)

	me, _ := users.GetUnsafeUserSelf(c.MD.UserId)

	// TODO(@benqi): 性能优化，复用users
	rUpdates := mtproto.MakeUpdatesByUpdatesUsers(
		[]*mtproto.User{me, cUser},
		mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{
			Peer_PEER: mtproto.MakePeerUser(userId),
			Settings: mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{
				ReportSpam:             false,
				AddContact:             false,
				BlockContact:           false,
				ShareContact:           false,
				NeedContactsException:  false,
				ReportGeo:              false,
				Autoarchived:           false,
				InviteMembers:          false,
				RequestChatBroadcast:   false,
				BusinessBotPaused:      false,
				BusinessBotCanReply:    false,
				GeoDistance:            nil,
				RequestChatTitle:       nil,
				RequestChatDate:        nil,
				BusinessBotId:          nil,
				BusinessBotManageUrl:   nil,
				ChargePaidMessageStars: nil,
				RegistrationMonth:      nil,
				PhoneCountry:           nil,
				NameChangeDate:         nil,
				PhotoChangeDate:        nil,
			}).To_PeerSettings(),
		}).To_Update())

	return rUpdates, nil
}

func (c *ContactsCore) resolveUserFromMessage(in *mtproto.InputUser) (int64, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetMsgId() <= 0 || in.GetPeer() == nil {
		return 0, mtproto.ErrContactIdInvalid
	}
	peer := in.GetPeer()
	var peerType int32
	var peerId int64
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerUser:
		peerType, peerId = mtproto.PEER_USER, peer.GetUserId()
	case mtproto.Predicate_inputPeerChat:
		peerType, peerId = mtproto.PEER_CHAT, peer.GetChatId()
	case mtproto.Predicate_inputPeerChannel:
		peerType, peerId = mtproto.PEER_CHANNEL, peer.GetChannelId()
	default:
		return 0, mtproto.ErrContactIdInvalid
	}
	if peerId <= 0 || c.svcCtx.Dao.MessageClient == nil {
		return 0, mtproto.ErrContactIdInvalid
	}

	box, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &messagepb.TLMessageGetUserMessage{
		UserId: c.MD.UserId,
		Id:     in.GetMsgId(),
	})
	if err != nil || box == nil || box.GetMessage() == nil || box.GetUserId() != c.MD.UserId || box.GetMessageId() != in.GetMsgId() {
		return 0, mtproto.ErrContactIdInvalid
	}
	if box.GetPeerType() != peerType || box.GetPeerId() != peerId {
		return 0, mtproto.ErrContactIdInvalid
	}

	// A private peer is the other user in that dialog; in group and channel
	// contexts, only the author of the referenced message can be resolved.
	userMatchesContext := peerType == mtproto.PEER_USER && peerId == in.GetUserId()
	if peerType != mtproto.PEER_USER {
		userMatchesContext = box.GetSenderUserId() == in.GetUserId()
	}
	if userMatchesContext {
		return in.GetUserId(), nil
	}
	return 0, mtproto.ErrContactIdInvalid
}
