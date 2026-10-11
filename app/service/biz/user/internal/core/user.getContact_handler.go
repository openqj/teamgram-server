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

// UserGetContact
// user.getContact user_id:long id:long = ContactData;
func (c *UserCore) UserGetContact(in *user.TLUserGetContact) (*mtproto.ContactData, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetUserId() <= 0 || in.GetId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	contact, err := c.svcCtx.Dao.GetUserContact(c.ctx, in.GetUserId(), in.GetId())
	if err != nil {
		return nil, err
	}
	if contact == nil {
		err := mtproto.ErrContactIdInvalid
		c.Logger.Errorf("user.getContact - error: %v", err)
		return nil, err
	}

	return contact, nil
}
