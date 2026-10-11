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

// UserIsBlockedByUser
// user.isBlockedByUser user_id:long peer_user_id:long = Bool;
func (c *UserCore) UserIsBlockedByUser(in *user.TLUserIsBlockedByUser) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetUserId() <= 0 || in.GetPeerUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	blocked, err := c.svcCtx.Dao.CheckBlocked(c.ctx, in.GetUserId(), in.GetPeerUserId())
	if err != nil {
		return nil, err
	}

	return mtproto.ToBool(blocked), nil
}
