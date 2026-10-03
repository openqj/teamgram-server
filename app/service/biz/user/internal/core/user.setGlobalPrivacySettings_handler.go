/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2024-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserSetGlobalPrivacySettings
// user.setGlobalPrivacySettings user_id:int settings:GlobalPrivacySettings = Bool;
func (c *UserCore) UserSetGlobalPrivacySettings(in *user.TLUserSetGlobalPrivacySettings) (*mtproto.Bool, error) {
	if in == nil {
		return nil, fmt.Errorf("user.setGlobalPrivacySettings: request is nil")
	}
	settings := in.GetSettings()
	if settings == nil {
		return nil, fmt.Errorf("user.setGlobalPrivacySettings: settings is nil")
	}

	var (
		archiveAndMuteNewNoncontactPeers bool
	)

	if settings.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOL() != nil {
		archiveAndMuteNewNoncontactPeers = mtproto.FromBool(settings.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOL())
	} else {
		archiveAndMuteNewNoncontactPeers = settings.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN()
	}
	disallowedGifts, err := encodeGlobalPrivacyDisallowedGifts(settings.GetDisallowedGifts())
	if err != nil {
		c.Logger.Errorf("user.setGlobalPrivacySettings - error: %v", err)
		return nil, fmt.Errorf("user.setGlobalPrivacySettings: encode settings: %w", err)
	}

	if _, _, err := c.svcCtx.Dao.UserGlobalPrivacySettingsDAO.InsertOrUpdate(c.ctx, &dataobject.UserGlobalPrivacySettingsDO{
		UserId:                           in.UserId,
		ArchiveAndMuteNewNoncontactPeers: archiveAndMuteNewNoncontactPeers,
		KeepArchivedUnmuted:              settings.GetKeepArchivedUnmuted(),
		KeepArchivedFolders:              settings.GetKeepArchivedFolders(),
		HideReadMarks:                    settings.GetHideReadMarks(),
		NewNoncontactPeersRequirePremium: settings.GetNewNoncontactPeersRequirePremium(),
		DisplayGiftsButton:               settings.GetDisplayGiftsButton(),
		NoncontactPeersPaidStars:         globalPrivacyPaidStars(settings),
		DisallowedGiftsJSON:              disallowedGifts,
	}); err != nil {
		c.Logger.Errorf("user.setGlobalPrivacySettings - error: %v", err)
		return nil, fmt.Errorf("user.setGlobalPrivacySettings: save settings: %w", err)
	}

	return mtproto.BoolTrue, nil
}
