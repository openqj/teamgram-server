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

// UserUpdateProfilePhoto
// user.updateProfilePhoto user_id:long id:long = Int64;
func (c *UserCore) UserUpdateProfilePhoto(in *user.TLUserUpdateProfilePhoto) (*mtproto.Int64, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetId() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	rV, err := c.svcCtx.Dao.UpdateProfilePhoto(
		c.ctx,
		in.GetUserId(),
		in.GetId())
	if err != nil {
		c.Logger.Errorf("user.updateProfilePhoto - error: %v", err)
		return nil, err
	}

	return &mtproto.Int64{
		V: rV,
	}, nil
}
