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

/*
   private int getContactsHash(ArrayList<TLRPC.TL_contact> contacts) {
       long acc = 0;
       contacts = new ArrayList<>(contacts);
       Collections.sort(contacts, (tl_contact, tl_contact2) -> {
           if (tl_contact.user_id > tl_contact2.user_id) {
               return 1;
           } else if (tl_contact.user_id < tl_contact2.user_id) {
               return -1;
           }
           return 0;
       });
       int count = contacts.size();
       for (int a = -1; a < count; a++) {
           if (a == -1) {
               acc = ((acc * 20261) + 0x80000000L + getUserConfig().contactsSavedCount) % 0x80000000L;
           } else {
               TLRPC.TL_contact set = contacts.get(a);
               acc = ((acc * 20261) + 0x80000000L + set.user_id) % 0x80000000L;
           }
       }
       return (int) acc;
   }
*/

// ContactsGetContacts
// contacts.getContacts#5dd69e12 hash:long = contacts.Contacts;
func (c *ContactsCore) ContactsGetContacts(in *mtproto.TLContactsGetContacts) (*mtproto.Contacts_Contacts, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	contactList, err := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("contacts.getContacts - user.getContactList error: %v", err)
		return nil, err
	}
	if contactList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	// 避免查询数据库时IN()条件为empty
	idList := make([]int64, 0, len(contactList.GetDatas()))
	for _, contact := range contactList.GetDatas() {
		if contact == nil || contact.GetContactUserId() <= 0 {
			return nil, mtproto.ErrContactIdInvalid
		}
		idList = append(idList, contact.GetContactUserId())
	}

	if len(idList) == 0 {
		return mtproto.MakeTLContactsContacts(&mtproto.Contacts_Contacts{
			Contacts:   []*mtproto.Contact{},
			SavedCount: 0,
			Users:      []*mtproto.User{},
		}).To_Contacts_Contacts(), nil
	}

	c.Logger.Infof("contactIdList - {%v}", idList)
	users, err := c.svcCtx.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: append([]int64{c.MD.UserId}, idList...),
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		c.Logger.Errorf("contacts.getContacts - user.getMutableUsers error: %v", err)
		return nil, err
	}
	if users == nil {
		return nil, mtproto.ErrInternalServerError
	}
	for _, user := range users.GetDatas() {
		if user == nil || user.GetUser() == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}
	if !users.CheckExistUser(append([]int64{c.MD.UserId}, idList...)...) {
		return nil, mtproto.ErrInternalServerError
	}

	return mtproto.MakeTLContactsContacts(&mtproto.Contacts_Contacts{
		Contacts:   contactList.ToContacts(),
		SavedCount: 0,
		Users:      users.GetUserListByIdList(c.MD.UserId, idList...),
	}).To_Contacts_Contacts(), nil
}
