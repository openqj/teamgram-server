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

// ContactsDeleteByPhones
// contacts.deleteByPhones#1013fd9e phones:Vector<string> = Bool;
func (c *ContactsCore) ContactsDeleteByPhones(in *mtproto.TLContactsDeleteByPhones) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if len(in.GetPhones()) == 0 {
		return mtproto.BoolTrue, nil
	}
	phoneSet := make(map[string]struct{}, len(in.GetPhones()))
	phones := make([]string, 0, len(in.GetPhones()))
	for _, phone := range in.GetPhones() {
		_, normalized, normalizeErr := phonenumber.CheckPhoneNumberInvalid(phone)
		if normalizeErr != nil {
			return nil, mtproto.ErrPhoneNumberInvalid
		}
		if _, ok := phoneSet[normalized]; !ok {
			phones = append(phones, normalized)
		}
		phoneSet[normalized] = struct{}{}
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	saved, err := loadSavedContacts(c.MD.UserId)
	if err != nil {
		c.Logger.Errorf("contacts.deleteByPhones - load saved contacts error: %v", err)
		return nil, err
	}

	contactList, err := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("contacts.deleteByPhones - get contact list error: %v", err)
		return nil, err
	}
	if contactList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	deletedContactIds := make(map[int64]struct{})
	for _, contact := range contactList.GetDatas() {
		if contact == nil || contact.GetContactUserId() == 0 {
			continue
		}
		_, phone, normalizeErr := phonenumber.CheckPhoneNumberInvalid(contact.GetPhone().GetValue())
		if normalizeErr != nil {
			continue
		}
		if _, ok := phoneSet[phone]; !ok {
			continue
		}
		if _, ok := deletedContactIds[contact.GetContactUserId()]; ok {
			continue
		}
		deleted, deleteErr := c.svcCtx.Dao.UserClient.UserDeleteContact(c.ctx, &userpb.TLUserDeleteContact{
			UserId: c.MD.UserId,
			Id:     contact.GetContactUserId(),
		})
		if deleteErr != nil {
			c.Logger.Errorf("contacts.deleteByPhones - delete contact %d error: %v", contact.GetContactUserId(), deleteErr)
			return nil, deleteErr
		}
		if !mtproto.FromBool(deleted) {
			return mtproto.BoolFalse, nil
		}
		deletedContactIds[contact.GetContactUserId()] = struct{}{}
	}

	for _, phone := range phones {
		deleted, deleteErr := c.svcCtx.Dao.UserClient.UserDeleteImportersByPhone(c.ctx, &userpb.TLUserDeleteImportersByPhone{
			Phone: phone,
		})
		if deleteErr != nil {
			c.Logger.Errorf("contacts.deleteByPhones - delete unregistered import for phone error: %v", deleteErr)
			return nil, deleteErr
		}
		if !mtproto.FromBool(deleted) {
			return mtproto.BoolFalse, nil
		}
	}

	remaining := make([]savedPhoneContact, 0, len(saved))
	for _, row := range saved {
		if _, ok := phoneSet[row.Phone]; !ok {
			remaining = append(remaining, row)
		}
	}

	if err = saveSavedContacts(c.MD.UserId, remaining); err != nil {
		c.Logger.Errorf("contacts.deleteByPhones - save contacts error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
