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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogInsertOrUpdateDialogFilter
// dialog.insertOrUpdateDialogFilter user_id:long id:int dialog_filter:DialogFilter = Bool;
func (c *DialogCore) DialogInsertOrUpdateDialogFilter(in *dialog.TLDialogInsertOrUpdateDialogFilter) (*mtproto.Bool, error) {
	dialogFilterData, err := jsonx.Marshal(in.GetDialogFilter())
	isChatlist := in.GetDialogFilter().GetPredicateName() == mtproto.Predicate_dialogFilterChatlist

	if err != nil {
		c.Logger.Errorf("dialog.insertOrUpdateDialogFilter - error: %v", err)
		return nil, err
	}

	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.DialogFilters == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, _, err = c.svcCtx.Dao.Postgres.Store.DialogFilters.InsertOrUpdate(
		c.ctx,
		&dataobject.DialogFiltersDO{
			UserId:         in.UserId,
			DialogFilterId: in.Id,
			IsChatlist:     isChatlist,
			DialogFilter:   string(dialogFilterData),
			OrderValue:     time.Now().Unix() << 32,
			FromSuggested:  -1,
			Deleted:        false,
		})
	if err != nil {
		c.Logger.Errorf("dialog.insertOrUpdateDialogFilter - persist filter error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
