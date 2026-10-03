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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetFullUser
// user.getFullUser self_user_id:long id:long = users.UserFull;
func (c *UserCore) UserGetFullUser(in *user.TLUserGetFullUser) (*mtproto.Users_UserFull, error) {
	if in == nil || in.GetSelfUserId() <= 0 || in.GetId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}

	selfUser, err := c.svcCtx.Dao.GetImmutableUser(c.ctx, in.GetSelfUserId(), true, in.GetId())
	if err != nil {
		c.Logger.Errorf("user.getFullUser - load self user: %v", err)
		return nil, err
	}
	peerUser, err := c.svcCtx.Dao.GetImmutableUser(c.ctx, in.GetId(), true, in.GetSelfUserId())
	if err != nil {
		c.Logger.Errorf("user.getFullUser - load peer user: %v", err)
		return nil, err
	}
	if selfUser == nil || selfUser.GetUser() == nil || peerUser == nil || peerUser.GetUser() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}

	full := mtproto.MakeTLUserFull(&mtproto.UserFull{
		Id:             in.GetId(),
		About:          peerUser.GetUser().GetAbout(),
		ProfilePhoto:   peerUser.GetUser().GetProfilePhoto(),
		Settings:       nil,
		NotifySettings: nil,
		BotInfo:        nil,
		Birthday:       nil,
		MainTab:        peerUser.GetUser().GetMainTab(),
		SavedMusic:     nil,
	}).To_UserFull()

	if in.GetSelfUserId() != in.GetId() {
		full.Blocked = c.svcCtx.Dao.CheckBlocked(c.ctx, in.GetSelfUserId(), in.GetId())
	}
	full.Settings, err = c.UserGetPeerSettings(&user.TLUserGetPeerSettings{
		UserId:   in.GetSelfUserId(),
		PeerType: mtproto.PEER_USER,
		PeerId:   in.GetId(),
	})
	if err != nil {
		return nil, err
	}
	full.NotifySettings, err = c.UserGetNotifySettings(&user.TLUserGetNotifySettings{
		UserId:   in.GetSelfUserId(),
		PeerType: mtproto.PEER_USER,
		PeerId:   in.GetId(),
	})
	if err != nil {
		return nil, err
	}

	if peerUser.CheckPrivacy(mtproto.BIRTHDAY, in.GetSelfUserId()) {
		full.Birthday = peerUser.Birthday()
	}
	if peerUser.GetUser().GetSavedMusic() != nil &&
		peerUser.CheckPrivacy(mtproto.SAVED_MUSIC, in.GetSelfUserId()) {
		full.SavedMusic = peerUser.GetUser().GetSavedMusic()
	}
	if peerUser.IsBot() {
		full.BotInfo, err = c.UserGetBotInfo(&user.TLUserGetBotInfo{BotId: in.GetId()})
		if err != nil {
			return nil, err
		}
	}

	return mtproto.MakeTLUsersUserFull(&mtproto.Users_UserFull{
		FullUser: full,
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{peerUser.ToUnsafeUser(selfUser)},
	}).To_Users_UserFull(), nil
}
