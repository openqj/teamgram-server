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
	"github.com/teamgram/marmota/pkg/utils"
	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/mr"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func getFullUserUsers(mutableUsers *mtproto.MutableUsers, selfId, peerId int64) (*mtproto.ImmutableUser, *mtproto.ImmutableUser, error) {
	if mutableUsers != nil {
		var me, peer *mtproto.ImmutableUser
		for _, candidate := range mutableUsers.GetUsers() {
			if candidate == nil || candidate.GetUser() == nil {
				continue
			}
			id := candidate.GetUser().GetId()
			if id == selfId {
				me = candidate
			}
			if id == peerId {
				peer = candidate
			}
		}
		if me != nil && peer != nil {
			return me, peer, nil
		}
	}

	return nil, nil, mtproto.ErrInternalServerError
}

func (c *UsersCore) checkFullUserPrivacy(owner *mtproto.ImmutableUser, ownerId, viewerId int64, rules []*mtproto.PrivacyRule) (bool, error) {
	if owner == nil || owner.GetUser() == nil {
		return false, mtproto.ErrInternalServerError
	}

	var groupIds []int64
	for _, rule := range rules {
		if rule == nil {
			return false, mtproto.ErrInternalServerError
		}
		switch rule.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowChatParticipants, mtproto.Predicate_privacyValueDisallowChatParticipants:
			for _, id := range rule.GetChats() {
				if id <= 0 {
					return false, mtproto.ErrInternalServerError
				}
				if !mtproto.ChatIdIsChat(id) {
					return false, mtproto.ErrMethodNotImpl
				}
				groupIds = append(groupIds, id)
			}
		}
	}

	memberChatIds := make(map[int64]struct{})
	if len(groupIds) > 0 {
		memberships, err := c.svcCtx.Dao.ChatClient.ChatGetUsersChatIdList(c.ctx, &chatpb.TLChatGetUsersChatIdList{
			Id: []int64{viewerId},
		})
		if err != nil {
			return false, err
		}
		if memberships == nil {
			return false, mtproto.ErrInternalServerError
		}
		for _, userChats := range memberships.GetDatas() {
			if userChats == nil {
				return false, mtproto.ErrInternalServerError
			}
			if userChats.GetUserId() != viewerId {
				continue
			}
			for _, id := range userChats.GetChatIdList() {
				if !mtproto.ChatIdIsChat(id) {
					return false, mtproto.ErrInternalServerError
				}
				memberChatIds[id] = struct{}{}
			}
		}
	}

	allowed := mtproto.CheckPrivacyIsAllow(
		ownerId,
		rules,
		viewerId,
		func(_, checkId int64) bool {
			contact, _ := owner.CheckContact(checkId)
			return contact
		},
		func(checkId int64, chatIds []int64) bool {
			if checkId != viewerId {
				return false
			}
			for _, id := range chatIds {
				if _, ok := memberChatIds[id]; ok {
					return true
				}
			}
			return false
		})
	return allowed, nil
}

