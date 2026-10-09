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

// UserGetProfilePhotos
// user.getProfilePhotos user_id:long = Vector<long>;
func (c *UserCore) UserGetProfilePhotos(in *user.TLUserGetProfilePhotos) (*user.Vector_Long, error) {
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	idList, err := c.svcCtx.Dao.Postgres.Store.ProfilePhotos.SelectList(c.ctx, in.UserId)
	if err != nil {
		c.Logger.Errorf("user.getProfilePhotos - error: %v", err)
		return nil, err
	}

	return &user.Vector_Long{
		Datas: idList,
	}, nil
}
