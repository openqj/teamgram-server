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
	"math/rand"
	"time"

	"github.com/teamgram/marmota/pkg/container2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

func (d *Dao) getBotData(ctx context.Context, botId int64) (*mtproto.BotData, bool, int64, error) {
	var (
		botData *mtproto.BotData
	)

	botDO, err := d.SelectBot(ctx, botId)
	if err != nil {
		return nil, false, 0, err
	}
	if botDO != nil {
		// userData.Bot
		botData = mtproto.MakeTLBotData(&mtproto.BotData{
			Id:                   botDO.BotId,
			BotType:              botDO.BotType,
			Creator:              botDO.CreatorUserId,
			Description:          botDO.Description,
			BotChatHistory:       botDO.BotChatHistory,
			BotNochats:           botDO.BotNochats,
			BotInlineGeo:         botDO.BotInlineGeo,
			BotInfoVersion:       botDO.BotInfoVersion,
			BotInlinePlaceholder: mtproto.MakeFlagsString(botDO.BotInlinePlaceholder),
			BotAttachMenu:        botDO.BotAttachMenu,
			AttachMenuEnabled:    botDO.AttachMenuEnabled,
			BotCanEdit:           botDO.BotCanEdit,
			BotBusiness:          botDO.BotBusiness,
			BotHasMainApp:        botDO.BotHasMainApp,
			BotActiveUsers:       mtproto.MakeFlagsInt32(botDO.BotActiveUsers),
		}).To_BotData()
		return botData, botDO.BotCanManageBots, botDO.ManagerBotId, nil
	}

	return nil, false, 0, nil
}

func (d *Dao) CreateNewUserV2(
	ctx context.Context,
	secretKeyId int64,
	phone string,
	countryCode string,
	firstName string, lastName string) (*mtproto.ImmutableUser, error) {
	var (
		//err    error
		userDO        *dataobject.UsersDO
		now           = time.Now().Unix()
		cacheUserData = NewCacheUserData()
	)

	userDO = &dataobject.UsersDO{
		UserType:       user.UserTypeRegular,
		AccessHash:     rand.Int63(),
		Phone:          phone,
		SecretKeyId:    secretKeyId,
		FirstName:      firstName,
		LastName:       lastName,
		CountryCode:    countryCode,
		AccountDaysTtl: 548,
	}
	if err := d.pgCreateNewUser(ctx, userDO, now, 300); err != nil {
		return nil, err
	}
	cacheUserData.UserData = d.MakeUserDataByDO(userDO)
	cacheUserData.CachesPrivacyKeyRules = append(
		cacheUserData.CachesPrivacyKeyRules,
		mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
			Key:   mtproto.STATUS_TIMESTAMP,
			Rules: defaultRules,
		}).To_PrivacyKeyRules(),
		mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
			Key:   mtproto.PHONE_NUMBER,
			Rules: phoneNumberRules,
		}).To_PrivacyKeyRules(),
		mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
			Key:   mtproto.PROFILE_PHOTO,
			Rules: defaultRules,
		}).To_PrivacyKeyRules())

	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User:             cacheUserData.UserData,
		LastSeenAt:       now,
		Contacts:         nil,
		KeysPrivacyRules: nil,
	}).To_ImmutableUser(), nil
}

func (d *Dao) UpdateUserFirstAndLastName(ctx context.Context, id int64, firstName, lastName string) error {
	_, err := d.UpdateUserFields(ctx, id, map[string]any{"first_name": firstName, "last_name": lastName})
	return err
}

func (d *Dao) UpdateUserAbout(ctx context.Context, id int64, about string) error {
	_, err := d.UpdateUserFields(ctx, id, map[string]any{"about": about})
	return err
}

func (d *Dao) UpdateUserUsername(ctx context.Context, id int64, username string) error {
	return d.pgUpdateUsername(ctx, id, username)
}

//func (d *Dao) DeleteProfilePhoto(ctx context.Context, userId, photoId int64) int64 {
//}
//
//func (d *Dao) DeleteMainProfilePhoto(ctx context.Context, userId int64) int64 {
//}

