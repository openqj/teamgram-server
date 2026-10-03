/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/container2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

type (
	contactItem = dao.ContactItem
)

// UserImportContacts
// user.importContacts user_id:long contacts:Vector<InputContact> = UserImportedContacts;
func (c *UserCore) UserImportContacts(in *user.TLUserImportContacts) (*user.UserImportedContacts, error) {
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}

	contacts := in.GetContacts()
	items, itemByPhone, phones, err := prepareImportContacts(contacts)
	if err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return user.MakeTLUserImportedContacts(&user.UserImportedContacts{
			Imported:       []*mtproto.ImportedContact{},
			PopularInvites: []*mtproto.PopularContact{},
			RetryContacts:  []int64{},
			Users:          []*mtproto.User{},
			UpdateIdList:   []int64{},
		}).To_UserImportedContacts(), nil
	}

	myUserData := c.svcCtx.Dao.GetCacheUserData(c.ctx, in.GetUserId())
	if myUserData == nil {
		c.Logger.Errorf("UserImportContacts - myUserData == nil")
		return nil, mtproto.ErrInternalServerError
	}

	if _, err := c.svcCtx.Dao.UsersDAO.SelectUsersByPhoneListWithCB(c.ctx, phones, func(_ int, _ int, row *dataobject.UsersDO) {
		if row == nil {
			return
		}
		item, ok := itemByPhone[row.Phone]
		if !ok {
			return
		}
		item.Unregistered = false
		item.UserId = row.Id
		if container2.ContainsInt64(myUserData.ContactIdList, row.Id) {
			item.ContactId = row.Id
		}
		if container2.ContainsInt64(myUserData.ReverseContactIdList, row.Id) {
			item.ImportContactId = row.Id
		}
	}); err != nil {
		c.Logger.Errorf("UserImportContacts - select users by phone error: %v", err)
		return nil, err
	}

	importedContacts := make([]*mtproto.ImportedContact, 0, len(contacts))
	updateIDs := make([]int64, 0, len(contacts))
	userIDs := make([]int64, 0, len(contacts))
	for _, item := range items {
		if item.Unregistered {
			continue
		}
		contact := &dataobject.UserContactsDO{
			OwnerUserId:      in.GetUserId(),
			ContactUserId:    item.UserId,
			ContactPhone:     item.C.GetPhone(),
			ContactFirstName: item.C.GetFirstName(),
			ContactLastName:  item.C.GetLastName(),
			Mutual:           item.ImportContactId > 0,
			CloseFriend:      false,
			StoriesHidden:    false,
			IsDeleted:        false,
			Date2:            time.Now().Unix(),
		}

		if item.ContactId > 0 {
			if item.ImportContactId > 0 {
				updateIDs = append(updateIDs, item.ImportContactId)
			}
			if _, err := c.svcCtx.Dao.UserContactsDAO.UpdateContactName(c.ctx, contact.ContactFirstName, contact.ContactLastName, contact.OwnerUserId, contact.ContactUserId); err != nil {
				return nil, err
			}
		} else {
			if item.ImportContactId > 0 {
				if _, err := c.svcCtx.Dao.UserContactsDAO.UpdateMutual(c.ctx, true, contact.ContactUserId, contact.OwnerUserId); err != nil {
					return nil, err
				}
				updateIDs = append(updateIDs, item.ImportContactId)
			} else {
				if _, _, err := c.svcCtx.Dao.ImportedContactsDAO.InsertOrUpdate(c.ctx, &dataobject.ImportedContactsDO{
					UserId:         contact.ContactUserId,
					ImportedUserId: contact.OwnerUserId,
				}); err != nil {
					return nil, err
				}
			}
			if _, _, err := c.svcCtx.Dao.UserContactsDAO.InsertOrUpdate(c.ctx, contact); err != nil {
				return nil, err
			}
		}

		importedContacts = append(importedContacts, mtproto.MakeTLImportedContact(&mtproto.ImportedContact{
			UserId:   item.UserId,
			ClientId: item.C.GetClientId(),
		}).To_ImportedContact())
		userIDs = append(userIDs, item.UserId)
	}

	if err := c.svcCtx.Dao.ClearContactCaches(c.ctx, in.GetUserId(), userIDs...); err != nil {
		c.Logger.Errorf("UserImportContacts - clear contact cache error: %v", err)
		return nil, err
	}
	users, err := c.UserGetMutableUsers(&user.TLUserGetMutableUsers{Id: append(userIDs, in.GetUserId())})
	if err != nil {
		c.Logger.Errorf("UserImportContacts - get mutable users error: %v", err)
		return nil, err
	}
	if users == nil {
		return nil, mtproto.ErrInternalServerError
	}

	for _, item := range items {
		if !item.Unregistered {
			continue
		}
		if _, _, err := c.svcCtx.Dao.UnregisteredContactsDAO.InsertOrUpdate(c.ctx, &dataobject.UnregisteredContactsDO{
			Phone:           item.C.GetPhone(),
			ImporterUserId:  in.GetUserId(),
			ImportFirstName: item.C.GetFirstName(),
			ImportLastName:  item.C.GetLastName(),
		}); err != nil {
			c.Logger.Errorf("UserImportContacts - save unregistered contact error: %v", err)
			return nil, err
		}
	}
	popularInvites, err := c.getPopularInvites(items)
	if err != nil {
		return nil, err
	}

	return user.MakeTLUserImportedContacts(&user.UserImportedContacts{
		Imported:       importedContacts,
		PopularInvites: popularInvites,
		RetryContacts:  []int64{},
		Users:          users.GetUserListByIdList(in.GetUserId(), userIDs...),
		UpdateIdList:   updateIDs,
	}).To_UserImportedContacts(), nil
}

