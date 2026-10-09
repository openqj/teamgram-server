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
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/teamgram/marmota/pkg/container2"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/jsonx"
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

	//
	//tR := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
	// var err error
	// user
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
	if d.Postgres != nil {
		if err := d.pgCreateNewUser(ctx, userDO, now, 300); err != nil {
			return nil, err
		}
		cacheUserData.UserData = d.MakeUserDataByDO(userDO)
		return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: cacheUserData.UserData, LastSeenAt: now}).To_ImmutableUser(), nil
	}
	if lastInsertId, _, err2 := d.UsersDAO.Insert(ctx, userDO); err2 != nil {
		if sqlx.IsDuplicate(err2) {
			err2 = mtproto.ErrPhoneNumberOccupied
		}
		return nil, err2
		//result.Err = err2
		//return
	} else {
		userDO.Id = lastInsertId
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

	// 1. cacheUserData
	d.CachedConn.SetCache(ctx, genCacheUserDataCacheKey(userDO.Id), cacheUserData)

	// 2. PutLastSeenAt
	_ = d.PutLastSeenAt(ctx, userDO.Id, now, 300)

	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User:             cacheUserData.UserData,
		LastSeenAt:       now,
		Contacts:         nil,
		KeysPrivacyRules: nil,
	}).To_ImmutableUser(), nil
}

func (d *Dao) UpdateUserFirstAndLastName(ctx context.Context, id int64, firstName, lastName string) error {
	if d.Postgres != nil {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"first_name": firstName, "last_name": lastName})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			rowsAffected, err := d.UsersDAO.UpdateUser(ctx, map[string]interface{}{
				"first_name": firstName,
				"last_name":  lastName,
			}, id)

			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateUserFirstAndLastName - error: %v", err)
	}

	return err
}

func (d *Dao) UpdateUserAbout(ctx context.Context, id int64, about string) error {
	if d.Postgres != nil {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"about": about})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			rowsAffected, err := d.UsersDAO.UpdateUser(ctx, map[string]interface{}{
				"about": about,
			}, id)

			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateUserAbout - error: %v", err)
	}

	return err
}

func (d *Dao) UpdateUserUsername(ctx context.Context, id int64, username string) error {
	if d.Postgres != nil {
		return d.pgUpdateUsername(ctx, id, username)
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			transaction := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
				var current struct {
					Username string `db:"username"`
				}
				result.Err = tx.QueryRowPartial(&current, "SELECT username FROM users WHERE id = ? FOR UPDATE", id)
				if result.Err != nil {
					if errors.Is(result.Err, sqlx.ErrNotFound) {
						result.Err = mtproto.ErrUserIdInvalid
					}
					return
				}

				if username != "" {
					var entry dataobject.UsernameDO
					err := tx.QueryRowPartial(&entry,
						"SELECT username, peer_type, peer_id FROM username WHERE username = ? FOR UPDATE", username)
					if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
						result.Err = err
						return
					}
					if err == nil {
						if current.Username != username || entry.PeerType != mtproto.PEER_USER || entry.PeerId != id {
							result.Err = mtproto.ErrUsernameOccupied
							return
						}
					} else {
						_, _, result.Err = d.UsernameDAO.InsertTx(tx, &dataobject.UsernameDO{
							Username: username,
							PeerType: mtproto.PEER_USER,
							PeerId:   id,
							Editable: true,
							Active:   true,
							Order2:   time.Now().Unix() << 32,
						})
						if result.Err != nil {
							return
						}
					}
				}

				if current.Username != "" && current.Username != username {
					var old dataobject.UsernameDO
					err := tx.QueryRowPartial(&old,
						"SELECT username, peer_type, peer_id FROM username WHERE username = ? FOR UPDATE", current.Username)
					if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
						result.Err = err
						return
					}
					if err == nil {
						if old.PeerType != mtproto.PEER_USER || old.PeerId != id {
							result.Err = mtproto.ErrInternalServerError
							return
						}
						_, result.Err = tx.Exec("DELETE FROM username WHERE username = ? AND peer_type = ? AND peer_id = ?", current.Username, mtproto.PEER_USER, id)
						if result.Err != nil {
							return
						}
					}
				}

				if current.Username != username {
					var rowsAffected int64
					rowsAffected, result.Err = d.UsersDAO.UpdateUsernameTx(tx, username, id)
					if result.Err != nil {
						return
					}
					if rowsAffected != 1 {
						result.Err = mtproto.ErrInternalServerError
					}
				}
			})
			if transaction.Err != nil {
				return 0, 0, transaction.Err
			}
			return 0, 1, nil
		},
		genCacheUserDataCacheKey(id), fmt.Sprintf("username_%d", id))
	if err != nil {
		if sqlx.IsDuplicate(err) {
			return mtproto.ErrUsernameOccupied
		}
		logx.WithContext(ctx).Errorf("updateUserUsername - error: %v", err)
		return err
	}

	return nil
}

