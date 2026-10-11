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

// UserGetLastSeen
// user.getLastSeen id:long = LastSeenData;
func (c *UserCore) UserGetLastSeen(in *user.TLUserGetLastSeen) (*user.LastSeenData, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	if c.svcCtx.Dao.Postgres.Store.Presences == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	do, err := c.svcCtx.Dao.GetLastSeenAt(c.ctx, in.Id)
	if err != nil {
		return nil, err
	}
	if do == nil {
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("user.getLastSeen - error: %d", err)
		return nil, err
	}

	return user.MakeTLLastSeenData(&user.LastSeenData{
		UserId:     in.Id,
		LastSeenAt: do.LastSeenAt,
		Expires:    do.Expires,
	}).To_LastSeenData(), nil
}
