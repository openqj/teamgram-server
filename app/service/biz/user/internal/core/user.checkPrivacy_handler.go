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

// UserCheckPrivacy
// user.checkPrivacy user_id:long key_type:int peer_id:long = Bool;
func (c *UserCore) UserCheckPrivacy(in *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	allowed, err := c.svcCtx.Dao.CheckUserPrivacy(c.ctx, in.GetUserId(), in.GetKeyType(), in.GetPeerId())
	if err != nil {
		return nil, err
	}
	return mtproto.ToBool(allowed), nil
}
