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
	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserSetContentSettings
// user.setContentSettings flags:# user_id:long sensitive_enabled:flags.0?true = Bool;
func (c *UserCore) UserSetContentSettings(in *user.TLUserSetContentSettings) (*mtproto.Bool, error) {
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	var (
		k, v string
	)

	k = "sensitive_enabled"

	if in.SensitiveEnabled {
		v = "true"
	} else {
		v = "false"
	}

	// TODO: check
	// 403	SENSITIVE_CHANGE_FORBIDDEN	You can't change your sensitive content settings.

	err := c.svcCtx.Dao.Postgres.InTx(c.ctx, func(tx pgx.Tx) error {
		_, _, err := c.svcCtx.Dao.Postgres.Store.Settings.InsertOrUpdateTx(c.ctx, tx, &dataobject.UserSettingsDO{
			UserId: in.UserId, Key2: k, Value: v,
		})
		return err
	})
	if err != nil {
		c.Logger.Errorf("user.setContentSettings - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
