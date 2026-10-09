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

// UserSetAccountDaysTTL
// user.setAccountDaysTTL user_id:int ttl:int = Bool;
func (c *UserCore) UserSetAccountDaysTTL(in *user.TLUserSetAccountDaysTTL) (*mtproto.Bool, error) {
	if _, err := c.svcCtx.Dao.UpdateUserFields(c.ctx, in.UserId, map[string]any{"account_days_ttl": in.Ttl}); err != nil {
		c.Logger.Errorf("user.setAccountDaysTTL - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
