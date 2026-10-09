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

// UserGetPrivacy
// user.getPrivacy user_id:int key_type:int = Vector<PrivacyRule>;
func (c *UserCore) UserGetPrivacy(in *user.TLUserGetPrivacy) (*user.Vector_PrivacyRule, error) {
	rules, err := c.svcCtx.Dao.GetUserPrivacyRules(c.ctx, in.GetUserId(), in.GetKeyType())
	if err != nil {
		return nil, err
	}
	if rules == nil {
		err := mtproto.ErrPrivacyKeyInvalid
		c.Logger.Errorf("user.getPrivacy - error: %v", err)
		return nil, err
	}

	return &user.Vector_PrivacyRule{
		Datas: rules.GetRules(),
	}, nil
}