func (d *Dao) UpdateProfilePhoto(ctx context.Context, userId, photoId int64) (int64, error) {
	return d.pgUpdateProfilePhoto(ctx, userId, photoId)
}

func (d *Dao) DeleteProfilePhotos(ctx context.Context, userId int64, photoIds []int64) (int64, error) {
	return d.pgDeleteProfilePhotos(ctx, userId, photoIds)
}

func (d *Dao) GetImmutableUser(ctx context.Context, id int64, privacy bool, contacts ...int64) (*mtproto.ImmutableUser, error) {
	cacheUserData, err := d.GetCacheUserDataWithError(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err != nil {
		return nil, err
	}

	// userDO, _ := c.svcCtx.Dao.UsersDAO.SelectById(c.ctx, in.Id)
	if cacheUserData == nil {
		err := mtproto.ErrUserIdInvalid
		logx.WithContext(ctx).Errorf("user.getImmutableUser - error: %v", err)
		return nil, err
	}
	userData := cacheUserData.UserData
	immutableUser := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User:             userData,
		LastSeenAt:       0,
		Contacts:         nil,
		KeysPrivacyRules: nil,
	}).To_ImmutableUser()

	if userData.Deleted {
		return immutableUser, nil
	}
	if userData.UserType == user.UserTypeUnknown ||
		userData.UserType == user.UserTypeBot ||
		userData.UserType == user.UserTypeDeleted {
		// not need load
		if privacy && userData.UserType == user.UserTypeBot {
			if err := d.prepareUserPrivacy(ctx, []*mtproto.ImmutableUser{immutableUser}, map[int64][]int64{id: contacts}); err != nil {
				return nil, err
			}
		}
		return immutableUser, nil
	}

	var presenceErr, contactsErr error
	mr.FinishVoid(
		func() {
			var lastSeenAt *dataobject.UserPresencesDO
			lastSeenAt, presenceErr = d.GetLastSeenAt(ctx, id)
			if lastSeenAt != nil {
				immutableUser.LastSeenAt = lastSeenAt.LastSeenAt
			}
		},
		func() {
			// TODO: aaa
			// immutableUser.Contacts = c.svcCtx.Dao.GetUserContactListByIdList(c.ctx, id, contacts...)

			idList := cacheUserData.GetContactIdList()
			if len(idList) == 0 {
				return
			}

			idList2 := make([]int64, 0, len(idList))
			for _, id2 := range contacts {
				if ok := container2.ContainsInt64(idList, id2); ok && id2 != id {
					idList2 = append(idList2, id2)
				}
			}
			if len(idList2) == 0 {
				return
			}

			immutableUser.Contacts, contactsErr = d.getContactListByIdList(ctx, id, idList2)
		})
	if presenceErr != nil && !errors.Is(presenceErr, sql.ErrNoRows) {
		return nil, presenceErr
	}
	if contactsErr != nil {
		return nil, contactsErr
	}
	//func() {
	//	if privacy {
	//		immutableUser.KeysPrivacyRules = c.svcCtx.Dao.GetUserPrivacyRulesListByKeys(
	//			c.ctx,
	//			id,
	//			user.STATUS_TIMESTAMP,
	//			user.PROFILE_PHOTO,
	//			user.PHONE_NUMBER)
	//	}
	//})
	if privacy {
		immutableUser.KeysPrivacyRules = cacheUserData.CachesPrivacyKeyRules
		if err := d.prepareUserPrivacy(ctx, []*mtproto.ImmutableUser{immutableUser}, map[int64][]int64{id: contacts}); err != nil {
			return nil, err
		}
	}

	return immutableUser, nil
}

func (d *Dao) UpdateUserEmojiStatus(ctx context.Context, id int64, emojiStatusDocumentId int64, emojiStatusUntil int32) error {
	_, err := d.UpdateUserFields(ctx, id, map[string]any{"emoji_status_document_id": emojiStatusDocumentId, "emoji_status_until": emojiStatusUntil})
	return err
}

// DeleteUser marks the account deleted and removes all user-owned data kept by
// the user service. The operation is transactional so a failure cannot leave a
// half-deleted profile that the BFF reports as successful.
func (d *Dao) DeleteUser(ctx context.Context, id int64, phoneNumber string, reason string) (bool, error) {
	if id <= 0 {
		return false, mtproto.ErrUserIdInvalid
	}
	if err := d.pgDeleteUser(ctx, id, phoneNumber, reason); err != nil {
		return false, err
	}
	return true, nil
}

