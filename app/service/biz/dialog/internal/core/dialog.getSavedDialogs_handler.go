// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogGetSavedDialogs
// dialog.getSavedDialogs user_id:long exclude_pinned:Bool offset_date:int offset_id:int offset_peer:PeerUtil limit:int = SavedDialogList;
func (c *DialogCore) DialogGetSavedDialogs(in *dialog.TLDialogGetSavedDialogs) (*dialog.SavedDialogList, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.SavedDialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var (
		excludePinned = mtproto.FromBool(in.GetExcludePinned())
		meId          = in.GetUserId()
		limit         = in.GetLimit()
		offsetId      = in.GetOffsetId()
		dList         = dialog.MakeTLSavedDialogList(&dialog.SavedDialogList{
			Count:   0,
			Dialogs: []*mtproto.SavedDialog{},
		}).To_SavedDialogList()
	)

	if offsetId <= 0 {
		offsetId = math.MaxInt32
	}

	if limit == 0 {
		limit = 100
	}

	if excludePinned {
		rows, err := c.svcCtx.Dao.Postgres.Store.SavedDialogs.SelectExcludePinnedDialogs(c.ctx, meId, offsetId, limit)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			dList.Dialogs = append(dList.Dialogs, c.svcCtx.Dao.MakeSavedDialog(&rows[i]))
		}
		count, err := c.svcCtx.Dao.Postgres.Store.SavedDialogs.Count(c.ctx, meId, true)
		if err != nil {
			return nil, err
		}
		dList.Count = int32(count)
	} else {
		rows, err := c.svcCtx.Dao.Postgres.Store.SavedDialogs.SelectDialogs(c.ctx, meId, offsetId, limit)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			dList.Dialogs = append(dList.Dialogs, c.svcCtx.Dao.MakeSavedDialog(&rows[i]))
		}
		count, err := c.svcCtx.Dao.Postgres.Store.SavedDialogs.Count(c.ctx, meId, false)
		if err != nil {
			return nil, err
		}
		dList.Count = int32(count)
	}

	return dList, nil
}
