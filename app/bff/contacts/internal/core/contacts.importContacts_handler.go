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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

// savedPhoneContact is one row of contacts.getSaved.
// Unregistered phones are not listable through a user RPC, so import persists them here.
type savedPhoneContact struct {
	Phone     string `json:"phone"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Date      int32  `json:"date"`
}

func savedContactsKey(userId int64) string {
	return fmt.Sprintf("contacts:%d:saved", userId)
}

func loadSavedContacts(userId int64) ([]savedPhoneContact, error) {
	raw, err := persist.Default.Get(savedContactsKey(userId))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return []savedPhoneContact{}, nil
	}
	var list []savedPhoneContact
	if err = json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []savedPhoneContact{}
	}
	return normalizeSavedContacts(list), nil
}

func saveSavedContacts(userId int64, list []savedPhoneContact) error {
	if list == nil {
		list = []savedPhoneContact{}
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(savedContactsKey(userId), string(raw))
}

func mergeSavedContacts(list []savedPhoneContact, in []*mtproto.InputContact, now int32) []savedPhoneContact {
	list = normalizeSavedContacts(list)
	index := make(map[string]int, len(list))
	for i, row := range list {
		index[row.Phone] = i
	}
	for _, contact := range in {
		if contact == nil || contact.Phone == "" {
			continue
		}
		if i, ok := index[contact.Phone]; ok {
			list[i].FirstName = contact.FirstName
			list[i].LastName = contact.LastName
			continue
		}
		index[contact.Phone] = len(list)
		list = append(list, savedPhoneContact{
			Phone:     contact.Phone,
			FirstName: contact.FirstName,
			LastName:  contact.LastName,
			Date:      now,
		})
	}
	return list
}

func normalizeSavedContacts(list []savedPhoneContact) []savedPhoneContact {
	normalized := make([]savedPhoneContact, 0, len(list))
	index := make(map[string]int, len(list))
	for _, row := range list {
		if _, phone, err := phonenumber.CheckPhoneNumberInvalid(row.Phone); err == nil {
			row.Phone = phone
		}
		if i, ok := index[row.Phone]; ok {
			normalized[i].FirstName = row.FirstName
			normalized[i].LastName = row.LastName
			continue
		}
		index[row.Phone] = len(normalized)
		normalized = append(normalized, row)
	}
	return normalized
}

func emptyImportedContacts(r *userpb.UserImportedContacts) *mtproto.Contacts_ImportedContacts {
	imported := []*mtproto.ImportedContact{}
	popular := []*mtproto.PopularContact{}
	retry := []int64{}
	users := []*mtproto.User{}
	if r != nil {
		if r.GetImported() != nil {
			imported = r.GetImported()
		}
		if r.GetPopularInvites() != nil {
			popular = r.GetPopularInvites()
		}
		if r.GetRetryContacts() != nil {
			retry = r.GetRetryContacts()
		}
		if r.GetUsers() != nil {
			users = r.GetUsers()
		}
	}
	return mtproto.MakeTLContactsImportedContacts(&mtproto.Contacts_ImportedContacts{
		Imported:       imported,
		PopularInvites: popular,
		RetryContacts:  retry,
		Users:          users,
	}).To_Contacts_ImportedContacts()
}

func normalizeImportContacts(in *mtproto.TLContactsImportContacts) ([]*mtproto.InputContact, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}

	seenPhones := make(map[string]struct{}, len(in.GetContacts()))
	seenClientIDs := make(map[int64]struct{}, len(in.GetContacts()))
	contacts := make([]*mtproto.InputContact, 0, len(in.GetContacts()))
	for _, contact := range in.GetContacts() {
		if contact == nil || contact.GetPredicateName() != mtproto.Predicate_inputPhoneContact || strings.TrimSpace(contact.GetPhone()) == "" {
			return nil, mtproto.ErrInputRequestInvalid
		}
		_, phone, err := phonenumber.CheckPhoneNumberInvalid(contact.GetPhone())
		if err != nil {
			return nil, mtproto.ErrPhoneNumberInvalid
		}
		if _, ok := seenPhones[phone]; ok {
			return nil, mtproto.ErrInputRequestInvalid
		}
		seenPhones[phone] = struct{}{}
		if _, ok := seenClientIDs[contact.GetClientId()]; ok {
			return nil, mtproto.ErrInputRequestInvalid
		}
		seenClientIDs[contact.GetClientId()] = struct{}{}
		contacts = append(contacts, &mtproto.InputContact{
			PredicateName: contact.GetPredicateName(),
			Constructor:   contact.GetConstructor(),
			ClientId:      contact.GetClientId(),
			Phone:         phone,
			FirstName:     contact.GetFirstName(),
			LastName:      contact.GetLastName(),
			Note:          contact.GetNote(),
		})
	}
	return contacts, nil
}

// stableImportedContacts preserves the request order while validating that the
// User service returned a complete, unambiguous response. The service uses a
// phone-indexed map internally, so its wire order is not authoritative.
func stableImportedContacts(in []*mtproto.InputContact, r *userpb.UserImportedContacts) (*userpb.UserImportedContacts, error) {
	if r == nil {
		return nil, mtproto.ErrInternalServerError
	}

	inputClientIDs := make(map[int64]struct{}, len(in))
	for _, contact := range in {
		inputClientIDs[contact.GetClientId()] = struct{}{}
	}

	orderedImported := make([]*mtproto.ImportedContact, 0, len(r.GetImported()))
	importedByClientID := make(map[int64]*mtproto.ImportedContact, len(r.GetImported()))
	for _, imported := range r.GetImported() {
		if imported == nil || imported.GetUserId() <= 0 {
			return nil, mtproto.ErrInternalServerError
		}
		if _, ok := inputClientIDs[imported.GetClientId()]; !ok {
			return nil, mtproto.ErrInternalServerError
		}
		if _, ok := importedByClientID[imported.GetClientId()]; ok {
			return nil, mtproto.ErrInternalServerError
		}
		importedByClientID[imported.GetClientId()] = imported
	}
	for _, contact := range in {
		if imported, ok := importedByClientID[contact.GetClientId()]; ok {
			orderedImported = append(orderedImported, imported)
		}
	}

	orderedPopular := make([]*mtproto.PopularContact, 0, len(r.GetPopularInvites()))
	popularByClientID := make(map[int64]*mtproto.PopularContact, len(r.GetPopularInvites()))
	for _, popular := range r.GetPopularInvites() {
		if popular == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if _, ok := inputClientIDs[popular.GetClientId()]; !ok {
			return nil, mtproto.ErrInternalServerError
		}
		if _, ok := popularByClientID[popular.GetClientId()]; ok {
			return nil, mtproto.ErrInternalServerError
		}
		popularByClientID[popular.GetClientId()] = popular
	}
	for _, contact := range in {
		if popular, ok := popularByClientID[contact.GetClientId()]; ok {
			orderedPopular = append(orderedPopular, popular)
		}
	}

	retryByClientID := make(map[int64]struct{}, len(r.GetRetryContacts()))
	for _, clientID := range r.GetRetryContacts() {
		if _, ok := inputClientIDs[clientID]; !ok {
			return nil, mtproto.ErrInternalServerError
		}
		if _, ok := retryByClientID[clientID]; ok {
			return nil, mtproto.ErrInternalServerError
		}
		retryByClientID[clientID] = struct{}{}
	}
	orderedRetry := make([]int64, 0, len(r.GetRetryContacts()))
	for _, contact := range in {
		if _, ok := retryByClientID[contact.GetClientId()]; ok {
			orderedRetry = append(orderedRetry, contact.GetClientId())
		}
	}

	usersByID := make(map[int64]*mtproto.User, len(r.GetUsers()))
	for _, user := range r.GetUsers() {
		if user == nil || user.GetId() <= 0 {
			return nil, mtproto.ErrInternalServerError
		}
		if _, ok := usersByID[user.GetId()]; ok {
			return nil, mtproto.ErrInternalServerError
		}
		usersByID[user.GetId()] = user
	}
	orderedUsers := make([]*mtproto.User, 0, len(r.GetUsers()))
	seenUserIDs := make(map[int64]struct{}, len(r.GetUsers()))
	for _, imported := range orderedImported {
		if _, seen := seenUserIDs[imported.GetUserId()]; seen {
			continue
		}
		user, ok := usersByID[imported.GetUserId()]
		if !ok {
			return nil, mtproto.ErrInternalServerError
		}
		seenUserIDs[imported.GetUserId()] = struct{}{}
		orderedUsers = append(orderedUsers, user)
	}
	if len(orderedUsers) != len(usersByID) {
		return nil, mtproto.ErrInternalServerError
	}

	return &userpb.UserImportedContacts{
		Imported:       orderedImported,
		PopularInvites: orderedPopular,
		RetryContacts:  orderedRetry,
		Users:          orderedUsers,
		UpdateIdList:   r.GetUpdateIdList(),
	}, nil
}

// ContactsImportContacts
// contacts.importContacts#2c800be5 contacts:Vector<InputContact> = contacts.ImportedContacts;
func (c *ContactsCore) ContactsImportContacts(in *mtproto.TLContactsImportContacts) (*mtproto.Contacts_ImportedContacts, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	contacts, err := normalizeImportContacts(in)
	if err != nil {
		c.Logger.Errorf("contacts.importContacts - error: %v", err)
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	if len(contacts) == 0 {
		return mtproto.MakeTLContactsImportedContacts(&mtproto.Contacts_ImportedContacts{
			Imported:       []*mtproto.ImportedContact{},
			PopularInvites: []*mtproto.PopularContact{},
			RetryContacts:  []int64{},
			Users:          []*mtproto.User{},
		}).To_Contacts_ImportedContacts(), nil
	}

	imported, err := c.svcCtx.Dao.UserClient.UserImportContacts(c.ctx, &userpb.TLUserImportContacts{
		UserId:   c.MD.UserId,
		Contacts: contacts,
	})
	if err != nil {
		c.Logger.Errorf("contacts.importContacts - error: %v", err)
		return nil, err
	}
	imported, err = stableImportedContacts(contacts, imported)
	if err != nil {
		c.Logger.Errorf("contacts.importContacts - invalid user.importContacts response: %v", err)
		return nil, err
	}

	saved, err := loadSavedContacts(c.MD.UserId)
	if err != nil {
		c.Logger.Errorf("contacts.importContacts - error: %v", err)
		return nil, err
	}
	saved = mergeSavedContacts(saved, contacts, int32(time.Now().Unix()))
	if err = saveSavedContacts(c.MD.UserId, saved); err != nil {
		c.Logger.Errorf("contacts.importContacts - error: %v", err)
		return nil, err
	}

	return emptyImportedContacts(imported), nil
}