func (d *Dao) GetCacheImmutableUserList(ctx context.Context, idList2 []int64, contacts []int64) []*mtproto.ImmutableUser {
	id := make([]int64, 0, len(idList2)+len(contacts))
	for _, v := range idList2 {
		if ok := container2.ContainsInt64(id, v); !ok {
			id = append(id, v)
		}
	}
	for _, v := range contacts {
		if ok := container2.ContainsInt64(id, v); !ok {
			id = append(id, v)
		}
	}

	if len(id) == 0 {
		return []*mtproto.ImmutableUser{}
	} else if len(id) == 1 {
		immutableUser, _ := d.GetImmutableUser(ctx, id[0], false)
		if immutableUser != nil {
			return []*mtproto.ImmutableUser{immutableUser}
		} else {
			return []*mtproto.ImmutableUser{}
		}
	}

	var (
		mUsers = make([]*mtproto.ImmutableUser, len(id))
	)

	mr.ForEach(
		func(source chan<- interface{}) {
			for idx := 0; idx < len(id); idx++ {
				source <- idx
			}
		},
		func(item interface{}) {
			var (
				idx = item.(int)
				err error
			)

			if ok := container2.ContainsInt64(contacts, id[idx]); ok {
				mUsers[idx], err = d.GetImmutableUser(ctx, id[idx], true, idList2...)
				if err != nil {
					logx.WithContext(ctx).Errorf("getImmutableUser - error: %v", err)
				}
			} else {
				if len(contacts) == 0 {
					mUsers[idx], err = d.GetImmutableUser(ctx, id[idx], true, idList2...)
					if err != nil {
						logx.WithContext(ctx).Errorf("getImmutableUser - error: %v", err)
					}
				} else {
					mUsers[idx], err = d.GetImmutableUser(ctx, id[idx], true, contacts...)
					if err != nil {
						logx.WithContext(ctx).Errorf("getImmutableUser - error: %v", err)
					}
				}
			}
		})

	for i := 0; i < len(mUsers); {
		if mUsers[i] != nil {
			i++
			continue
		}

		if i < len(mUsers)-1 {
			copy(mUsers[i:], mUsers[i+1:])
		}

		mUsers[len(mUsers)-1] = nil
		mUsers = mUsers[:len(mUsers)-1]
	}

	return mUsers
}

func (d *Dao) UpdateStoriesMaxId(ctx context.Context, id int64, maxId int32) error {
	_, err := d.UpdateUserFields(ctx, id, map[string]any{"stories_max_id": maxId})
	return err
}

func (d *Dao) UpdateColor(ctx context.Context, id int64, forProfile bool, color int32, backgroundEmojiId int64) error {
	field := "color"
	background := "color_background_emoji_id"
	if forProfile {
		field = "profile_color"
		background = "profile_color_background_emoji_id"
	}
	_, err := d.UpdateUserFields(ctx, id, map[string]any{field: color, background: backgroundEmojiId})
	return err
}

func (d *Dao) UpdateBirthday(ctx context.Context, id int64, birthday *mtproto.Birthday) error {
	_, err := d.UpdateUserFields(ctx, id, map[string]any{"birthday": birthday.ToBirthdayString()})
	return err
}

