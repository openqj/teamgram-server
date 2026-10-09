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

// UserSetPrivacy
// user.setPrivacy user_id:int key_type:int rules:Vector<PrivacyRule> = Bool;
func (c *UserCore) UserSetPrivacy(in *user.TLUserSetPrivacy) (*mtproto.Bool, error) {
	if err := c.svcCtx.Dao.SetUserPrivacyRules(
		c.ctx,
		in.GetUserId(),
		in.GetKeyType(),
		in.GetRules()); err != nil {
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
