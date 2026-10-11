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

// UserGetContactList
// user.getContactList user_id:long = Vector<ContactData>;
func (c *UserCore) UserGetContactList(in *user.TLUserGetContactList) (*user.Vector_ContactData, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	rValList := &user.Vector_ContactData{}

	var err error
	rValList.Datas, err = c.svcCtx.Dao.GetUserContactList(c.ctx, in.GetUserId())
	if err != nil {
		return nil, err
	}
	if rValList.Datas == nil {
		rValList.Datas = []*mtproto.ContactData{}
	}

	return rValList, nil
}