func (d *Dao) GetCacheImmutableUserListV2(ctx context.Context, idList2 []int64, contacts []int64) ([]*mtproto.ImmutableUser, error) {
	logger := logx.WithContext(ctx)

	logger.Infof("getCacheImmutableUserList - request: {id: %v, contacts: %v}", idList2, contacts)

	id2 := make([]int64, 0, len(idList2)+len(contacts))
	for _, v := range idList2 {
		if ok := container2.ContainsInt64(id2, v); !ok {
			id2 = append(id2, v)
		}
	}
	for _, v := range contacts {
		if ok := container2.ContainsInt64(id2, v); !ok {
			id2 = append(id2, v)
		}
	}

	if len(id2) == 0 {
		return []*mtproto.ImmutableUser{}, nil
	}

	if len(id2) == 1 {
		immutableUser, err := d.GetImmutableUser(ctx, id2[0], true, contacts...)
		if err != nil && !errors.Is(err, mtproto.ErrUserIdInvalid) {
			return nil, err
		}
		if immutableUser != nil {
			return []*mtproto.ImmutableUser{immutableUser}, nil
		} else {
			return []*mtproto.ImmutableUser{}, nil
		}
	}

	// len(id) > 1

	cDataList, err := d.GetCacheUserDataListByIdList(ctx, id2)
	if err != nil {
		return nil, err
	}
	if len(cDataList) == 0 {
		return []*mtproto.ImmutableUser{}, nil
	}

	var (
		cUserList = make([]*mtproto.ImmutableUser, 0, len(cDataList))
		mUsers    = make(map[int64]*mtproto.ImmutableUser, len(cDataList))
	)

	for _, cData := range cDataList {
		id := cData.GetUserData().GetId()

		cUser := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User:             cData.GetUserData(),
			LastSeenAt:       0,
			Contacts:         nil,
			KeysPrivacyRules: cData.CachesPrivacyKeyRules,
		}).To_ImmutableUser()

		cUserList = append(cUserList, cUser)

		if cUser.Deleted() {
			continue
		}
		if cData.GetUserData().GetUserType() == user.UserTypeUnknown ||
			cData.GetUserData().GetUserType() == user.UserTypeBot ||
			cData.GetUserData().GetUserType() == user.UserTypeDeleted {
			// not load these data
			continue
		}

		mUsers[id] = cUser

		// contacts
		var (
			myContacts []int64
		)
		if ok := container2.ContainsInt64(contacts, id); ok {
			myContacts = idList2
		} else {
			if len(contacts) == 0 {
				myContacts = idList2
			} else {
				myContacts = contacts
			}
		}

		cIdList := cData.GetContactIdList()
		if len(cIdList) == 0 {
			continue
		}
		for _, v := range myContacts {
			if ok := container2.ContainsInt64(cIdList, v); ok && v != id {
			}
		}
	}
	logger.Infof("getCacheImmutableUserList - cDataList: %d", len(cDataList))
	for _, cData := range cDataList {
		id := cData.GetUserData().GetId()
		cUser, ok := mUsers[id]
		if !ok {
			continue
		}
		presence, err := d.GetLastSeenAt(ctx, id)
		if err != nil {
			return nil, err
		}
		if presence != nil {
			cUser.LastSeenAt = presence.LastSeenAt
		}
		myContacts := contacts
		if len(myContacts) == 0 || container2.ContainsInt64(contacts, id) {
			myContacts = idList2
		}
		list, err := d.Postgres.Store.Contacts.SelectListByIdList(ctx, id, myContacts)
		if err != nil {
			return nil, err
		}
		for _, contact := range list {
			cUser.Contacts = append(cUser.Contacts, mtproto.MakeTLContactData(&mtproto.ContactData{UserId: contact.OwnerUserId, ContactUserId: contact.ContactUserId, FirstName: mtproto.MakeFlagsString(contact.ContactFirstName), LastName: mtproto.MakeFlagsString(contact.ContactLastName), MutualContact: contact.Mutual, Phone: mtproto.MakeFlagsString(contact.ContactPhone), CloseFriend: contact.CloseFriend}).To_ContactData())
		}
	}
	viewers := make(map[int64][]int64, len(cUserList))
	for _, entry := range cUserList {
		if len(contacts) == 0 || container2.ContainsInt64(contacts, entry.Id()) {
			viewers[entry.Id()] = idList2
		} else {
			viewers[entry.Id()] = contacts
		}
	}
	if err := d.prepareUserPrivacy(ctx, cUserList, viewers); err != nil {
		return nil, err
	}
	return cUserList, nil
}

