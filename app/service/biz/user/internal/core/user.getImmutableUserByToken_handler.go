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

// UserGetImmutableUserByToken
// user.getImmutableUserByToken token:string = ImmutableUser;
func (c *UserCore) UserGetImmutableUserByToken(in *user.TLUserGetImmutableUserByToken) (*mtproto.ImmutableUser, error) {
	// TODO: performance optimization
	botId, err := c.svcCtx.Dao.BotsDAO.SelectByToken(c.ctx, in.Token)
	if err != nil {
		return nil, err
	}
	if botId == 0 {
		return nil, mtproto.ErrTokenInvalid
	}

	return c.UserGetImmutableUser(&user.TLUserGetImmutableUser{
		Id: botId,
	})
}