func prepareImportContacts(contacts []*mtproto.InputContact) ([]*contactItem, map[string]*contactItem, []string, error) {
	items := make([]*contactItem, 0, len(contacts))
	itemByPhone := make(map[string]*contactItem, len(contacts))
	seenClientIDs := make(map[int64]struct{}, len(contacts))
	phones := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		if contact == nil || contact.GetPredicateName() != mtproto.Predicate_inputPhoneContact || strings.TrimSpace(contact.GetPhone()) == "" {
			return nil, nil, nil, mtproto.ErrInputRequestInvalid
		}
		_, phone, err := phonenumber.CheckPhoneNumberInvalid(contact.GetPhone())
		if err != nil {
			return nil, nil, nil, err
		}
		contact.Phone = phone
		if _, ok := itemByPhone[phone]; ok {
			return nil, nil, nil, mtproto.ErrInputRequestInvalid
		}
		if _, ok := seenClientIDs[contact.GetClientId()]; ok {
			return nil, nil, nil, mtproto.ErrInputRequestInvalid
		}
		seenClientIDs[contact.GetClientId()] = struct{}{}
		item := &contactItem{Unregistered: true, C: contact}
		items = append(items, item)
		itemByPhone[phone] = item
		phones = append(phones, phone)
	}
	return items, itemByPhone, phones, nil
}

func (c *UserCore) getPopularInvites(items []*contactItem) ([]*mtproto.PopularContact, error) {
	phones := make([]string, 0, len(items))
	for _, item := range items {
		if item.Unregistered {
			phones = append(phones, item.C.GetPhone())
		}
	}
	if len(phones) == 0 {
		return []*mtproto.PopularContact{}, nil
	}

	importersByPhone, err := c.svcCtx.Dao.UnregisteredContactsDAO.SelectDistinctImporterCountsByPhoneList(c.ctx, phones)
	if err != nil {
		c.Logger.Errorf("UserImportContacts - select unregistered contact importers error: %v", err)
		return nil, err
	}

	return makePopularInvites(items, importersByPhone), nil
}

func makePopularInvites(items []*contactItem, importersByPhone map[string]int32) []*mtproto.PopularContact {
	popularInvites := make([]*mtproto.PopularContact, 0, len(items))
	for _, item := range items {
		if !item.Unregistered {
			continue
		}
		popularInvites = append(popularInvites, mtproto.MakeTLPopularContact(&mtproto.PopularContact{
			ClientId:  item.C.GetClientId(),
			Importers: importersByPhone[item.C.GetPhone()],
		}).To_PopularContact())
	}
	return popularInvites
}