func (d *Dao) GetImmutableUserV2(ctx context.Context, id int64, privacy bool, hasReverseContacts bool, reverseContacts []int64) (*mtproto.ImmutableUser, error) {
	cacheUserData, err := d.GetCacheUserDataWithError(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err != nil {
		return nil, err
	}

	if cacheUserData == nil {
		err := mtproto.ErrUserIdInvalid
		logx.WithContext(ctx).Errorf("user.getImmutableUser - error: %v", err)
		return nil, err
	}
	userData := cacheUserData.UserData
	immutableUser := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User:             userData,
		LastSeenAt:       0,
		Contacts:         nil,
		KeysPrivacyRules: nil,
		ReverseContacts:  nil,
	}).To_ImmutableUser()

	if userData.Deleted {
		return immutableUser, nil
	}
	if userData.UserType == user.UserTypeUnknown ||
		userData.UserType == user.UserTypeBot ||
		userData.UserType == user.UserTypeDeleted {
		// not load these data
		if privacy && userData.UserType == user.UserTypeBot {
			if err := d.prepareUserPrivacy(ctx, []*mtproto.ImmutableUser{immutableUser}, map[int64][]int64{id: reverseContacts}); err != nil {
				return nil, err
			}
		}
		return immutableUser, nil
	}

	var (
		rIdList []int64
	)

	if hasReverseContacts && len(cacheUserData.ReverseContactIdList) > 0 {
		if len(reverseContacts) == 0 {
			rIdList = cacheUserData.ReverseContactIdList
		} else {
			for _, id2 := range reverseContacts {
				if ok := container2.ContainsInt64(cacheUserData.ReverseContactIdList, id2); ok && id2 != id {
					rIdList = append(rIdList, id2)
				}
			}
		}
	}

	var presenceErr, contactsErr error
	fns := []func(){
		func() {
			var lastSeenAt *dataobject.UserPresencesDO
			lastSeenAt, presenceErr = d.GetLastSeenAt(ctx, id)
			if lastSeenAt != nil {
				immutableUser.LastSeenAt = lastSeenAt.LastSeenAt
			}
		},
	}

	if len(rIdList) > 0 {
		fns = append(
			fns,
			func() {
				immutableUser.ReverseContacts, contactsErr = d.getReverseContactListByIdList(ctx, id, rIdList)
			})

	}

	if len(fns) == 1 {
		fns[0]()
	} else {
		mr.FinishVoid(fns...)
	}
	if presenceErr != nil && !errors.Is(presenceErr, sql.ErrNoRows) {
		return nil, presenceErr
	}
	if contactsErr != nil {
		return nil, contactsErr
	}

	if privacy {
		immutableUser.KeysPrivacyRules = cacheUserData.CachesPrivacyKeyRules
		viewers := reverseContacts
		if len(viewers) == 0 && hasReverseContacts {
			viewers = rIdList
		}
		if err := d.prepareUserPrivacy(ctx, []*mtproto.ImmutableUser{immutableUser}, map[int64][]int64{id: viewers}); err != nil {
			return nil, err
		}
	}

	return immutableUser, nil
}

