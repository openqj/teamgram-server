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
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetContentSettings
// user.getContentSettings user_id:long = account.ContentSettings;
func (c *UserCore) UserGetContentSettings(in *user.TLUserGetContentSettings) (*mtproto.Account_ContentSettings, error) {
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	var (
		rV = false
	)

	var do *dataobject.UserSettingsDO
	var err error
	do, err = c.svcCtx.Dao.Postgres.Store.Settings.SelectByKey(c.ctx, in.UserId, "sensitive_enabled")
	if err != nil {
		c.Logger.Errorf("user.getContentSettings - error: %v", err)
		return nil, err
	}
	if do != nil {
		if do.Value == "true" {
			rV = true
		}
	}

	return mtproto.MakeTLAccountContentSettings(&mtproto.Account_ContentSettings{
		SensitiveEnabled:   rV,
		SensitiveCanChange: true,
	}).To_Account_ContentSettings(), nil
}
