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
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserIsBot
// user.isBot id:long = Bool;
func (c *UserCore) UserIsBot(in *user.TLUserIsBot) (*mtproto.Bool, error) {
	userData, err := c.svcCtx.Dao.GetCacheUserDataWithError(c.ctx, in.GetId())
	if errors.Is(err, sqlc.ErrNotFound) {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err != nil {
		return nil, err
	}
	if userData == nil {
		c.Logger.Errorf("user.isBot - error: invalid user(%d)", in.GetId())
		return nil, mtproto.ErrUserIdInvalid
	}

	return mtproto.ToBool(userData.GetUserData().GetBot() != nil), nil
}
