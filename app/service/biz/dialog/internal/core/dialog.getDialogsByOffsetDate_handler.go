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
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogGetDialogsByOffsetDate
// dialog.getDialogsByOffsetDate user_id:long exclude_pinned:Bool offset_date:int limit:int = Vector<DialogExt>;
func (c *DialogCore) DialogGetDialogsByOffsetDate(in *dialog.TLDialogGetDialogsByOffsetDate) (*dialog.Vector_DialogExt, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetOffsetDate() < 0 || in.GetLimit() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetLimit() == 0 {
		return &dialog.Vector_DialogExt{Datas: dialog.DialogExtList{}}, nil
	}
	if in.GetLimit() > 1000 {
		return nil, mtproto.ErrLimitInvalid
	}

	rows, err := c.svcCtx.Dao.DialogsDAO.SelectDialogs(c.ctx, in.GetUserId(), 0)
	if err != nil {
		c.Logger.Errorf("dialog.getDialogsByOffsetDate - select dialogs error: %v", err)
		return nil, err
	}
	excludePinned := mtproto.FromBool(in.GetExcludePinned())
	offsetDate := int64(in.GetOffsetDate())

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Date2 == rows[j].Date2 {
			return rows[i].PeerDialogId > rows[j].PeerDialogId
		}
		return rows[i].Date2 > rows[j].Date2
	})

	result := make(dialog.DialogExtList, 0, in.GetLimit())
	for _, row := range rows {
		if excludePinned && row.Pinned > 0 {
			continue
		}
		if offsetDate > 0 && row.Date2 > offsetDate {
			continue
		}
		result = append(result, c.svcCtx.Dao.MakeDialog(&row))
		if len(result) == int(in.GetLimit()) {
			break
		}
	}

	return &dialog.Vector_DialogExt{Datas: result}, nil
}