func (d *Dao) GetMutableUsersV2(ctx context.Context, idList2 []int64, privacy bool, hasTo bool, to []int64) ([]*mtproto.ImmutableUser, error) {
	if len(idList2) == 0 {
		return []*mtproto.ImmutableUser{}, nil
	}

	if len(idList2) == 1 {
		immutableUser, err := d.GetImmutableUserV2(ctx, idList2[0], privacy, hasTo, to)
		if err != nil && !errors.Is(err, mtproto.ErrUserIdInvalid) {
			return nil, err
		}
		if immutableUser != nil {
			return []*mtproto.ImmutableUser{immutableUser}, nil
		} else {
			return []*mtproto.ImmutableUser{}, nil
		}
	}

	cDataList, err := d.GetCacheUserDataListByIdList(ctx, idList2)
	if err != nil {
		return nil, err
	}
	if len(cDataList) == 0 {
		return []*mtproto.ImmutableUser{}, nil
	}

	var (
		cUserList = make([]*mtproto.ImmutableUser, 0, len(cDataList))
		mUsers    = make(map[int64]*mtproto.ImmutableUser, len(cDataList))
	)

	for _, cData := range cDataList {
		id := cData.GetUserData().GetId()

		cUser := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User:             cData.GetUserData(),
			LastSeenAt:       0,
			Contacts:         nil,
			ReverseContacts:  nil,
			KeysPrivacyRules: nil,
		}).To_ImmutableUser()
		if privacy {
			cUser.KeysPrivacyRules = cData.CachesPrivacyKeyRules
		}

		cUserList = append(cUserList, cUser)

		if cUser.Deleted() {
			continue
		}
		if cData.GetUserData().GetUserType() == user.UserTypeUnknown ||
			cData.GetUserData().GetUserType() == user.UserTypeBot ||
			cData.GetUserData().GetUserType() == user.UserTypeDeleted {
			continue
		}

		mUsers[id] = cUser

	}
	for _, cData := range cDataList {
		id := cData.GetUserData().GetId()
		cUser, ok := mUsers[id]
		if !ok {
			continue
		}
		presence, err := d.GetLastSeenAt(ctx, id)
		if err != nil {
			return nil, err
		}
		if presence != nil {
			cUser.LastSeenAt = presence.LastSeenAt
		}
		if !hasTo || len(cData.ReverseContactIdList) == 0 {
			continue
		}
		owners := cData.ReverseContactIdList
		if len(to) > 0 {
			owners = make([]int64, 0, len(to))
			for _, owner := range to {
				if container2.ContainsInt64(cData.ReverseContactIdList, owner) {
					owners = append(owners, owner)
				}
			}
		}
		list, err := d.Postgres.Store.Contacts.SelectReverseListByIdList(ctx, id, owners)
		if err != nil {
			return nil, err
		}
		for _, contact := range list {
			cUser.ReverseContacts = append(cUser.ReverseContacts, mtproto.MakeTLContactData(&mtproto.ContactData{UserId: contact.OwnerUserId, ContactUserId: contact.ContactUserId, FirstName: mtproto.MakeFlagsString(contact.ContactFirstName), LastName: mtproto.MakeFlagsString(contact.ContactLastName), MutualContact: contact.Mutual, Phone: mtproto.MakeFlagsString(contact.ContactPhone), CloseFriend: contact.CloseFriend}).To_ContactData())
		}
	}
	if privacy {
		viewers := make(map[int64][]int64, len(cUserList))
		for _, entry := range cUserList {
			if len(to) == 0 || container2.ContainsInt64(to, entry.Id()) {
				viewers[entry.Id()] = idList2
			} else {
				viewers[entry.Id()] = to
			}
			if hasTo && len(to) == 0 {
				viewers[entry.Id()] = append([]int64{}, viewers[entry.Id()]...)
				for _, contact := range entry.GetReverseContacts() {
					viewers[entry.Id()] = append(viewers[entry.Id()], contact.GetUserId())
				}
			}
		}
		if err := d.prepareUserPrivacy(ctx, cUserList, viewers); err != nil {
			return nil, err
		}
	}
	return cUserList, nil
}

func (d *Dao) UpdatePersonalChannel(ctx context.Context, id int64, personalChanelId int64) error {
	_, err := d.UpdateUserFields(ctx, id, map[string]any{"personal_channel_id": personalChanelId})
	return err
}

func (d *Dao) UpdatePhoneNumber(ctx context.Context, id int64, phoneNumber string) error {
	_, err := d.UpdateUserFields(ctx, id, map[string]any{"phone": phoneNumber})
	return err
}

func (d *Dao) UpdateUserPremium(ctx context.Context, id int64, premium bool, months int32) error {
	return d.pgUpdatePremium(ctx, id, premium, months)
}

func (d *Dao) SaveUserMusic(ctx context.Context, userID, musicID int64, unsave bool) error {
	return d.pgSaveMusic(ctx, userID, musicID, unsave)
}

var errPremiumPaymentConflict = errors.New("premium payment transaction conflicts with an existing grant")

func (d *Dao) GrantUserPremium(ctx context.Context, id int64, months int32, provider, transactionID string) (bool, error) {
	if id <= 0 || months < 1 || months > 36 || provider == "" || len(provider) > 32 || transactionID == "" || len(transactionID) > 191 {
		return false, mtproto.ErrInputRequestInvalid
	}
	if err := d.pgGrantPremium(ctx, id, months, provider, transactionID); err != nil {
		return false, err
	}
	return true, nil
}