//func (d *Dao) DeleteProfilePhoto(ctx context.Context, userId, photoId int64) int64 {
//}
//
//func (d *Dao) DeleteMainProfilePhoto(ctx context.Context, userId int64) int64 {
//}

func (d *Dao) UpdateProfilePhoto(ctx context.Context, userId, photoId int64) (int64, error) {
	if d.Postgres != nil {
		return d.pgUpdateProfilePhoto(ctx, userId, photoId)
	}
	var mainPhotoId int64
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			transaction := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
				var currentPhotoId int64
				result.Err = tx.QueryRowPartial(&currentPhotoId, "SELECT photo_id FROM users WHERE id = ? FOR UPDATE", userId)
				if result.Err != nil {
					if errors.Is(result.Err, sqlx.ErrNotFound) {
						result.Err = mtproto.ErrUserIdInvalid
					}
					return
				}

				mainPhotoId = photoId
				if photoId == 0 {
					if currentPhotoId > 0 {
						if _, result.Err = d.UserProfilePhotosDAO.DeleteTx(tx, userId, []int64{currentPhotoId}); result.Err != nil {
							return
						}
						query := "SELECT photo_id FROM user_profile_photos WHERE user_id = ? AND photo_id <> ? AND deleted = 0 ORDER BY date2 DESC LIMIT 1 FOR UPDATE"
						result.Err = tx.QueryRowPartial(&mainPhotoId, query, userId, currentPhotoId)
						if errors.Is(result.Err, sqlx.ErrNotFound) {
							mainPhotoId = 0
							result.Err = nil
						} else if result.Err != nil {
							return
						}
					}
				} else {
					_, _, result.Err = d.UserProfilePhotosDAO.InsertOrUpdateTx(tx, &dataobject.UserProfilePhotosDO{
						UserId:  userId,
						PhotoId: photoId,
						Date2:   time.Now().Unix(),
					})
					if result.Err != nil {
						return
					}
				}

				_, result.Err = d.UsersDAO.UpdateProfilePhotoTx(tx, mainPhotoId, userId)
			})
			if transaction.Err != nil {
				return 0, 0, transaction.Err
			}
			return 0, 1, nil
		},
		genCacheUserDataCacheKey(userId))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateProfilePhoto - error: %v", err)
		return 0, err
	}

	return mainPhotoId, nil
}

