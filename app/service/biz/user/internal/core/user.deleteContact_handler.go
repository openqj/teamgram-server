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

// UserDeleteContact
// user.deleteContact user_id:long id:long = Bool;
// id=0 is reserved for the internal full-contact reset request.
func (c *UserCore) UserDeleteContact(in *user.TLUserDeleteContact) (*mtproto.Bool, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetId() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetId() == 0 {
		if err := c.svcCtx.Dao.ResetUserContacts(c.ctx, in.GetUserId()); err != nil {
			c.Logger.Errorf("user.deleteContact - reset contacts error: %v", err)
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}

	if err := c.svcCtx.Dao.DeleteUserContact(c.ctx, in.GetUserId(), in.GetId()); err != nil {
		c.Logger.Errorf("user.deleteContact - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
