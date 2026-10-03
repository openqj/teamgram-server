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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsAcceptContact
// contacts.acceptContact#f831a20f id:InputUser = Updates;
func (c *ContactsCore) ContactsAcceptContact(in *mtproto.TLContactsAcceptContact) (*mtproto.Updates, error) {
	if in.GetId() == nil {
		err := mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}

	id := mtproto.FromInputUser(c.MD.UserId, in.Id)
	if !id.IsUser() || id.IsSelf() || id.PeerId == c.MD.UserId {
		err := mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}

	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      []int64{c.MD.UserId, id.PeerId},
		Privacy: true,
		HasTo:   true,
		To:      []int64{id.PeerId},
	})
	if err != nil {
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}
	if !users.CheckExistUser(c.MD.UserId, id.PeerId) {
		err = mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}

	peerUser, ok := users.GetImmutableUser(id.PeerId)
	if !ok || peerUser == nil || peerUser.Deleted() {
		err = mtproto.ErrContactIdInvalid
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}

	firstName := peerUser.FirstName()
	lastName := peerUser.LastName()
	if firstName == "" && lastName == "" {
		firstName = peerUser.Username()
	}
	if firstName == "" && lastName == "" {
		err = mtproto.ErrContactNameEmpty
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}
	phone := peerUser.Phone()

	changeMutual, err := c.svcCtx.Dao.UserClient.UserAddContact(c.ctx, &userpb.TLUserAddContact{
		UserId:                   c.MD.UserId,
		AddPhonePrivacyException: mtproto.ToBool(false),
		Id:                       id.PeerId,
		FirstName:                firstName,
		LastName:                 lastName,
		Phone:                    phone,
	})
	if err != nil {
		c.Logger.Errorf("contacts.acceptContact - error: %v", err)
		return nil, err
	}

	cUser, _ := users.GetUnsafeUser(c.MD.UserId, id.PeerId)
	cUser.Contact = true
	cUser.MutualContact = mtproto.FromBool(changeMutual)
	cUser.FirstName = mtproto.MakeFlagsString(firstName)
	cUser.LastName = mtproto.MakeFlagsString(lastName)
	cUser.Phone = mtproto.MakeFlagsString(phone)

	me, _ := users.GetUnsafeUserSelf(c.MD.UserId)

	return mtproto.MakeUpdatesByUpdatesUsers(
		[]*mtproto.User{me, cUser},
		mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{
			Peer_PEER: id.ToPeer(),
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
		}).To_Update()), nil
}