func (d *Dao) DeleteProfilePhotos(ctx context.Context, userId int64, photoIds []int64) (int64, error) {
	if d.Postgres != nil {
		return d.pgDeleteProfilePhotos(ctx, userId, photoIds)
	}
	var mainPhotoId int64
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			transaction := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
				result.Err = tx.QueryRowPartial(&mainPhotoId, "SELECT photo_id FROM users WHERE id = ? FOR UPDATE", userId)
				if result.Err != nil {
					if errors.Is(result.Err, sqlx.ErrNotFound) {
						result.Err = mtproto.ErrUserIdInvalid
					}
					return
				}
				if len(photoIds) == 0 {
					return
				}

				if _, result.Err = d.UserProfilePhotosDAO.DeleteTx(tx, userId, photoIds); result.Err != nil {
					return
				}
				if !container2.ContainsInt64(photoIds, mainPhotoId) {
					return
				}

				query := fmt.Sprintf("SELECT photo_id FROM user_profile_photos WHERE user_id = ? AND photo_id NOT IN (%s) AND deleted = 0 ORDER BY date2 DESC LIMIT 1 FOR UPDATE", sqlx.InInt64List(photoIds))
				result.Err = tx.QueryRowPartial(&mainPhotoId, query, userId)
				if errors.Is(result.Err, sqlx.ErrNotFound) {
					mainPhotoId = 0
					result.Err = nil
				} else if result.Err != nil {
					return
				}
				_, result.Err = d.UsersDAO.UpdateProfilePhotoTx(tx, mainPhotoId, userId)
			})
			if transaction.Err != nil {
				return 0, 0, transaction.Err
			}
			return 0, 1, nil
		},
		genCacheUserDataCacheKey(userId))
	if err != nil {
		logx.WithContext(ctx).Errorf("deleteProfilePhotos - error: %v", err)
		return 0, err
	}
	return mainPhotoId, nil
}

func (d *Dao) GetImmutableUser(ctx context.Context, id int64, privacy bool, contacts ...int64) (*mtproto.ImmutableUser, error) {
	cacheUserData, err := d.GetCacheUserDataWithError(ctx, id)
	if errors.Is(err, sqlc.ErrNotFound) {
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
	if presenceErr != nil && !errors.Is(presenceErr, sqlc.ErrNotFound) {
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
	if d.Postgres != nil {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"emoji_status_document_id": emojiStatusDocumentId, "emoji_status_until": emojiStatusUntil})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			rowsAffected, err := d.UsersDAO.UpdateEmojiStatus(
				ctx,
				emojiStatusDocumentId,
				emojiStatusUntil,
				id)

			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateUserEmojiStatus - error: %v", err)
	}

	return err
}

// DeleteUser marks the account deleted and removes all user-owned data kept by
// the user service. The operation is transactional so a failure cannot leave a
// half-deleted profile that the BFF reports as successful.
func (d *Dao) DeleteUser(ctx context.Context, id int64, phoneNumber string, reason string) (bool, error) {
	if id <= 0 {
		return false, mtproto.ErrUserIdInvalid
	}
	if d.Postgres != nil {
		if err := d.pgDeleteUser(ctx, id, phoneNumber, reason); err != nil {
			return false, err
		}
		return true, nil
	}

	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		// These tables contain data whose owner is the account being deleted.
		// Keep the list explicit: message/chat data lives in other services and
		// must be purged by their own authoritative providers.
		cleanup := []struct {
			query string
			args  []interface{}
		}{
			{"DELETE FROM username WHERE peer_type = ? AND peer_id = ?", []interface{}{mtproto.PEER_USER, id}},
			{"DELETE FROM user_contacts WHERE owner_user_id = ? OR contact_user_id = ?", []interface{}{id, id}},
			{"DELETE FROM imported_contacts WHERE user_id = ? OR imported_user_id = ?", []interface{}{id, id}},
			{"DELETE FROM unregistered_contacts WHERE importer_user_id = ?", []interface{}{id}},
			{"DELETE FROM user_global_privacy_settings WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_notify_settings WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_peer_blocks WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_peer_settings WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_presences WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_privacies WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_profile_photos WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_saved_music WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM user_settings WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM default_history_ttl WHERE user_id = ?", []interface{}{id}},
			{"DELETE FROM bots WHERE creator_user_id = ?", []interface{}{id}},
			{"DELETE FROM popular_contacts WHERE phone = ?", []interface{}{phoneNumber}},
		}
		for _, item := range cleanup {
			if _, err := tx.Exec(item.query, item.args...); err != nil {
				result.Err = err
				return
			}
		}

		rowsAffected, err := d.UsersDAO.DeleteTx(
			tx,
			"-"+strconv.FormatInt(id, 10), // preserve the historical phone tombstone format
			reason,
			id)
		if err != nil {
			result.Err = err
			return
		}
		if rowsAffected != 1 {
			result.Err = mtproto.ErrUserIdInvalid
		}
	})
	if result.Err != nil {
		logx.WithContext(ctx).Errorf("DeleteUser - error: %v", result.Err)
		return false, result.Err
	}

	if err := d.CachedConn.DelCache(ctx, genCacheUserDataCacheKey(id), genCachePhoneUserKey(phoneNumber)); err != nil {
		logx.WithContext(ctx).Errorf("DeleteUser - invalidate cache: %v", err)
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
	if d.Postgres != nil {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"stories_max_id": maxId})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			rowsAffected, err := d.UsersDAO.UpdateStoriesMaxId(ctx, maxId, id)

			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateStoriesMaxId - error: %v", err)
	}

	return err
}

