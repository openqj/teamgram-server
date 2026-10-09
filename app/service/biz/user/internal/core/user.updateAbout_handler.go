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

// UserUpdateAbout
// user.updateAbout user_id:long about:string = Bool;
func (c *UserCore) UserUpdateAbout(in *user.TLUserUpdateAbout) (*mtproto.Bool, error) {
	err := c.svcCtx.Dao.UpdateUserAbout(
		c.ctx,
		in.GetUserId(),
		in.GetAbout())

	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
