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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogGetPinnedSavedDialogs
// dialog.getPinnedSavedDialogs user_id:long = SavedDialogList;
func (c *DialogCore) DialogGetPinnedSavedDialogs(in *dialog.TLDialogGetPinnedSavedDialogs) (*dialog.SavedDialogList, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.SavedDialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var (
		meId  = in.GetUserId()
		dList = dialog.MakeTLSavedDialogList(&dialog.SavedDialogList{
			Count:   0,
			Dialogs: []*mtproto.SavedDialog{},
		}).To_SavedDialogList()
	)

	rows, err := c.svcCtx.Dao.Postgres.Store.SavedDialogs.SelectPinnedDialogs(c.ctx, meId)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		dList.Dialogs = append(dList.Dialogs, c.svcCtx.Dao.MakeSavedDialog(&rows[i]))
	}
	dList.Count = int32(len(dList.Dialogs))

	return dList, nil
}
