// Copyright 2026 Teamgram Authors
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
	"strings"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCBotsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) BotsSetBotCommands(in *mtproto.TLBotsSetBotCommands) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = validateBotCommandScope(in.GetScope(), in.GetLangCode()); err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	return c.svcCtx.Dao.UserSetBotCommands(c.ctx, &userpb.TLUserSetBotCommands{
		UserId:   userId,
		BotId:    userId,
		Commands: in.GetCommands(),
	})
}

func (c *ApiFullCore) BotsResetBotCommands(in *mtproto.TLBotsResetBotCommands) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = validateBotCommandScope(in.GetScope(), in.GetLangCode()); err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	return c.svcCtx.Dao.UserSetBotCommands(c.ctx, &userpb.TLUserSetBotCommands{
		UserId: userId,
		BotId:  userId,
	})
}

func (c *ApiFullCore) BotsGetBotCommands(in *mtproto.TLBotsGetBotCommands) (*mtproto.Vector_BotCommand, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = validateBotCommandScope(in.GetScope(), in.GetLangCode()); err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	info, err := c.svcCtx.Dao.UserGetBotInfo(c.ctx, &userpb.TLUserGetBotInfo{BotId: userId})
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, mtproto.ErrInternalServerError
	}
	commands := info.GetCommands()
	if commands == nil {
		commands = []*mtproto.BotCommand{}
	}
	return &mtproto.Vector_BotCommand{Datas: commands}, nil
}

func validateBotCommandScope(scope *mtproto.BotCommandScope, langCode string) error {
	if scope == nil {
		return mtproto.ErrInputConstructorInvalid
	}
	if scope.GetPredicateName() != mtproto.Predicate_botCommandScopeDefault || langCode != "" {
		return mtproto.ErrMethodNotImpl
	}
	return nil
}

func (c *ApiFullCore) BotsSetBotInfo(in *mtproto.TLBotsSetBotInfo) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetLangCode() != "" {
		return nil, mtproto.ErrMethodNotImpl
	}
	bot := in.GetBot()
	switch bot.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
	case mtproto.Predicate_inputUser:
		if bot.GetUserId() != userID || bot.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.BotRegistryClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil || profile.Deleted() {
		return nil, mtproto.ErrBotInvalid
	}
	if bot.GetPredicateName() == mtproto.Predicate_inputUser && profile.GetUser().GetAccessHash() != bot.GetAccessHash() {
		return nil, mtproto.ErrUserIdInvalid
	}
	return c.svcCtx.Dao.BotRegistryClient.SetBotInfo(c.ctx, &userpb.BotRegistrySetBotInfoRequest{
		BotId:       userID,
		Name:        in.GetName(),
		About:       in.GetAbout(),
		Description: in.GetDescription(),
	})
}

func (c *ApiFullCore) BotsGetBotInfoDCD914FD(in *mtproto.TLBotsGetBotInfoDCD914FD) (*mtproto.Bots_BotInfo, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	bot := in.GetBot()
	var botID int64
	switch bot.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		botID = callerID
	case mtproto.Predicate_inputUser:
		botID = bot.GetUserId()
		if botID <= 0 || bot.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: botID})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	if bot.GetPredicateName() == mtproto.Predicate_inputUser && profile.GetUser().GetAccessHash() != bot.GetAccessHash() {
		return nil, mtproto.ErrUserIdInvalid
	}
	registryInfo, err := c.svcCtx.Dao.UserGetBotInfo(c.ctx, &userpb.TLUserGetBotInfo{BotId: botID})
	if err != nil {
		return nil, err
	}
	if registryInfo == nil {
		return nil, mtproto.ErrInternalServerError
	}
	name := strings.TrimSpace(profile.GetUser().GetFirstName() + " " + profile.GetUser().GetLastName())
	about := ""
	if profile.GetUser().GetAbout() != nil {
		about = profile.GetUser().GetAbout().GetValue()
	}
	return mtproto.MakeTLBotsBotInfo(&mtproto.Bots_BotInfo{
		Name:        name,
		About:       about,
		Description: registryInfo.GetDescription_STRING(),
	}).To_Bots_BotInfo(), nil
}

func (c *ApiFullCore) BotsGetAdminedBots(in *mtproto.TLBotsGetAdminedBots) (*mtproto.Vector_User, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.BotRegistryClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	admined, err := c.svcCtx.Dao.BotRegistryClient.GetAdminedBots(c.ctx, &userpb.BotRegistryGetAdminedBotsRequest{})
	if err != nil {
		return nil, err
	}
	if admined == nil {
		return nil, mtproto.ErrInternalServerError
	}
	users := &mtproto.Vector_User{Datas: make([]*mtproto.User, 0, len(admined.GetDatas()))}
	for _, bot := range admined.GetDatas() {
		if bot == nil || bot.GetUser() == nil || bot.GetUser().GetBot() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		users.Datas = append(users.Datas, bot.ToUser(userId))
	}
	return users, nil
}

