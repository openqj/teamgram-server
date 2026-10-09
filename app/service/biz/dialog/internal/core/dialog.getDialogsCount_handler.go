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

// DialogGetDialogsCount
// dialog.getDialogsCount user_id:long exclude_pinned:Bool folder_id:int = Int32;
func (c *DialogCore) DialogGetDialogsCount(in *dialog.TLDialogGetDialogsCount) (*mtproto.Int32, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetFolderId() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	rows, err := c.svcCtx.Dao.Postgres.Store.Dialogs.SelectDialogs(c.ctx, in.GetUserId(), in.GetFolderId())
	if err != nil {
		c.Logger.Errorf("dialog.getDialogsCount - select dialogs error: %v", err)
		return nil, err
	}

	excludePinned := mtproto.FromBool(in.GetExcludePinned())
	count := 0
	for _, row := range rows {
		if excludePinned && row.Pinned > 0 {
			continue
		}
		count++
	}

	return mtproto.MakeTLInt32(&mtproto.Int32{V: int32(count)}).To_Int32(), nil
}
