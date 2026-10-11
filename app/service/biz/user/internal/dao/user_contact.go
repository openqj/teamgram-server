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

package dao

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/marmota/pkg/container2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

const (
	contactKeyPrefix = "user_contact.2#"
)

var (
	GenContactCacheKey = genContactCacheKey
)

func getContactIdList(cached []int64, load func() ([]int64, error)) ([]int64, error) {
	if len(cached) > 0 {
		return cached, nil
	}

	return load()
}

type ContactItem struct {
	C               *mtproto.InputContact
	Unregistered    bool  // 未注册
	UserId          int64 // 已经注册的用户ID
	ContactId       int64 // 已经注册是我的联系人
	ImportContactId int64 // 已经注册的反向联系人
}

func genContactCacheKey(selfId, contactId int64) string {
	return fmt.Sprintf("%s%d_%d", contactKeyPrefix, selfId, contactId)
}

func isContactCacheKey(k string) bool {
	return strings.HasPrefix(k, contactKeyPrefix)
}

func parseContactCacheKey(k string) (int64, int64) {
	if strings.HasPrefix(k, contactKeyPrefix) {
		v := strings.Split(k[len(contactKeyPrefix):], "_")
		if len(v) != 2 {
			return 0, 0
		}
		v0, _ := strconv.ParseInt(v[0], 10, 64)
		v1, _ := strconv.ParseInt(v[1], 10, 64)

		return v0, v1
	}

	return 0, 0
}

func (d *Dao) GetUserContactList(ctx context.Context, id int64) ([]*mtproto.ContactData, error) {
	cacheUserData, err := d.GetCacheUserDataWithError(ctx, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var cachedIDs []int64
	if cacheUserData != nil {
		cachedIDs = cacheUserData.GetContactIdList()
	}
	idList, err := getContactIdList(cachedIDs, func() ([]int64, error) {
		return d.SelectUserContactIDs(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	if len(idList) == 0 {
		return nil, nil
	}

	return d.getContactListByIdList(ctx, id, idList)
}

func (d *Dao) GetUserContact(ctx context.Context, id, contactId int64) (*mtproto.ContactData, error) {
	contacts, err := d.GetUserContactListByIdList(ctx, id, contactId)
	if err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return nil, nil
	}

	return contacts[0], nil
}

func (d *Dao) GetUserContactListByIdList(ctx context.Context, id int64, contactId ...int64) ([]*mtproto.ContactData, error) {
	cacheUserData, err := d.GetCacheUserDataWithError(ctx, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var cachedIDs []int64
	if cacheUserData != nil {
		cachedIDs = cacheUserData.GetContactIdList()
	}
	idList, err := getContactIdList(cachedIDs, func() ([]int64, error) {
		return d.SelectUserContactIDs(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	if len(idList) == 0 {
		return nil, nil
	}

	idList2 := make([]int64, 0, len(idList))
	for _, id2 := range contactId {
		if ok := container2.ContainsInt64(idList, id2); ok {
			idList2 = append(idList2, id2)
		}
	}
	if len(idList2) == 0 {
		return nil, nil
	}

	return d.getContactListByIdList(ctx, id, idList2)
}

func (d *Dao) DeleteUserContact(ctx context.Context, id int64, contactId int64) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := d.Postgres.Store.Contacts.DeleteContactsTx(ctx, tx, id, []int64{contactId}); err != nil {
			return err
		}
		_, err := d.Postgres.Store.Contacts.UpdateMutualTx(ctx, tx, false, contactId, id)
		return err
	})
}

func (d *Dao) ResetUserContacts(ctx context.Context, userId int64) error {
	if userId <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE user_contacts SET is_deleted=TRUE,mutual=FALSE,close_friend=FALSE,stories_hidden=FALSE WHERE owner_user_id=$1 AND contact_user_id<>0 AND is_deleted=FALSE`, userId); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE user_contacts SET mutual=FALSE WHERE contact_user_id=$1 AND owner_user_id<>$1 AND is_deleted=FALSE`, userId); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE imported_contacts SET deleted=TRUE WHERE imported_user_id=$1 AND deleted=FALSE`, userId); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM unregistered_contacts WHERE importer_user_id=$1`, userId)
		return err
	})
}

