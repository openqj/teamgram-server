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
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

// ContactsResolvePhone
// contacts.resolvePhone#8af94344 phone:string = contacts.ResolvedPeer;
func (c *UsersCore) ContactsResolvePhone(in *mtproto.TLContactsResolvePhone) (*mtproto.Contacts_ResolvedPeer, error) {
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, phone, err := phonenumber.CheckPhoneNumberInvalid(in.GetPhone())
	if err != nil {
		return nil, err
	}

	id, err := c.svcCtx.Dao.UserClient.UserGetUserIdByPhone(c.ctx, &userpb.TLUserGetUserIdByPhone{
		Phone: phone,
	})
	if err != nil {
		c.Logger.Errorf("contacts.resolvePhone - error: %v", err)
		return nil, err
	}

	if id == nil || id.GetV() <= 0 {
		return nil, mtproto.ErrPhoneNotOccupied
	}

	contactList, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      []int64{id.GetV(), c.MD.UserId},
		Privacy: true,
		HasTo:   true,
		To:      []int64{c.MD.UserId},
	})
	if err != nil {
		c.Logger.Errorf("contacts.resolvePhone - error: %v", err)
		return nil, err
	}
	if contactList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	me, _ := contactList.GetImmutableUser(c.MD.UserId)
	resolved, _ := contactList.GetImmutableUser(id.GetV())

	if me == nil || resolved == nil {
		err = mtproto.ErrInternalServerError
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	}

	allowed, err := c.svcCtx.Dao.UserClient.UserCheckPrivacy(c.ctx, &userpb.TLUserCheckPrivacy{
		UserId:  id.GetV(),
		KeyType: mtproto.ADDED_BY_PHONE,
		PeerId:  c.MD.UserId,
	})
	if err != nil {
		return nil, err
	}
	if allowed == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if !mtproto.FromBool(allowed) {
		return nil, mtproto.ErrPhoneNotOccupied
	}

	return mtproto.MakeTLContactsResolvedPeer(&mtproto.Contacts_ResolvedPeer{
		Peer:  mtproto.MakePeerUser(id.GetV()),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{resolved.ToUnsafeUser(me)},
	}).To_Contacts_ResolvedPeer(), nil
}