func (d *Dao) UpdateColor(ctx context.Context, id int64, forProfile bool, color int32, backgroundEmojiId int64) error {
	if d.Postgres != nil {
		field := "color"
		background := "color_background_emoji_id"
		if forProfile {
			field = "profile_color"
			background = "profile_color_background_emoji_id"
		}
		_, err := d.UpdateUserFields(ctx, id, map[string]any{field: color, background: backgroundEmojiId})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			var (
				err          error
				rowsAffected int64
			)

			if forProfile {
				rowsAffected, err = d.UsersDAO.UpdateProfileColor(ctx, color, backgroundEmojiId, id)
			} else {
				rowsAffected, err = d.UsersDAO.UpdateColor(ctx, color, backgroundEmojiId, id)
			}

			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateColor - error: %v", err)
	}

	return err
}

func (d *Dao) UpdateBirthday(ctx context.Context, id int64, birthday *mtproto.Birthday) error {
	if d.Postgres != nil {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"birthday": birthday.ToBirthdayString()})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			var (
				err          error
				rowsAffected int64
			)

			rowsAffected, err = d.UsersDAO.UpdateBirthday(ctx, birthday.ToBirthdayString(), id)

			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateBirthday - error: %v", err)
	}

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
		keyList   = make([]string, 0, len(id2))
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

		// LastSeenAt
		keyList = append(keyList, genUserPresencesKey(cData.GetUserData().GetId()))

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
				keyList = append(keyList, genContactCacheKey(id, v))
			}
		}
	}
	logger.Infof("getCacheImmutableUserList - cDataList: %d", len(cDataList))
	if d.Postgres != nil {
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

	err = d.CachedConn.QueryRows(
		ctx,
		func(ctx context.Context, conn *sqlx.DB, keys ...string) (map[string]interface{}, error) {
			noCaches := make(map[string]interface{}, len(keys))
			for _, key := range keys {
				logger.Infof("getCacheImmutableUserList - noCache: %v", keys)
				if isUserPresencesKey(key) {
					lastSeenAt, err := d.UserPresencesDAO.Select(ctx, parseUserPresencesKey(key))
					if err != nil {
						continue
					}
					if lastSeenAt != nil {
						noCaches[key] = lastSeenAt

						if cUser, ok := mUsers[lastSeenAt.UserId]; ok {
							cUser.LastSeenAt = lastSeenAt.LastSeenAt
						}
					}
				} else {
					id0, id1 := parseContactCacheKey(key)
					contact, _ := d.UserContactsDAO.SelectContact(ctx, id0, id1)
					if contact == nil {
						// return sqlc.ErrNotFound
						continue
					}
					noCaches[key] = contact
					if cUser, ok := mUsers[contact.OwnerUserId]; ok {
						cUser.Contacts = append(cUser.Contacts, mtproto.MakeTLContactData(&mtproto.ContactData{
							UserId:        contact.OwnerUserId,
							ContactUserId: contact.ContactUserId,
							FirstName:     mtproto.MakeFlagsString(contact.ContactFirstName),
							LastName:      mtproto.MakeFlagsString(contact.ContactLastName),
							MutualContact: contact.Mutual,
							Phone:         mtproto.MakeFlagsString(contact.ContactPhone),
							CloseFriend:   contact.CloseFriend,
						}).To_ContactData())
					}
				}
			}

			return noCaches, nil
		},
		func(k, v string) (interface{}, error) {
			logger.Infof("getCacheImmutableUserList - cache: {k: %s, v: %d}", k, len(v))

			if isUserPresencesKey(k) {
				var (
					lastSeenAt *dataobject.UserPresencesDO
				)
				err := jsonx.UnmarshalFromString(v, &lastSeenAt)
				if err != nil {
					return nil, err
				}

				if cUser, ok := mUsers[lastSeenAt.UserId]; ok {
					cUser.LastSeenAt = lastSeenAt.LastSeenAt
				}

				return lastSeenAt, nil
			} else {
				var (
					contact *dataobject.UserContactsDO
				)

				err := jsonx.UnmarshalFromString(v, &contact)
				if err != nil {
					return nil, err
				}

				if cUser, ok := mUsers[contact.OwnerUserId]; ok {
					cUser.Contacts = append(cUser.Contacts, mtproto.MakeTLContactData(&mtproto.ContactData{
						UserId:        contact.OwnerUserId,
						ContactUserId: contact.ContactUserId,
						FirstName:     mtproto.MakeFlagsString(contact.ContactFirstName),
						LastName:      mtproto.MakeFlagsString(contact.ContactLastName),
						MutualContact: contact.Mutual,
						Phone:         mtproto.MakeFlagsString(contact.ContactPhone),
						CloseFriend:   contact.CloseFriend,
					}).To_ContactData())
				}

				return contact, nil
			}
		},
		keyList...)

	logger.Infof("getCacheImmutableUserList - cUserList: %d", len(cUserList))

	return cUserList, err
}

