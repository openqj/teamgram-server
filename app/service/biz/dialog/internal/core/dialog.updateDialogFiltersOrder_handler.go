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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogUpdateDialogFiltersOrder
// dialog.updateDialogFiltersOrder user_id:long order:Vector<long> = Bool;
func (c *DialogCore) DialogUpdateDialogFiltersOrder(in *dialog.TLDialogUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, mtproto.ErrInternalServerError
	}
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer func() { _ = tx.Rollback(c.ctx) }()
		orderV := time.Now().Unix() << 32
		for _, id := range in.Order {
			if _, err = c.svcCtx.Dao.Postgres.Store.DialogFilters.UpdateOrderTx(c.ctx, tx, orderV, in.UserId, id); err != nil {
				break
			}
			orderV--
		}
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		c.Logger.Errorf("dialog.updateDialogFiltersOrder - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