func (d *Dao) PutUserContact(ctx context.Context, changeMutual bool, do *dataobject.UserContactsDO) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if changeMutual {
			if _, err := d.Postgres.Store.Contacts.UpdateMutualTx(ctx, tx, true, do.ContactUserId, do.OwnerUserId); err != nil {
				return err
			}
		}
		_, _, err := d.Postgres.Store.Contacts.InsertOrUpdateTx(ctx, tx, do)
		return err
	})
}

func (d *Dao) ClearContactCaches(ctx context.Context, userId int64, contactId ...int64) error {
	return nil
}

func (d *Dao) GetCloseFriendList(ctx context.Context, id int64) []*mtproto.ContactData {
	cacheUserData := d.GetCacheUserData(ctx, id)
	if cacheUserData == nil {
		return nil
	}
	if len(cacheUserData.GetContactIdList()) == 0 {
		return nil
	}

	return d.getCloseFriendListByIdList(ctx, id, cacheUserData.GetContactIdList())
}

func (d *Dao) getCloseFriendListByIdList(ctx context.Context, id int64, idList []int64) []*mtproto.ContactData {
	contacts, err := d.Postgres.Store.Contacts.SelectListByIdList(ctx, id, idList)
	if err != nil {
		return nil
	}
	result := make([]*mtproto.ContactData, 0, len(contacts))
	for _, do := range contacts {
		if do.CloseFriend {
			result = append(result, mtproto.MakeTLContactData(&mtproto.ContactData{UserId: id, ContactUserId: do.ContactUserId, FirstName: mtproto.MakeFlagsString(do.ContactFirstName), LastName: mtproto.MakeFlagsString(do.ContactLastName), MutualContact: do.Mutual, Phone: mtproto.MakeFlagsString(do.ContactPhone), CloseFriend: do.CloseFriend}).To_ContactData())
		}
	}
	return result
}

func (d *Dao) getContactListByIdList(ctx context.Context, id int64, idList []int64) ([]*mtproto.ContactData, error) {
	var (
		cKeys = make([]string, 0, len(idList))
	)

	for _, v := range idList {
		cKeys = append(cKeys, genContactCacheKey(id, v))
	}

	return d.getContactListByKeyList(ctx, cKeys...)
}

func (d *Dao) getReverseContactListByIdList(ctx context.Context, id int64, idList []int64) ([]*mtproto.ContactData, error) {
	var (
		cKeys = make([]string, 0, len(idList))
	)

	for _, v := range idList {
		cKeys = append(cKeys, genContactCacheKey(v, id))
	}

	return d.getContactListByKeyList(ctx, cKeys...)
}

func (d *Dao) getContactListByKeyList(ctx context.Context, cKeys ...string) ([]*mtproto.ContactData, error) {
	if len(cKeys) == 0 {
		return []*mtproto.ContactData{}, nil
	}
	result := make([]*mtproto.ContactData, 0, len(cKeys))
	for _, key := range cKeys {
		id0, id1 := parseContactCacheKey(key)
		contact, err := d.Postgres.Store.Contacts.SelectContact(ctx, id0, id1)
		if err != nil {
			return nil, err
		}
		if contact == nil {
			continue
		}
		result = append(result, mtproto.MakeTLContactData(&mtproto.ContactData{UserId: contact.OwnerUserId, ContactUserId: contact.ContactUserId, FirstName: mtproto.MakeFlagsString(contact.ContactFirstName), LastName: mtproto.MakeFlagsString(contact.ContactLastName), MutualContact: contact.Mutual, Phone: mtproto.MakeFlagsString(contact.ContactPhone), CloseFriend: contact.CloseFriend}).To_ContactData())
	}
	return result, nil
}
