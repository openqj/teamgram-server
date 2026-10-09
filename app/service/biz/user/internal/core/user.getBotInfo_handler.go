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
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// UserGetBotInfo
// user.getBotInfo bot_id:long = BotInfo;
func (c *UserCore) UserGetBotInfo(in *user.TLUserGetBotInfo) (*mtproto.BotInfo, error) {
	botsDO, err := c.svcCtx.Dao.SelectBot(c.ctx, in.BotId)
	if err != nil {
		c.Logger.Errorf("user.getBotInfo - error: %v", err)
		return nil, err
	} else if botsDO == nil {
		return nil, mtproto.ErrUserIdInvalid
	}

	botInfo := mtproto.MakeTLBotInfo(&mtproto.BotInfo{
		HasPreviewMedias:       botsDO.HasPreviewMedias,
		UserId_INT64:           in.BotId,
		UserId_FLAGINT64:       mtproto.MakeFlagsInt64(in.BotId),
		Description_STRING:     botsDO.Description,
		Description_FLAGSTRING: mtproto.MakeFlagsString(botsDO.Description),
		DescriptionPhoto:       nil,
		DescriptionDocument:    nil,
		Commands:               []*mtproto.BotCommand{},
		MenuButton:             nil,
		PrivacyPolicyUrl:       mtproto.MakeFlagsString(botsDO.PrivacyPolicyUrl),
		AppSettings:            nil,
	}).To_BotInfo()

	// TODO: HasPreviewMedias

	// Commands
	commands, err := c.svcCtx.Dao.SelectBotCommands(c.ctx, in.BotId)
	if err != nil {
		c.Logger.Errorf("user.getBotInfo - load bot(%d) commands error: %v", in.BotId, err)
		return nil, err
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Id < commands[j].Id
	})
	for i := range commands {
		botInfo.Commands = append(botInfo.Commands, mtproto.MakeTLBotCommand(&mtproto.BotCommand{
			Command:     commands[i].Command,
			Description: commands[i].Description,
		}).To_BotCommand())
	}

	// MenuButton
	if botsDO.HasMenuButton {
		botInfo.MenuButton = mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{
			Text: botsDO.MenuButtonText,
			Url:  botsDO.MenuButtonUrl,
		}).To_BotMenuButton()
	}

	// DescriptionPhoto
	if botsDO.DescriptionPhotoId != 0 {
		botInfo.DescriptionPhoto, _ = c.svcCtx.Dao.MediaClient.MediaGetPhoto(c.ctx, &media.TLMediaGetPhoto{
			PhotoId: botsDO.DescriptionPhotoId,
		})
	}

	// AppSettings
	if botsDO.HasAppSettings {
		botInfo.AppSettings = mtproto.MakeTLBotAppSettings(&mtproto.BotAppSettings{
			PlaceholderPath:     nil, // TODO: botsDO.PlaceholderPath,
			BackgroundColor:     mtproto.MakeFlagsInt32(botsDO.BackgroundColor),
			BackgroundDarkColor: mtproto.MakeFlagsInt32(botsDO.BackgroundDarkColor),
			HeaderColor:         mtproto.MakeFlagsInt32(botsDO.HeaderColor),
			HeaderDarkColor:     mtproto.MakeFlagsInt32(botsDO.HeaderDarkColor),
		}).To_BotAppSettings()
	}

	return botInfo, nil
}
