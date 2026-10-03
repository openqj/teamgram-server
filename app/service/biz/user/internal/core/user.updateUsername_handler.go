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

// UserUpdateUsername
// user.updateUsername user_id:long username:string = Bool;
func (c *UserCore) UserUpdateUsername(in *user.TLUserUpdateUsername) (*mtproto.Bool, error) {
	if err := c.svcCtx.Dao.UpdateUserUsername(
		c.ctx,
		in.GetUserId(),
		in.GetUsername()); err != nil {
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