func (d *Dao) GetImmutableUserV2(ctx context.Context, id int64, privacy bool, hasReverseContacts bool, reverseContacts []int64) (*mtproto.ImmutableUser, error) {
	cacheUserData, err := d.GetCacheUserDataWithError(ctx, id)
	if errors.Is(err, sqlc.ErrNotFound) {
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
	if presenceErr != nil && !errors.Is(presenceErr, sqlc.ErrNotFound) {
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
		keyList   = make([]string, 0, len(idList2))
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

		// LastSeenAt
		keyList = append(keyList, genUserPresencesKey(cData.GetUserData().GetId()))

		// contacts
		var (
			rContacts []int64
		)

		if hasTo && len(cData.ReverseContactIdList) > 0 {
			if len(to) == 0 {
				rContacts = cData.ReverseContactIdList
			} else {
				for _, rId := range to {
					if ok := container2.ContainsInt64(cData.ReverseContactIdList, rId); ok {
						rContacts = append(rContacts, rId)
					}
				}
			}
		}

		for _, v := range rContacts {
			keyList = append(keyList, genContactCacheKey(v, id))
		}
	}
	if d.Postgres != nil {
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

	err = d.CachedConn.QueryRows(
		ctx,
		func(ctx context.Context, conn *sqlx.DB, keys ...string) (map[string]interface{}, error) {
			noCaches := make(map[string]interface{}, len(keys))
			for _, key := range keys {
				//logger.Infof("getCacheImmutableUserList - noCache: %v", keys)
				if isUserPresencesKey(key) {
					lastSeenAt, err := d.UserPresencesDAO.Select(ctx, parseUserPresencesKey(key))
					if err != nil {
						continue
					}
					if lastSeenAt != nil {
						noCaches[key] = lastSeenAt

						if cUser, ok := mUsers[lastSeenAt.UserId]; ok {
							cUser.LastSeenAt = lastSeenAt.LastSeenAt
						}
					}
				} else {
					id0, id1 := parseContactCacheKey(key)
					contact, _ := d.UserContactsDAO.SelectContact(ctx, id0, id1)
					if contact == nil {
						// return sqlc.ErrNotFound
						continue
					}
					noCaches[key] = contact
					if cUser, ok := mUsers[contact.ContactUserId]; ok {
						cUser.ReverseContacts = append(cUser.ReverseContacts, mtproto.MakeTLContactData(&mtproto.ContactData{
							UserId:        contact.OwnerUserId,
							ContactUserId: contact.ContactUserId,
							FirstName:     mtproto.MakeFlagsString(contact.ContactFirstName),
							LastName:      mtproto.MakeFlagsString(contact.ContactLastName),
							MutualContact: contact.Mutual,
							Phone:         mtproto.MakeFlagsString(contact.ContactPhone),
							CloseFriend:   contact.CloseFriend,
						}).To_ContactData())
					}
				}
			}

			return noCaches, nil
		},
		func(k, v string) (interface{}, error) {
			//logger.Infof("getCacheImmutableUserList - cache: {k: %s, v: %d}", k, len(v))
			if isUserPresencesKey(k) {
				var (
					lastSeenAt *dataobject.UserPresencesDO
				)
				err := jsonx.UnmarshalFromString(v, &lastSeenAt)
				if err != nil {
					return nil, err
				}

				if cUser, ok := mUsers[lastSeenAt.UserId]; ok {
					cUser.LastSeenAt = lastSeenAt.LastSeenAt
				}

				return lastSeenAt, nil
			} else {
				var (
					contact *dataobject.UserContactsDO
				)

				err := jsonx.UnmarshalFromString(v, &contact)
				if err != nil {
					return nil, err
				}

				if cUser, ok := mUsers[contact.ContactUserId]; ok {
					cUser.ReverseContacts = append(cUser.ReverseContacts, mtproto.MakeTLContactData(&mtproto.ContactData{
						UserId:        contact.OwnerUserId,
						ContactUserId: contact.ContactUserId,
						FirstName:     mtproto.MakeFlagsString(contact.ContactFirstName),
						LastName:      mtproto.MakeFlagsString(contact.ContactLastName),
						MutualContact: contact.Mutual,
						Phone:         mtproto.MakeFlagsString(contact.ContactPhone),
						CloseFriend:   contact.CloseFriend,
					}).To_ContactData())
				}

				return contact, nil
			}
		},
		keyList...)

	return cUserList, err
}

func (d *Dao) UpdatePersonalChannel(ctx context.Context, id int64, personalChanelId int64) error {
	if d.Postgres != nil {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"personal_channel_id": personalChanelId})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			rowsAffected, err := d.UsersDAO.UpdatePersonalChannelId(ctx, personalChanelId, id)

			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updatePersonalChannel - error: %v", err)
	}

	return err
}

