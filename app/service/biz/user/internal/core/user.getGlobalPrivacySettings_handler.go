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
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetGlobalPrivacySettings
// user.getGlobalPrivacySettings user_id:int = GlobalPrivacySettings;
func (c *UserCore) UserGetGlobalPrivacySettings(in *user.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	if in == nil {
		return nil, fmt.Errorf("user.getGlobalPrivacySettings: request is nil")
	}

	var (
		rV = mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
			ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: false,
			ArchiveAndMuteNewNoncontactPeers_FLAGBOOL:    mtproto.BoolFalse,
			KeepArchivedUnmuted:                          false,
			KeepArchivedFolders:                          false,
			HideReadMarks:                                false,
			NewNoncontactPeersRequirePremium:             false,
		}).To_GlobalPrivacySettings()
	)

	// globalPrivacySettings#734c4ccb flags:#
	//	archive_and_mute_new_noncontact_peers:flags.0?true
	//	keep_archived_unmuted:flags.1?true
	//	keep_archived_folders:flags.2?true
	//	hide_read_marks:flags.3?true
	//	new_noncontact_peers_require_premium:flags.4?true = GlobalPrivacySettings;
	do, err := c.svcCtx.Dao.UserGlobalPrivacySettingsDAO.Select(c.ctx, in.UserId)
	if err != nil {
		c.Logger.Errorf("user.getGlobalPrivacySettings - error: %v", err)
		return nil, fmt.Errorf("user.getGlobalPrivacySettings: load settings: %w", err)
	} else if do == nil {
		c.Logger.Infof("user.getGlobalPrivacySettings - not found by %d", in.UserId)
		return rV, nil
	}
	disallowedGifts, err := decodeGlobalPrivacyDisallowedGifts(do.DisallowedGiftsJSON)
	if err != nil {
		c.Logger.Errorf("user.getGlobalPrivacySettings - error: %v", err)
		return nil, fmt.Errorf("user.getGlobalPrivacySettings: decode settings: %w", err)
	}

	return mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: do.ArchiveAndMuteNewNoncontactPeers,
		ArchiveAndMuteNewNoncontactPeers_FLAGBOOL:    mtproto.ToBool(do.ArchiveAndMuteNewNoncontactPeers),
		KeepArchivedUnmuted:                          do.KeepArchivedUnmuted,
		KeepArchivedFolders:                          do.KeepArchivedFolders,
		HideReadMarks:                                do.HideReadMarks,
		NewNoncontactPeersRequirePremium:             do.NewNoncontactPeersRequirePremium,
		DisplayGiftsButton:                           do.DisplayGiftsButton,
		NoncontactPeersPaidStars:                     globalPrivacyPaidStarsValue(do.NoncontactPeersPaidStars),
		DisallowedGifts:                              disallowedGifts,
	}).To_GlobalPrivacySettings(), nil
}