// UsersGetFullUser
// users.getFullUser#b60f5918 id:InputUser = users.UserFull;
func (c *UsersCore) UsersGetFullUser(in *mtproto.TLUsersGetFullUser) (*mtproto.Users_UserFull, error) {
	if in == nil || in.Id == nil {
		return nil, mtproto.ErrUserIdInvalid
	}

	var (
		peerId int64
		id     = mtproto.FromInputUser(c.MD.UserId, in.Id)
		me     *mtproto.ImmutableUser
		user   *mtproto.ImmutableUser
	)

	switch id.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER:
		peerId = id.PeerId
	default:
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	}

	mutableUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      []int64{c.MD.UserId, peerId},
		Privacy: true,
		HasTo:   true,
		To:      []int64{c.MD.UserId, peerId},
	})
	if err != nil {
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	}

	me, user, err = getFullUserUsers(mutableUsers, c.MD.UserId, peerId)
	if err != nil {
		c.Logger.Errorf("users.getFullUser - error: %v", err)
		return nil, err
	}

	userFull := mtproto.MakeTLUserFull(&mtproto.UserFull{
		Blocked:                  false,
		PhoneCallsAvailable:      c.MD.UserId != peerId,
		PhoneCallsPrivate:        false,
		CanPinMessage:            true,
		HasScheduled:             false,
		VideoCallsAvailable:      c.MD.UserId != peerId,
		VoiceMessagesForbidden:   false,
		TranslationsDisabled:     false,
		StoriesPinnedAvailable:   false,
		BlockedMyStoriesFrom:     false,
		WallpaperOverridden:      false,
		Id:                       peerId,
		About:                    user.GetUser().GetAbout(),
		Settings:                 nil,
		PersonalPhoto:            nil,
		ProfilePhoto:             user.GetUser().GetProfilePhoto(),
		FallbackPhoto:            nil,
		NotifySettings:           nil,
		BotInfo:                  nil,
		PinnedMsgId:              nil,
		CommonChatsCount:         0,
		FolderId:                 nil,
		TtlPeriod:                nil,
		ThemeEmoticon:            nil,
		Theme:                    nil,
		PrivateForwardName:       nil,
		BotGroupAdminRights:      nil,
		BotBroadcastAdminRights:  nil,
		PremiumGifts:             nil,
		Wallpaper:                nil,
		Stories_FLAGPEERSTORIES:  nil,
		Stories_FLAGUSERSTORIES:  nil,
		BusinessWorkHours:        nil,
		BusinessLocation:         nil,
		BusinessGreetingMessage:  nil,
		BusinessAwayMessage:      nil,
		BusinessIntro:            nil,
		Birthday:                 nil,
		PersonalChannelId:        nil,
		PersonalChannelMessage:   nil,
		StargiftsCount:           nil,
		StarrefProgram:           nil,
		BotVerification:          nil,
		SendPaidMessagesStars:    nil,
		DisallowedGifts:          nil,
		StarsMyPendingRating:     nil,
		StarsMyPendingRatingDate: nil,
		MainTab:                  user.GetUser().GetMainTab(),
		SavedMusic:               nil,
		Note:                     nil,
	}).To_UserFull()

	// PremiumGifts
	if user.Premium() {
		// TODO: config able
		userFull.PremiumGifts = []*mtproto.PremiumGiftOption{
			mtproto.MakeTLPremiumGiftOption(&mtproto.PremiumGiftOption{
				Months:       12,
				Currency:     "CNY",
				Amount:       20900,
				BotUrl:       "https://t.me/$premgift448603711_12_5248da16f536f717a2",
				StoreProduct: mtproto.MakeFlagsString("org.telegram.telegramPremium.twelveMonths"),
			}).To_PremiumGiftOption(),
			mtproto.MakeTLPremiumGiftOption(&mtproto.PremiumGiftOption{
				Months:       6,
				Currency:     "CNY",
				Amount:       10900,
				BotUrl:       "https://t.me/$premgift448603711_6_c7aae8edbdae927b72",
				StoreProduct: mtproto.MakeFlagsString("org.telegram.telegramPremium.sixMonths"),
			}).To_PremiumGiftOption(),
			mtproto.MakeTLPremiumGiftOption(&mtproto.PremiumGiftOption{
				Months:       3,
				Currency:     "CNY",
				Amount:       8499,
				BotUrl:       "https://t.me/$premgift448603711_3_051b80db4901b91dd5",
				StoreProduct: mtproto.MakeFlagsString("org.telegram.telegramPremium.threeMonths"),
			}).To_PremiumGiftOption(),
		}
	}
	err = mr.Finish(
		func() error {
			// blocked
			if c.MD.UserId != peerId {
				blocked, err := c.svcCtx.Dao.UserClient.UserBlockedByUser(
					c.ctx,
					&userpb.TLUserBlockedByUser{
						UserId:     c.MD.UserId,
						PeerUserId: peerId,
					})
				if err != nil {
					c.Logger.Errorf("users.getFullUser - error: %v", err)
					return err
				}
				if blocked == nil {
					return mtproto.ErrInternalServerError
				}
				userFull.Blocked = mtproto.FromBool(blocked)
			}
			return nil
		},
		func() error {
			settings, err := c.svcCtx.Dao.UserClient.UserGetPeerSettings(c.ctx, &userpb.TLUserGetPeerSettings{
				UserId:   c.MD.UserId,
				PeerType: mtproto.PEER_USER,
				PeerId:   peerId,
			})
			if err != nil {
				c.Logger.Errorf("users.getFullUser - error: %v", err)
				return err
			}
			if settings == nil {
				return mtproto.ErrInternalServerError
			}
			userFull.Settings = settings
			return nil
		},
		func() error {
			settings, err := c.svcCtx.Dao.UserClient.UserGetNotifySettings(c.ctx, &userpb.TLUserGetNotifySettings{
				UserId:   c.MD.UserId,
				PeerType: mtproto.PEER_USER,
				PeerId:   peerId,
			})
			if err != nil {
				c.Logger.Errorf("users.getFullUser - error: %v", err)
				return err
			}
			if settings == nil {
				return mtproto.ErrInternalServerError
			}
			userFull.NotifySettings = settings
			return nil
		},
		func() error {
			if user.GetUser().GetBot() != nil {
				userFull.PhoneCallsAvailable = false
				userFull.PhoneCallsPrivate = false
				userFull.VideoCallsAvailable = false
				botInfo, err := c.svcCtx.Dao.UserClient.UserGetBotInfo(c.ctx, &userpb.TLUserGetBotInfo{
					BotId: peerId,
				})
				if err != nil {
					c.Logger.Errorf("users.getFullUser - error: %v", err)
					return err
				}
				if botInfo == nil {
					return mtproto.ErrInternalServerError
				}
				userFull.BotInfo = botInfo
			}
			return nil
		},
		func() error {
			// TODO: PinnedMsgId:         nil,
			if c.MD.UserId != peerId {
				usersChatIdList, err := c.svcCtx.Dao.ChatClient.ChatGetUsersChatIdList(c.ctx, &chatpb.TLChatGetUsersChatIdList{
					Id: []int64{c.MD.UserId, peerId},
				})
				if err != nil {
					c.Logger.Errorf("users.getFullUser - error: %v", err)
					return err
				}
				if usersChatIdList == nil {
					return mtproto.ErrInternalServerError
				}
				if len(usersChatIdList.Datas) == 2 {
					if usersChatIdList.Datas[0] == nil || usersChatIdList.Datas[1] == nil {
						return mtproto.ErrInternalServerError
					}
					commonChats := utils.Int64Intersect(
						usersChatIdList.Datas[0].ChatIdList,
						usersChatIdList.Datas[1].ChatIdList)
					userFull.CommonChatsCount = int32(len(commonChats))
				}
				// TODO: Fetch CommonChannelsCount
			}
			return nil
		},
		func() error {
			if peerId != c.MD.UserId {
				// theme_emoticon
				dialogExt, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
					UserId:   c.MD.UserId,
					PeerType: mtproto.PEER_USER,
					PeerId:   peerId,
				})
				if err != nil {
					c.Logger.Errorf("users.getFullUser - error: %v", err)
					return err
				}
				if dialogExt != nil {
					userFull.ThemeEmoticon = mtproto.MakeFlagsString(dialogExt.ThemeEmoticon)
					userFull.TtlPeriod = mtproto.MakeFlagsInt32(dialogExt.TtlPeriod)
					if dialogExt.WallpaperId != 0 && c.svcCtx.Dao.WallpaperPlugin != nil {
						userFull.Wallpaper = c.svcCtx.Dao.WallpaperPlugin.GetChatWallpaper(c.ctx, c.MD.UserId, dialogExt.WallpaperId)
						userFull.WallpaperOverridden = true
					}
				}
			}
			return nil
		},
		func() error {
			rules, err := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
				UserId:  peerId,
				KeyType: mtproto.VOICE_MESSAGES,
			})
			if err != nil {
				c.Logger.Errorf("users.getFullUser - error: %v", err)
				return err
			}
			if rules == nil {
				return mtproto.ErrInternalServerError
			}
			if rules != nil && len(rules.Datas) > 0 {
				allowed, err := c.checkFullUserPrivacy(user, peerId, c.MD.UserId, rules.Datas)
				if err != nil {
					c.Logger.Errorf("users.getFullUser - error: %v", err)
					return err
				}
				userFull.VoiceMessagesForbidden = !allowed
			}
			return nil
		},
		func() error {
			if user.GetUser().GetSavedMusic() != nil {
				rules, err := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
					UserId:  peerId,
					KeyType: mtproto.SAVED_MUSIC,
				})
				if err != nil {
					c.Logger.Errorf("users.getFullUser - error: %v", err)
					return err
				}
				if rules == nil {
					return mtproto.ErrInternalServerError
				}
				if rules != nil && len(rules.Datas) > 0 {
					allowed, err := c.checkFullUserPrivacy(user, peerId, c.MD.UserId, rules.Datas)
					if err != nil {
						c.Logger.Errorf("users.getFullUser - error: %v", err)
						return err
					}
					if allowed {
						userFull.SavedMusic = user.GetUser().GetSavedMusic()
					}
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	// TODO: FolderId:    0,

	// TODO: WallPaper

	// TODO: Stories
	if c.svcCtx.Dao.StoryPlugin != nil {
		userFull.StoriesPinnedAvailable = c.svcCtx.Dao.StoryPlugin.GetStoriesPinnedAvailable(c.ctx, peerId, c.MD.UserId)
		userFull.BlockedMyStoriesFrom = c.svcCtx.Dao.StoryPlugin.GetBlockedMyStoriesFrom(c.ctx, peerId, c.MD.UserId)
		// c.Logger.Debugf("getActiveStories: peerId: %s, userId: %s", user, me)
		if peerId == c.MD.UserId {
			// c.Logger.Debugf("getActiveStories(peerId == c.MD.UserId): peerId: %d, userId: %d", peerId, c.MD.UserId)
			userFull.Stories_FLAGPEERSTORIES = c.svcCtx.Dao.StoryPlugin.GetActiveStories(c.ctx, peerId, c.MD.UserId)
		} else if ok, _ := me.CheckReverseContact(peerId); ok {
			// c.Logger.Debugf("getActiveStories(ok, _ := user.CheckContact(c.MD.UserId)): peerId: %d, userId: %d", peerId, c.MD.UserId)
			userFull.Stories_FLAGPEERSTORIES = c.svcCtx.Dao.StoryPlugin.GetActiveStories(c.ctx, peerId, c.MD.UserId)
		}
	}

	chats := make([]*mtproto.Chat, 0)

	if c.svcCtx.Dao.PersonalChannelPlugin != nil {
		personalChannelId := user.GetUser().GetPersonalChannelId()
		if personalChannelId != 0 {
			userFull.PersonalChannelId = mtproto.MakeFlagsInt64(personalChannelId)
			pChannel, topMessageId := c.svcCtx.Dao.PersonalChannelPlugin.GetPersonalChannel(c.ctx, personalChannelId, c.MD.UserId)
			if pChannel != nil {
				userFull.PersonalChannelMessage = &wrapperspb.Int32Value{Value: topMessageId}
				chats = append(chats, pChannel)
			}
		}
	}

	if c.MD.UserId != peerId {
		// userFull.Birthday = user.Birthday()
		if user.Birthday() != nil {
			//if user.GetUser().GetSavedMusic() != nil {
			rules, err := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
				UserId:  peerId,
				KeyType: mtproto.BIRTHDAY,
			})
			if err != nil {
				c.Logger.Errorf("users.getFullUser - error: %v", err)
				return nil, err
			}
			if rules == nil {
				return nil, mtproto.ErrInternalServerError
			}
			if rules != nil && len(rules.Datas) > 0 {
				allowed, err := c.checkFullUserPrivacy(user, peerId, c.MD.UserId, rules.Datas)
				if err != nil {
					c.Logger.Errorf("users.getFullUser - error: %v", err)
					return nil, err
				}
				if allowed {
					userFull.Birthday = user.Birthday()
				}
			}
			//}
		}
	} else {
		userFull.Birthday = user.Birthday()
	}

	if c.MD.UserId == 777000 {
		userFull.PhoneCallsAvailable = false
		userFull.PhoneCallsPrivate = false
		userFull.VideoCallsAvailable = false
	}

	unsafeUser := user.ToUnsafeUser(me)
	patchBotUsernameFromImmutable(unsafeUser, user)

	return mtproto.MakeTLUsersUserFull(&mtproto.Users_UserFull{
		FullUser: userFull,
		Chats:    chats,
		Users:    []*mtproto.User{unsafeUser},
	}).To_Users_UserFull(), nil
}