func (d *Dao) UpdatePhoneNumber(ctx context.Context, id int64, phoneNumber string) error {
	if d.Postgres != nil {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"phone": phoneNumber})
		return err
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			_, err := d.UsersDAO.UpdateUser(ctx, map[string]interface{}{
				"phone": phoneNumber, // TODO(@benqi): country_code
			}, id)

			// TODO: UpdatePhoneByContactId
			// c.svcCtx.Dao.UserContactsDAO.UpdatePhoneByContactId(c.ctx, in.Phone, in.UserId)
			if err != nil {
				return 0, 0, err
			}

			return 0, 0, nil
		},
		genCacheUserDataCacheKey(id),
		genCachePhoneUserKey(phoneNumber))
	if err != nil {
		logx.WithContext(ctx).Errorf("updatePersonalChannel - error: %v", err)
		return err
	}

	return nil
}

func (d *Dao) UpdateUserPremium(ctx context.Context, id int64, premium bool, months int32) error {
	if d.Postgres != nil {
		return d.pgUpdatePremium(ctx, id, premium, months)
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			if !premium {
				rowsAffected, err := d.UsersDAO.UpdateUser(ctx, map[string]interface{}{
					"premium":             false,
					"premium_expire_date": 0,
				}, id)
				return 0, rowsAffected, err
			}

			if months < 0 {
				return 0, 0, mtproto.ErrInputRequestInvalid
			}
			userDO, err := d.UsersDAO.SelectById(ctx, id)
			if errors.Is(err, sqlc.ErrNotFound) {
				return 0, 0, mtproto.ErrUserIdInvalid
			}
			if err != nil {
				return 0, 0, err
			}
			if userDO == nil {
				return 0, 0, mtproto.ErrUserIdInvalid
			}

			var expireDate int64
			if months == 0 || (userDO.Premium && userDO.PremiumExpireDate == 0) {
				expireDate = 0
			} else {
				base := time.Now()
				if userDO.Premium && userDO.PremiumExpireDate > base.Unix() {
					base = time.Unix(userDO.PremiumExpireDate, 0)
				}
				expireDate = base.AddDate(0, int(months), 0).Unix()
			}
			rowsAffected, err := d.UsersDAO.UpdateUser(ctx, map[string]interface{}{
				"premium":             true,
				"premium_expire_date": expireDate,
			}, id)
			if err != nil {
				return 0, 0, err
			}

			return 0, rowsAffected, nil
		},
		genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("updateUserPremium - error: %v", err)
	}

	return err
}

