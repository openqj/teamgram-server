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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogDeleteDialogFilter
// dialog.deleteDialogFilter user_id:long id:int = Bool;
func (c *DialogCore) DialogDeleteDialogFilter(in *dialog.TLDialogDeleteDialogFilter) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.DialogFilters == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, err := c.svcCtx.Dao.Postgres.Store.DialogFilters.Clear(c.ctx, in.UserId, in.Id)
	if err != nil {
		c.Logger.Errorf("dialog.deleteDialogFilter - delete filter error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
