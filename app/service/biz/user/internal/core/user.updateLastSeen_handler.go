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

// UserUpdateLastSeen
// user.updateLastSeen id:long last_seen_at:long expires:int = Bool;
func (c *UserCore) UserUpdateLastSeen(in *user.TLUserUpdateLastSeen) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	if err := c.svcCtx.Dao.PutLastSeenAt(
		c.ctx,
		in.GetId(),
		in.GetLastSeenAt(),
		in.GetExpires()); err != nil {
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