func (d *Dao) SaveUserMusic(ctx context.Context, userID, musicID int64, unsave bool) error {
	if d == nil || d.Postgres == nil {
		return errors.New("biz/user: PostgreSQL store is not configured")
	}
	return d.pgSaveMusic(ctx, userID, musicID, unsave)
}

var errPremiumPaymentConflict = errors.New("premium payment transaction conflicts with an existing grant")

func (d *Dao) GrantUserPremium(ctx context.Context, id int64, months int32, provider, transactionID string) (bool, error) {
	if id <= 0 || months < 1 || months > 36 || provider == "" || len(provider) > 32 || transactionID == "" || len(transactionID) > 191 {
		return false, mtproto.ErrInputRequestInvalid
	}
	if d.Postgres != nil {
		if err := d.pgGrantPremium(ctx, id, months, provider, transactionID); err != nil {
			return false, err
		}
		return true, nil
	}
	transactionKey := sha256.Sum256([]byte(provider + "\x00" + transactionID))

	_, _, err := d.CachedConn.Exec(ctx, func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
		txResult := sqlx.TxWrapper(ctx, conn, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
			var userDO dataobject.UsersDO
			if result.Err = tx.QueryRow(&userDO, `SELECT premium, premium_expire_date FROM users WHERE id=? FOR UPDATE`, id); result.Err != nil {
				if errors.Is(result.Err, sqlc.ErrNotFound) {
					result.Err = mtproto.ErrUserIdInvalid
				}
				return
			}

			insert, insertErr := tx.Exec(`INSERT IGNORE INTO user_premium_payment_grant
				(transaction_key, provider, transaction_id, user_id, months, created_at) VALUES (?,?,?,?,?,?)`, transactionKey[:], provider, transactionID, id, months, time.Now().Unix())
			if insertErr != nil {
				result.Err = insertErr
				return
			}
			inserted, insertErr := insert.RowsAffected()
			if insertErr != nil {
				result.Err = insertErr
				return
			}
			if inserted == 0 {
				var existing struct {
					Provider      string `db:"provider"`
					TransactionID string `db:"transaction_id"`
					UserID        int64  `db:"user_id"`
					Months        int32  `db:"months"`
				}
				result.Err = tx.QueryRow(&existing, `SELECT provider, transaction_id, user_id, months FROM user_premium_payment_grant WHERE transaction_key=? FOR UPDATE`, transactionKey[:])
				if result.Err == nil && (existing.Provider != provider || existing.TransactionID != transactionID || existing.UserID != id || existing.Months != months) {
					result.Err = errPremiumPaymentConflict
				}
				return
			}

			if userDO.Premium && userDO.PremiumExpireDate == 0 {
				return
			}
			base := time.Now()
			if userDO.Premium && userDO.PremiumExpireDate > base.Unix() {
				base = time.Unix(userDO.PremiumExpireDate, 0)
			}
			_, result.Err = tx.Exec(`UPDATE users SET premium=1, premium_expire_date=? WHERE id=?`, base.AddDate(0, int(months), 0).Unix(), id)
		})
		if txResult.Err != nil {
			return 0, 0, txResult.Err
		}
		return 0, 0, nil
	}, genCacheUserDataCacheKey(id))
	if err != nil {
		logx.WithContext(ctx).Errorf("grantUserPremium - error: %v", err)
		return false, err
	}
	return true, nil
}
