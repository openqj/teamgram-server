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

// UserGetContactIdList
// user.getContactIdList user_id:long = Vector<long>;
func (c *UserCore) UserGetContactIdList(in *user.TLUserGetContactIdList) (*user.Vector_Long, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	rValList := &user.Vector_Long{
		Datas: []int64{},
	}

	cacheUserData, err := c.svcCtx.Dao.GetCacheUserDataWithError(c.ctx, in.GetUserId())
	if err != nil {
		return nil, err
	}
	if len(cacheUserData.GetContactIdList()) > 0 {
		rValList.Datas = cacheUserData.GetContactIdList()
	}

	return rValList, nil
}