func (c *ApiFullCore) BotsCheckUsername(in *mtproto.TLBotsCheckUsername) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	username := in.GetUsername()
	if !userpb.CheckUsernameInvalid(username) || !strings.HasSuffix(strings.ToLower(username), "bot") {
		return nil, mtproto.ErrUsernameInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	existed, err := c.svcCtx.Dao.UserCheckUsername(c.ctx, &userpb.TLUserCheckUsername{Username: username})
	if err != nil {
		return nil, err
	}
	if existed == nil {
		return nil, mtproto.ErrInternalServerError
	}
	switch existed.GetPredicateName() {
	case userpb.Predicate_usernameNotExisted:
		return mtproto.BoolTrue, nil
	case userpb.Predicate_usernameExisted, userpb.Predicate_usernameExistedNotMe, userpb.Predicate_usernameExistedIsMe:
		return mtproto.BoolFalse, nil
	default:
		return nil, mtproto.ErrInternalServerError
	}
}

func (c *ApiFullCore) BotsCreateBot(in *mtproto.TLBotsCreateBot) (*mtproto.User, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	username := in.GetUsername()
	if !userpb.CheckUsernameInvalid(username) || !strings.HasSuffix(strings.ToLower(username), "bot") {
		return nil, mtproto.ErrUsernameInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var (
		created   *mtproto.ImmutableUser
		createErr error
	)
	if in.GetViaDeeplink() {
		manager := in.GetManagerId()
		if manager == nil || manager.GetPredicateName() != mtproto.Predicate_inputUser || manager.GetUserId() <= 0 || manager.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		managerProfile, profileErr := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: manager.GetUserId()})
		if profileErr != nil {
			return nil, profileErr
		}
		if managerProfile == nil || managerProfile.GetUser() == nil || managerProfile.GetUser().GetBot() == nil ||
			managerProfile.GetUser().GetAccessHash() != manager.GetAccessHash() {
			return nil, mtproto.ErrForbiddenUserBotInvalid
		}
		created, createErr = c.svcCtx.Dao.UserCreateManagedBot(c.ctx, &userpb.BotRegistryCreateBotRequest{
			Name:              in.GetName(),
			Username:          username,
			ManagerBotId:      manager.GetUserId(),
			ManagerAccessHash: manager.GetAccessHash(),
		})
	} else {
		if manager := in.GetManagerId(); manager != nil && manager.GetPredicateName() != mtproto.Predicate_inputUserSelf {
			return nil, mtproto.ErrMethodNotImpl
		}
		created, createErr = c.svcCtx.Dao.UserCreateBot(c.ctx, &userpb.TLUserCreateBot{
			Name:     in.GetName(),
			Username: username,
		})
	}
	if createErr != nil {
		return nil, createErr
	}
	if created == nil || created.GetUser() == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return created.ToUser(userId), nil
}

func (c *ApiFullCore) BotsExportBotToken(in *mtproto.TLBotsExportBotToken) (*mtproto.Bots_ExportedBotToken, error) {
	callerId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	bot := in.GetBot()
	var botId int64
	switch bot.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		botId = callerId
	case mtproto.Predicate_inputUser:
		botId = bot.GetUserId()
		if botId <= 0 || bot.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	token, err := c.svcCtx.Dao.UserExportBotToken(c.ctx, &userpb.TLUserExportBotToken{
		BotId:      botId,
		AccessHash: bot.GetAccessHash(),
		Revoke:     mtproto.FromBool(in.GetRevoke()),
	})
	if err != nil {
		return nil, err
	}
	if token == nil || token.GetV() == "" {
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLBotsExportedBotToken(&mtproto.Bots_ExportedBotToken{Token: token.GetV()}).To_Bots_ExportedBotToken(), nil
}

func (c *ApiFullCore) BotsGetAccessSettings(in *mtproto.TLBotsGetAccessSettings) (*mtproto.Bots_AccessSettings, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsEditAccessSettings(in *mtproto.TLBotsEditAccessSettings) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsSetJoinChatResults(in *mtproto.TLBotsSetJoinChatResults) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsGetBotInfo75EC12E6(in *mtproto.TLBotsGetBotInfo75EC12E6) (*mtproto.Vector_String, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
